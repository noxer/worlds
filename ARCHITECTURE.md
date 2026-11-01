# Architecture

A single-player 2D open-world RPG focused on exploration, puzzles, dialog
and quests, with turn-based tactical combat (movement and action points).
The world is planet-sized: a wrapping main map of millions of tiles, mixing
hand-made and procedurally generated content. It is built with Go and
[Ebitengine](https://ebitengine.org), with pixel art from
[Kenney](https://kenney.nl) (CC0). It targets desktop and browser (WASM);
mobile comes later.

> **Status.** The first playable version implements an earlier model:
> finite Tiled maps grouped into *regions*. This document describes the target
> design. Section [Migration](#migration) lists the steps from here to there.

## Ground rules

- **Tile grid, 4 directions, everywhere.** 16×16 tiles. Exploration and
  combat use only up/down/left/right movement.
- **Logic is separate from rendering.** World, generation, navigation,
  entities, dialog, quests and combat don't import Ebitengine, so they are
  unit-tested headless. Scenes are tested by driving `Update` tick by tick
  with injected input (`input.Frame`).
- **Fixed timestep.** 60 ticks per second; 1 px per tick when walking.
- **Content is data.** Maps and patches are authored in
  [Tiled](https://www.mapeditor.org), and dialogs and quests in YAML.
  Everything is embedded with `go:embed`.
- **One world for everybody.** The world seed is fixed. The same game version
  generates the same world on every platform (see
  [Determinism](#determinism)).
- **Big coordinates.** Tile and pixel coordinates in the world are `int64`.
  Rendering is always relative to the camera, so values sent to the GPU stay
  small (float32 is only exact up to about 16.7 million).

## Maps

There are two kinds of maps. Both are built from **chunks** of 32×32 tiles.

**The main map** is the open world.
- It has a fixed size `W×H` tiles (multiples of the chunk size) and **wraps on
  both axes** (a torus). Walking off the east edge arrives in the west, and
  likewise north/south.
- All coordinate maths goes through one wrap helper:
  - positions are normalised to `[0,W)×[0,H)`;
  - distances and deltas use the shortest wrapped difference;
  - chunk coordinates wrap too.

**Detail maps** are the small places reached through warps: building
interiors, caves, dungeons, and vehicle interiors.
- They are bounded and don't wrap.
- They use the same chunk storage and code paths, but are fully loaded while
  in use.
- A detail map is a **template**. It can have several **instances** (every
  ship of one class shares an interior layout; procedural dungeons are
  instances of a generator). Each instance has its own state.

```go
type MapRef struct { Template MapID; Instance uint64 } // Instance 0 for singletons
type Pos    struct { Map MapRef; X, Y int64 }          // main map: Map.Template == "main"
```

**Anchor.** Every detail-map instance is anchored at a main-map position: a
building's entrance, a cave's mouth, or a vehicle's current position (which
moves). The anchor of wherever the party is decides what part of the main
map is streamed and simulated, and where the party appears on the world map.

### Chunk resolution

Each main-map chunk is produced by the same pipeline, which runs lazily and
is cached:

```
chunk(cx, cy) = generate(worldSeed, cx, cy)  // procedural base
              ← patches                      // hand-made tiles win, cell by cell
              ← save deltas                  // opened chests, cut trees, moved blocks
```

- A chunk fully covered by patches skips generation.
- A chunk with no patch is purely procedural.
- Most interesting chunks are mixed, for example a hand-drawn coastline with
  generated ocean around it.

The result of resolving a chunk:
- **Tile layers:** ground, decoration, and overhead (drawn above characters,
  no collision).
- **Terrain grid:** one terrain type per cell (deep water, shallow water,
  sand, grass, rock, rail, …), used for movement rules.
- **Objects:** spawns, warps, NPCs, signs, puzzle objects.
- **Portals:** the chunk's border entry/exit points for long-distance
  navigation.

### Hand-made content: patches

- **What a patch is:** a Tiled map covering part of the main map (a town, an
  island, an oasis, a coastline). Patches are placed through `.world` files at
  any tile offset. They don't need to align to chunks and may straddle the
  wrap seam.
- **Painted cells override generated ones.** An empty cell in a patch means
  "generated". A special *void* tile means "explicitly nothing generated
  here".
- **Patches declare their terrain**, via per-tile terrain properties in the
  tileset. The generator treats that terrain as a fixed constraint and blends
  into it over a strip a few tiles wide, so coastlines and roads continue
  across the seam.
- **Patch objects:**

  | Object type | Meaning |
  |---|---|
  | `spawn`, `warp`, `npc`, `sign` | as in detail maps |
  | `reserve` | the generator places no features here |
  | `pool` | an area where pool patches (below) may be placed |

- **Pool patches:** generic hand-made pieces (small islands, ruins, oases)
  that the generator places randomly inside `pool` areas or matching biomes.
  It keeps a minimum distance between them and uses the world seed.

### Procedural content: generation levels

Each level is a pure function of the world seed and coordinates, so nothing
is computed for the whole planet up front.

1. **World level** (one cell spans many chunks): elevation, temperature,
   moisture, and from those the biome.
   - It uses **tileable noise**, periodic in `W` and `H`, so the wrap is
     seamless.
   - It can be evaluated for any location in O(1).
   - The world map is rendered from this level.
2. **Area level** (blocks of 64×64 chunks, generated on demand and cached):
   anything that needs a view wider than one chunk.
   - Rivers, roads, rail lines and sea lanes.
   - Placing pool patches.
   - Reserving space around fixed patches, so a generated continent never runs
     into a hand-made island.
   - Checking that every entrance and island can be reached; the generator
     clears obstacles where a check fails.
3. **Chunk level:** tile detail.
   - Terrain from levels 1 and 2 plus the patch constraints.
   - Coastline and other edge tiles chosen by an *autotiler* from the terrain
     sets defined on the tileset (Tiled "Wang sets"), so swapping the art
     doesn't touch the generator.
   - Decorations scattered with an even-spacing algorithm (Poisson disk).
   - A chunk reads only levels 1–2 and the patches, so chunks can be
     generated in any order, in the background.

Generators are registered per biome (`ocean`, `desert`, `forest`, …). Each is a
small Go type with tests for determinism, connectivity, and "never overwrites
patch cells".

### Determinism

Changing a generator changes the world. So each release ships fingerprints
(hashes) of a set of generated chunks, and CI fails when they change
unintentionally.

The same seed must also give the same world on every platform. Rules:
- **Random numbers:** only use seeded generators derived from (world seed,
  level, coordinates) via a hash. Never use the global random source or any
  iteration order that isn't stable.
- **Noise:** use integer or fixed-point arithmetic, or force float rounding
  with explicit conversions. On ARM (Apple Silicon, phones), Go may fuse
  `x*y+z` into one FMA instruction, which rounds differently than x86.
  Fingerprint tests run on both amd64 and arm64.

### Saving

- Main-map changes are stored sparsely per chunk.
- Detail-map instance state is stored per instance.
- Flags, quests, party, inventory and vehicle states are stored globally.

Untouched chunks cost nothing, because they are regenerated from the seed.

## Terrain and movement

**Movement modes.** Passability depends on the terrain of a cell and the
movement mode of the mover:

| Mode | Can enter | Notes |
|---|---|---|
| walk | land terrain | |
| swim | shallow water only | entered by walking into shallow water |
| ride | land terrain, outdoors only | bicycle or scooter: the player's only own vehicles; faster; the whole party rides; mounting and dismounting are instant |
| passenger | none | travelling in public transport (see Vehicles) |

**Blockers.** Static collision comes from tiles. Dynamic blockers (doors,
pushed blocks, NPCs) come on top. Passability changes bump a version counter
for the affected chunk, which clears navigation caches for that chunk only.

## Vehicles

**Personal vehicles** (bicycle, scooter) are not separate entities. They are
a movement mode of the party: faster steps, ridden sprites, outdoors only.
They have no interior.

**Public transport** (taxi, bus, train, boat, airplane) is never steered by
the player. Each has a **route** with **stops** (stations, harbours, airports,
bus stops) and a timetable. A taxi instead takes the party to a chosen known
destination nearby. Each vehicle has two representations:

- **Exterior:** a small entity on the main map, for example a 2×1-tile boat, a
  few train cars, a bus, or a plane sprite. It moves along its route (roads,
  train lines, sea lanes, flight paths), occupies tiles and blocks other
  movers.
- **Interior:** a detail-map instance, much larger than the exterior (like a
  TARDIS). The party walks around inside, talks to passengers, fights on deck,
  and solves puzzles while the vehicle travels. The interior's anchor is the
  exterior position, so it moves.

**Boarding and leaving** use dynamic warps: warps attached to an entity rather
than a fixed tile.
- **Boarding:** stepping onto the exterior's door or gangway while the vehicle
  is stopped at a stop warps the party into the interior (with a ticket, where
  needed).
- **Leaving:** at a stop, the interior's exit warps to the exterior's current
  door position.
- **In transit:** the doors are closed. A taxi ride, or a long trip the player
  skips, can play as a short scene with the window view instead of
  real-time travel.

**Window view.** While inside, the main map scrolls past the vehicle:
- Cells marked as *exterior view* (a layer property, used for windows, the
  open sides of a deck, or the void around a small boat) show the main map as
  a backdrop.
- The backdrop camera follows the exterior position, smoothed per pixel. Its
  scroll direction is mapped through the interior's `forward` property: a
  train whose interior is drawn left-to-right shows the landscape sliding
  left, whichever way the train actually travels.
- Aircraft render the backdrop at a reduced scale with cloud layers.
- Interior and exterior sizes are not consistent on purpose: the backdrop is
  a view, not geometry.

**Simulation.**
- Vehicles move in the world simulation, independent of the current scene.
- Time passes while the party walks around inside.
- Arrival raises events (doors open, announcements).
- Streaming follows the anchor, so the landscape outside is always loaded.

## Streaming and simulation

- **Loaded chunks:** the chunks within a radius around the current anchor are
  resolved and kept. Chunks ahead of the direction of travel are generated in
  the background. Distant chunks are evicted (least recently used). Detail
  maps in use are fully loaded.
- **Full simulation** (actors moving tile by tile) runs only in loaded chunks
  and the current detail map.
- **Elsewhere, entities are simulated abstractly:** NPC schedules and vehicles
  on routes compute their position from time, and become concrete actors when
  their chunk loads.

## Navigation

Every mover uses the same rules: the keyboard, click/tap-to-move, followers,
NPC schedules, vehicles, and combat.

1. **Local:** A* on tiles over loaded chunks, with a wrap-aware heuristic and
   the mover's movement mode. Warp tiles count as walls unless they are the
   target.
2. **Main map, long range:** a graph of chunk portals. When a chunk is
   resolved, it records its border portals and the costs between them inside
   the chunk. A* runs on the portal graph first, then the current stretch is
   refined tile by tile. Chunks that aren't resolved yet use estimates from
   the world level (biome passability) and are resolved as the route reaches
   them.
3. **Across maps:** a graph of warp entries and exits, including dynamic
   vehicle warps while a vehicle is at a stop. Later, routes can include
   public transport lines as edges.
4. **Only the current leg becomes concrete moves.** After a warp, a chunk
   change or a blocker, the mover plans again.

Earth-scale trips use vehicles, fast travel or the world map; click-to-move
is meant for what's on screen.

## Party

The leader plus up to 5 companions, queued up behind the leader like a snake.
Each follower steps onto the tile the member in front of it just left; this
works across chunk borders and the wrap seam. After a warp everyone stands on
the arrival tile and the queue unfolds again. Followers don't block the leader.
When riding, the whole party rides.

## Scenes

A stack: only the top scene updates, but all scenes draw. Exploration (real
time) and combat (turn based) share one scene on the same map; a fight swaps
the real-time controller for a turn controller. Overlays: dialog, menu,
inventory, quest journal, world map, save/load.

### Combat (planned)

- **Turns:** initiative order. Each combatant has movement points and action
  points per turn.
- **Movement:** reachable tiles come from a cost-limited flood fill in
  navigation, with the same passability rules as exploration (4 directions).
- **Abilities are data:** AP cost, range, area shape, line of sight
  (straight-line ray on the grid), and effects (damage, status, push and
  pull). Push and pull also work on puzzle objects.
- **Testable rules:** the `combat` package has no Ebitengine dependency and
  uses a seeded RNG, so fights are deterministic in tests and replays.
- **AI:** scores (move target, ability) pairs over the reachable tiles.
- **Scope:** warps are locked during a fight. Fights also work inside vehicle
  interiors.

## Dialog, quests, scripting, puzzles

- **Dialog trees (implemented):** nodes have a speaker, lines, and choices or
  `next`, and are validated at load. Planned: conditions (flags, quest stage,
  items, party members, skill checks with dice) and effects.
- **Scripting:** a command runner (`Say`, `Move`, `Face`, `Wait`, `SetFlag`,
  `GiveItem`, `Warp`, `StartQuest`, …) that runs over several ticks. Story
  state is a set of named flags. Game events go through an event bus.
- **Quests:** YAML stages and objectives, completed by events (`ItemAcquired`,
  `TalkedTo`, `EnteredArea`, `EnemyDefeated`, `FlagSet`). A journal and quest
  markers, with direction from navigation even across maps and the wrap
  seam.
- **Puzzles:** objects such as levers, plates, push blocks, doors and keys,
  wired in Tiled (`target=door_3`). State lives in chunk or instance deltas
  and drives the dynamic blockers.

## Packages (target)

```
cmd/worlds/          entry point
assets/              embedded content: tiles, tilesets, patches, detail maps,
                     dialogs, quests; content validation test
internal/
  geom/              int64 positions, wrap maths, chunk coordinates
  world/             main map, detail maps, chunk storage and cache,
                     terrain, warps (static and dynamic), anchors
  tiled/             Tiled loading (maps, tilesets, Wang sets, .world)
  gen/               generation levels, biome generators, autotiler, noise
  nav/               local A*, chunk portal graph, warp graph
  entity/            actors, party, vehicles
  sim/               world clock, abstract simulation, streaming
  dialog/ quest/ script/ combat/ save/
  input/ gfx/ scene/ game/ platform/
```

## Platforms and screen

**Screen size:** the logical screen is at least 240×180 pixels. It is scaled
by the largest whole-number factor that fits, and leftover space shows more of
the world (320×180 at 1280×720 and 1920×1080).

**Input:** keyboard, standard-layout gamepads, mouse and touch all map to
actions plus a tap position.

**Web:** `make web` builds the browser version into `build/web`.

**Mobile (later):** `ebitenmobile`, plus on-screen controls if tap-to-move
isn't enough. 32-bit ARM is one reason coordinates are explicitly `int64`.

## Migration

The first playable version has finite maps, regions stitched together by
`.world` files, and `int` coordinates. The steps to the target design:

1. `geom` with `int64` and wrap maths. Chunked storage for main and detail
   maps. The main map is assembled from patches (town, meadow) plus a stub
   generator (grass and trees). The smithy becomes a detail map. Navigation is
   ported to chunks and the wrap. Rendering becomes camera-relative. All
   current behaviour and tests keep working.
2. Terrain types and movement modes: swimming (shallow water), riding
   (bicycle, scooter).
3. Generation levels 1–3, tileable noise, autotiler, fingerprint tests. The
   first ocean biome, with a fixed and a pool island. Needs water art (a
   placeholder sheet until a pack is chosen).
4. Streaming in the background, the chunk portal graph, abstract simulation.
5. Public transport: routes, stops and timetables; dynamic warps; interiors as
   instances; window view; taxis.

## World size

2²⁰ × 2¹⁹ tiles (about 1 million × 500,000), set in `assets/world.yaml`
together with the world seed.

## Open questions

- Art for water, coastlines, rails, vehicles and riding sprites.
