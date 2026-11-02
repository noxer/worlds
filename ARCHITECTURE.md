# Architecture

A single-player 2D open-world RPG focused on exploration, puzzles, dialog
and quests, with turn-based tactical combat (movement and action points).
The world is planet-sized: a wrapping main map of millions of tiles, mixing
hand-made and procedurally generated content. It is built with Go and
[Ebitengine](https://ebitengine.org), with pixel art from
[Kenney](https://kenney.nl) (CC0). It targets desktop and browser (WASM);
mobile comes later.

> **Status.** Migration steps 1–3 are done: chunked wrapping world, terrain
> and movement modes, and procedural generation with oceans, coastlines and
> islands. This document describes the target design; [Migration](#migration)
> lists what is left.

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
  | any object with `bump: <dialog>` | walking into it shows that dialog's first line as a passive notice: no confirmation, it disappears when the party moves or turns away (e.g. a locked door) |
  | `reserve` | the generator places no features here |
  | `pool` | an area where pool patches (below) may be placed |

- **Surroundings:** a patch can set `surround: land` or `surround: ocean`
  with a `margin` (tiles, default 8). Within the margin the generator puts
  land or water, so a coastline never runs into a hand-made town and an
  island always sits in water. The depth of that water still comes from the
  elevation: an island in the shallows can be reached by swimming.
- **Pool patches** (`maps/pool`, `pool:` in `world.yaml`): generic hand-made
  pieces (small islands now; ruins, oases later) that the world places by
  itself. Each 512×512 area gets at most one, chosen by `weight` from the
  seed and the area's coordinates, at a random spot where the generator
  reports open deep sea for the piece plus its margin, and never near a
  hand-placed patch. Pool patches can't contain objects yet (they would need
  one instance per placement).

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

**Multi-tile features.** Some art spans several tiles: tall trees and the
well have a top half that belongs on the overhead layer one tile above the
bottom half. The tileset declares this, not code: the bottom tile gets the
integer tile property `above` naming its top tile (set in Tiled's tileset
editor, next to `solid`).
- Patches and detail maps are checked when they load. A lower part without
  its upper part above, a lower part in the top row, or a floating upper part
  is a load error naming the file and cell.
- Generators look up tops with `Tilesets.Above`. They decide a feature from
  the coordinates of its root cell, wrap every neighbouring coordinate they
  look at (`ChunkBuilder.Wrap`), and check `ChunkBuilder.Painted` so that a
  feature never gets cut in half by a patch. Where there isn't room, they use
  a one-tile variant instead.
- The content test scans generated land, including the world's seams, for
  incomplete objects, using the same tileset data.

**Tile animations.** Animations are defined in Tiled's tileset editor
(frames of the same tileset, each with a duration) and stored in the `.tsj`.
Maps and generators place the animated tile; only rendering picks the
current frame, from a clock that counts game ticks. Collision, terrain and
`above` always come from the placed tile, and a content test checks that
all frames agree with it. Animations run in sync (water) unless the tile
property `phase: random` gives every cell its own offset (flowers, torches).
If chunks are ever pre-rendered to images for speed, animated tiles get a
separate pass on top.

**Art tools.** Derived art is generated by scripts in `tools/art/` and
committed: `water.py` builds the animated water tileset (`water.png`/`.tsj`)
from Kenney's Tiny Battle water tiles, moving the wave highlights and
keeping shore foam still, and adds deep water with a full set of
deep-to-shallow transitions (4 edges, 4 rounded outer corners, 4 inner
corners, named after where the shallow water is: `deep n`, `deep nw`,
`deep inner nw`, ...). The script prints the tile index of each name; the
coastline autotiler of the generator will use these sets. `worlds_sheet.py`
draws the project's own placeholder tiles.

**The `earth` generator (implemented).**
- **World level:** elevation from tileable value noise in integer
  arithmetic (`gen.Field`), with octaves from 131,072 tiles (continents) down
  to 8 tiles (coastline detail). Below `SeaLevel` is water, below
  `DeepLevel` deep water. `Preview` gives this level cheaply for any
  position (overview and world maps).
- **Chunk level:** per-cell classes for the chunk plus a 6-tile margin.
  Painted patch cells are fixed constraints, and surroundings force land or
  water. Cleanup passes then make sure the art always fits:
  - water and land cells come in 2×2 blocks (no one-tile channels, spits or
    islets);
  - deep water only where all eight neighbours are water;
  - deep water also comes in 2×2 blocks.
- **Autotiling:** water cells get Kenney's shore tiles chosen from the land
  around them, including single and double inner corners; deep cells get
  the deep-to-shallow transitions. Tiles are looked up by their `name`
  property in the water tileset. Neighbourhoods without exact art fall back
  to the closest tile (about 1 in 16,000 cells).
- **Land:** grass, flowers and trees from the grassland generator. Tall trees
  only where there is land above them.
- **Speed:** about 1.2 ms per chunk.

`cmd/worldmap` renders any part of the world to a PNG: terrain colours,
real tile art (`-tiles`), or a whole-planet preview (`-step 1024`). It is the
tool for choosing where hand-made content goes.

Generators are registered by name (`earth`, `grassland`). Later biomes
(desert, forest, mountains) become land generators chosen from further
noise fields (temperature, moisture).

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

**Terrain.** Each tile in a tileset can declare `terrain: shallow` or
`terrain: deep` (anything else is land). The ground layer's tile gives a
cell its terrain; solid tiles still block independently.

**Movement modes.** Passability depends on the terrain of a cell and the
movement mode of the mover (implemented in `nav`, where step durations
double as path costs, so routes are the fastest ones, not the shortest):

| Mode | Can enter | Notes |
|---|---|---|
| walk | land terrain | 16 ticks per tile |
| swim | shallow water only | 24 ticks; entered by walking into shallow water; deep water shows "too deep" |
| ride | land terrain, main map only | 8 ticks; bicycle or scooter, the player's only own vehicles; toggled with R or by clicking the leader; the whole party rides; everyone dismounts at doors; clicking a target that needs walking or swimming dismounts first |
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

- **Loaded chunks (implemented):** each tick the exploration scene asks for
  the chunks within 2 chunks of the party leader, plus 3 more ahead in the
  direction of travel, nearest first (`World.Prefetch`). Workers build them
  in the background; the game thread moves finished chunks into the cache
  once per tick (`World.Pump`, called by the game even while dialogs are
  open). The cache belongs to the game thread alone; building a chunk is a
  pure function, and the only shared mutable state (pool placements) is
  locked.
  - **Desktop:** one worker goroutine.
  - **Browser:** WebAssembly is single-threaded, so `Pump` builds queued
    chunks itself within 3 ms per tick.
  - **Fallback:** a chunk that is needed before it arrives is built on
    demand. The debug overlay (F3) shows both counts: `bg` for prefetched,
    `sync` for on demand.
  - **Eviction:** distant chunks are evicted (least recently used). Detail
    maps in use are fully loaded.
- **Full simulation** (actors moving tile by tile) runs only in loaded chunks
  and the current detail map.
- **Elsewhere, entities are simulated abstractly:** NPC schedules and vehicles
  on routes compute their position from time, and become concrete actors when
  their chunk loads.

## Navigation

Every mover uses the same rules: the keyboard, click/tap-to-move, followers,
NPC schedules, vehicles, and combat.

1. **Local:** A* on tiles in a window around start and goal, with a
   wrap-aware heuristic and the mover's movement mode. Warp tiles count as
   walls unless they are the target.
   - **Wider windows:** if no path fits, the window grows step by step
     (margins 24, 96, 160 tiles), for detours such as walking around a
     fenced village.
   - **Enclosure check:** before any search, a small flood fill from the goal
     (up to 4,096 cells) detects goals in a closed-off area (walled in, an
     island, a pond) and gives up at once.
   - **Feedback:** a click on a free spot that can't be reached shows "There's
     no way there."
2. **Main map, long range (implemented):** for goals beyond the local
   range (up to `MaxFarRange`, 4,096 tiles), and as a fallback when a near
   goal needs a detour wider than the local window:
   - **Corridor:** A* over whole chunks. A chunk's cost comes from 3×3 terrain
     samples: the real tiles for generated chunks, otherwise the generator's
     elevation preview, so planning far ahead never generates chunks. Deep
     water blocks walkers, any water blocks riders.
   - **Stretch:** tile-level A* covers the next 4 corridor chunks, ending on
     the border to the following chunk. The mover plans again when the
     stretch is done, as after a warp.
   - **Learning:** a stretch that can't reach its border (tree lines, inlets
     the preview doesn't see) marks that chunk step as a dead end, and the
     corridor is planned again. Dead ends and cached chunk costs are cleared
     when passability changes.

   This replaces the portal graph first planned here: the preview already
   knows the large obstacles without generating chunks, and the local search
   handles the small ones. Typical cost: about 10 ms per stretch, including
   generating chunks not yet loaded. Unreachable goals give up within tens
   of milliseconds.
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
  geom/              int64 wrap maths
  content/           loads world.yaml, the world and dialogs (no Ebitengine)
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

**Rendering:** the game renders at the screen's physical resolution and
follows real time between its 60 ticks per second, so scrolling stays smooth
when display frames don't line up with updates (frames with two updates or
none, 120/144 Hz screens) and at speeds that aren't a whole number of
pixels per tick (swimming: 16 px in 24 ticks).
- **Exact positions:** an actor's exact position between two ticks is known
  (`Actor.OffsetAt`).
- **Camera:** follows the leader's exact position, so the leader stays still
  on screen.
- **Tiles:** drawn pixel-perfect at game resolution into two layers (ground
  and decoration; overhead), each scaled up by the integer scale and shifted
  by the camera's fraction of a game pixel.
- **Actors:** drawn between those layers at their exact positions, at
  screen resolution.
- **UI** (dialogs, notices, the debug overlay): a separate layer at game
  resolution.

F3 shows the worst chunk, update and draw times of the last second; ticks
over 8 ms are logged to the console while the overlay is on.

**Screen size:** the logical screen is at least 240×180 pixels. It is scaled
by the largest whole-number factor that fits, and leftover space shows more of
the world (320×180 at 1280×720 and 1920×1080).

**Input:** keyboard, standard-layout gamepads, mouse and touch all map to
actions plus a tap position.

**Web:** `make web` builds the browser version into `build/web`.

**Mobile (later):** `ebitenmobile`, plus on-screen controls if tap-to-move
isn't enough. 32-bit ARM is one reason coordinates are explicitly `int64`.

## Migration

1. **Done.** `geom` with `int64` wrap maths. Chunked storage (32×32 tiles,
   compact 4-byte tiles, a bounded cache of 1024 chunks) for main and detail
   maps. The main map is assembled from patches (town, meadow) plus a
   placeholder generator (`grassland`). The smithy is a detail map.
   Navigation searches a window around start and goal, so it works across the
   seam. Rendering is camera-relative. Fingerprint test for the generator.
   Still to come in this area: `tiled/` split from `world/`, and
   resolving chunks in the background (step 4).
2. **Done.** Terrain types (tileset property) and movement modes: swimming
   in shallow water, riding (bicycle, scooter) with faster steps. Followers
   scale their follow delay to the step speed. Art: Kenney Tiny Battle for
   water and shores, animated by `tools/art/water.py` (`water.tsj`, with a
   deep-water variant); the project's own sheet `worlds.png` (placeholder
   bicycle and scooter). Tile animations from Tiled tilesets.
3. **Done.** The `earth` generator: tileable integer noise, continents and
   oceans, cleanup rules, shore and deep-water autotiling, patch
   surroundings, pool islands placed per area, `cmd/worldmap`. The start
   (town and meadow) moved to a coast. Gull Isle is a hand-made island in
   the shallows, reachable by swimming; `islet` is the first pool island.
   Tests cover the coast rules, autotiler fallbacks, determinism, pool
   placement, speed, and a fingerprint of land, coast and sea chunks.
4. Streaming in the background (**done**: workers on desktop, a per-tick
   budget in the browser, tests for identical results and for no on-demand
   builds at riding speed, race detector in CI), long-distance planning
   (**done**: chunk corridors from the preview, stretches, learned dead
   ends), abstract simulation.
5. Public transport: routes, stops and timetables; dynamic warps; interiors as
   instances; window view; taxis.

## Asset delivery

The world itself costs almost nothing to ship: generated chunks come from the
seed, and only patches, detail maps and art are data. Today all content is
embedded with `go:embed`, about 100 KB. The executable is the big part: about
13.6 MB of WebAssembly is Ebitengine and the Go runtime, and the game adds
under 1 MB. (Ebitengine's `text/v2` was replaced by a bitmap glyph atlas,
because its shaping engine alone added 1.5 MB.)

As content grows (art packs, audio, hundreds of patches and detail maps), it
moves out of the binary:
- **One loading abstraction:** all content is read through an `fs.FS`. A
  content pack is a directory (desktop) or a set of files fetched over HTTP
  (browser, mobile).
- **Base pack:** UI, the start area and core tilesets, embedded or fetched at
  start.
- **Regional packs:** patches, detail maps and NPC data for an area of the
  main map, keyed by area coordinates. They are fetched in the background
  when the party's anchor approaches, alongside chunk streaming. Generation
  never waits on the network; a patch that hasn't arrived yet is applied when
  it does, before its chunks are shown.
- **Shared assets** (tilesets, sprites, audio) are fetched on first use and
  cached by the browser. A manifest lists every pack with a content hash, so
  updates only download what changed.
- **Compression** is left to the transport (gzip or brotli over HTTP); the
  WebAssembly binary compresses to about 4 MB.

## World size

2²⁰ × 2¹⁹ tiles (about 1 million × 500,000), set in `assets/world.yaml`
together with the world seed.

## Open questions

- Art: proper deep water with transitions to shallow water, rails, public
  transport vehicles, and riding sprites (the bicycle and scooter are
  placeholders drawn for this project). Owning bikes (inventory) is not
  modelled yet: the party can always ride.
