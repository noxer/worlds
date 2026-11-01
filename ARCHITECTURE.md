# Architecture

A single-player 2D open-world RPG focused on exploration, puzzles, dialog
and quests, with turn-based tactical combat (movement and action points).
Built with Go and [Ebitengine](https://ebitengine.org), pixel art from
[Kenney](https://kenney.nl) (Tiny Town and Tiny Dungeon, CC0). It targets
desktop and browser (WASM); mobile comes later.

## Ground rules

- **Tile grid, 4 directions, everywhere.** Exploration and combat use the same
  16×16 grid and only up/down/left/right movement.
- **Logic is separate from rendering.** `world`, `nav`, `entity`, `dialog` (and
  later `combat`, `quest`) don't import Ebitengine, so they are unit-tested
  headless. Scenes are tested by driving `Update` tick by tick with injected
  input (`input.Frame`).
- **Fixed timestep.** Ebitengine calls `Update` 60 times per second. Movement
  is 1 px per tick (16 ticks per tile), so there's no delta time anywhere.
- **Content is data.** Maps are authored in [Tiled](https://www.mapeditor.org),
  and dialogs (later also quests) are YAML. Everything is embedded with
  `go:embed`, so desktop and web builds are identical single files.

## Packages

```
cmd/worlds/          entry point (window setup)
assets/              embedded content + content validation test
  tiles/             Kenney sheets (packed, no spacing)
  tilesets/*.tsj     Tiled tilesets; per-tile property `solid`
  maps/*.tmj         Tiled maps; maps/*.world places maps into regions
  dialog/*.yaml      dialog trees
internal/
  world/             Tiled loading, maps, regions, warps, spawns, collision
  world/worldtest/   builds test worlds from ASCII art
  nav/               hierarchical pathfinding across edges and warps
  entity/            actors (grid movement) and the party
  dialog/            dialog trees and runner
  input/             keyboard/gamepad/mouse/touch -> actions
  gfx/               tile sheets, text, UI boxes
  scene/             scene stack, exploration scene, dialog box
  game/              wiring, ebiten.Game, screen layout
web/                 index.html for the WASM build
```

Planned: `combat/`, `quest/`, `script/` (commands, flags, event bus),
`save/`, `platform/` (file vs. localStorage saves, audio unlock).

## World model

**Positions.** `world.Pos{Map, X, Y}` is a tile on a map.

**Regions.** Maps placed in a Tiled `.world` file form a *region* and share one
coordinate space (the outdoors is region `overworld`). Walking off a map edge
continues on the neighbouring map: `World.Neighbor(pos, dir)` resolves edges.
Edge connections can be partial and offset. Overlapping maps are a load error.
Every map not in a `.world` file (house interiors, caves) is its own region.

**Warps.** A `warp` object (a rectangle of one or more tiles) has `to_map` and
`to_spawn`. Stepping onto it moves the party to that spawn after a fade. The
loader validates targets: the spawn must exist, and neither the trigger tile
nor the destination may be solid or another warp.

**Collision** has two layers:
1. Static: tiles whose tileset marks them `solid`. Layers with the property
   `overhead: true` (roof ridges, tree tops) are drawn above characters and
   ignored for collision.
2. Dynamic: `World.SetBlocked` (objects with `solid: true`, and later doors,
   pushed blocks and other puzzle state). Each change bumps `World.Version()`.

Actors (NPCs, later enemies) block movement but are not part of the world;
they are passed to the planner as a `Blocked` callback.

### Tiled conventions

| Object type | Fields / properties | Meaning |
|---|---|---|
| `spawn` | name, `facing` | arrival / start point |
| `warp` | `to_map`, `to_spawn` | door, stairs, cave mouth |
| `npc` | tile object (sprite), name, `dialog`, `facing` | character |
| `sign` | `dialog`, optional `solid` | interactable object |

Tile layers must use CSV encoding and external `.tsj` tilesets.
`assets/assets_test.go` checks that the content loads, that every referenced
dialog exists, and that every map is reachable from the start.

## Navigation (`nav`)

Every mover uses the same rules: the keyboard, click/tap-to-move, followers,
and later NPC schedules and combat.

1. **Inside a region:** A* on the tile grid in region coordinates, so routes
   cross map edges freely. Warp tiles count as walls unless they are the
   target, so a path never teleports by accident.
2. **Across regions:** Dijkstra over a small graph of warp entries and exits.
   The weight of an edge within a region is the static A* distance, computed
   lazily and cached until `World.Version()` changes. Using a warp costs
   `WarpCost` steps, so short walks beat detours through buildings.
3. **Only the current leg is turned into moves:** up to the first warp, or to
   the goal. That leg avoids dynamic blockers. After a warp the mover plans
   again, so plans stay valid while the world changes. If an NPC steps into
   the way, the mover waits briefly and replans.

Scaling notes for later: precompute warp-to-warp distances per region at load
time, and add region-level chunk abstraction if regions grow very large.

## Party

The leader plus up to `entity.MaxFollowers` (5) companions. When the leader
starts a step, each follower walks onto the tile the member in front of it
just left (a snake). This works across map edges. After a warp everyone
stands on the arrival tile and the queue unfolds as the leader walks on.
Followers don't block the leader (turning back swaps places). The drawing
order puts the leader on top when members share a tile.

## Scenes

A stack: only the top scene updates, but all scenes draw. So a `DialogBox`
pushed over `Explore` freezes the world while it stays visible. Planned
scenes: menu, inventory, quest journal, save/load, title.

**Explore** (real time):
- Holding a direction walks. Tapping a direction only turns.
- Confirm interacts with the tile in front.
- A click or tap walks there via `nav`. Clicking an NPC or sign walks next to
  it and talks.
- `goTo(pos)` accepts any tile in the world, which quest markers or scripted
  travel can reuse.
- F3 toggles a debug overlay (position, region, planned path).

### Combat (planned)

Combat happens on the exploration map itself, with no battle screen. When a
fight starts, Explore swaps its real-time controller for a turn controller:

- **Turns:** initiative order. Each combatant has movement points (tiles) and
  action points per turn.
- **Movement:** reachable tiles come from a cost-limited flood fill in `nav`,
  using the same passability rules as exploration (4 directions).
- **Abilities are data:** AP cost, range, area shape, line of sight
  (straight-line ray on the grid), and effects (damage, status, push and
  pull). Push and pull work on puzzle objects too, so fights and puzzles share
  one mechanic set.
- **Testable rules:** the `combat` package has no Ebitengine dependency and
  uses a seeded RNG, so fights are deterministic in tests and replays.
- **AI:** scores (move target, ability) pairs over the reachable tiles.
- **Scope:** combat is bounded to the current region, and warps are locked
  during a fight. Party members fight; companions are AI-controlled or
  player-controlled (to be decided).

## Dialog, quests, scripting

- **Dialog trees (implemented):** nodes have a speaker, lines, and either
  choices or `next`. They are validated at load. Next: conditions (flags,
  quest stage, items, party members, skill checks with dice) and effects.
- **Scripting (planned):** effects are commands for a small command runner
  (`Say`, `Move`, `Face`, `Wait`, `SetFlag`, `GiveItem`, `Warp`, `StartQuest`,
  …) that runs over several ticks. Story state is a set of named flags.
- **Quests (planned):** defined in YAML, with stages and objectives.
  Objectives complete by listening to game events (`ItemAcquired`,
  `TalkedTo`, `EnteredArea`, `EnemyDefeated`, `FlagSet`). There will be a
  journal scene and quest markers (using `nav` for direction across maps).
- **Puzzles (planned):** objects like levers, plates, push blocks, doors and
  keys, wired in Tiled (`target=door_3`). Their state lives in
  `WorldState[map][objectID]` and affects passability through `SetBlocked`.
- **Saving:** everything that needs saving lives in one serializable game
  state (party, inventory, flags, quests, object states).

## Platforms and screen

**Screen size:** the logical screen is at least 240×180. It is scaled by the
largest integer factor that fits, and leftover space shows more of the world
(320×180 at 1280×720 and 1920×1080). Pixels stay crisp, and odd aspect ratios
(browsers, phones) work.

**Input:** keyboard, standard-layout gamepads, mouse and touch all map to
actions plus a tap position. Tap-to-move and tap-to-advance dialogs already
cover touch.

**Web:** `make web` produces `build/web`, a static site.

**Mobile (later):**
- Ebitengine `ebitenmobile` bindings.
- An on-screen d-pad, if tap-to-move isn't enough.
- Safe-area insets.
