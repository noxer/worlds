# Architecture

A single-player 2D open-world RPG focused on exploration, puzzles, dialog
and quests, with turn-based tactical combat (movement and action points).
The world is planet-sized: a wrapping main map of millions of tiles, mixing
hand-made and procedurally generated content. It is built with Go and
[Ebitengine](https://ebitengine.org), with pixel art from
[Kenney](https://kenney.nl) (CC0). It targets desktop and browser (WASM);
mobile comes later.

> **Status.** Migration steps 1–4 are done: chunked wrapping world, terrain
> and movement modes, procedural generation with oceans, coastlines and
> islands, background streaming, the world clock with NPC schedules, and day
> and night. This document describes the target design; [Migration](#migration)
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
  | `road_end` | a gate on the patch border where generated paths and roads arrive (see Infrastructure) |

  Patch properties: `surround` and `margin` (below), and `settlement` (a
  tier: the patch is a hand-made settlement, see Infrastructure).

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
   - Rivers, and the routes of roads, rail lines and sea lanes (see
     Infrastructure).
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

**Day and night.** One global time for the whole world (`sim.LightAt`);
local time by longitude can come later, as an offset, when airplanes make
it noticeable.
- **Ambient light:** daylight from 07:30 to 17:30, night (about 40%
  brightness, blue) from 20:30 to 04:30, with violet and warm tones at dawn
  and dusk, blended per game minute.
- **Maps:** the main map follows the time. Detail maps have the map property
  `lighting`: `indoor` (the default, always lit), `outdoor` (follows the
  time) or `dark` (caves, always night).
- **Light sources:** the tile property `light` (radius in tiles, a float) and
  optionally `light_color` (a Tiled color; warm lamp light by default). Lamp
  posts in our own sheet and the Tiny Town house windows have it. They glow
  as strongly as it is dark.
- **Rendering:** after the world, a light map at game resolution is
  multiplied over it: filled with the ambient colour, each glow blended in
  towards its light colour in four hard bands, so it suits the pixel art. In
  full daylight the pass is skipped. The UI is drawn afterwards, unlit.
- F3 (or I, as browsers take F3) shows the brightness; with the overlay on,
  N skips an hour, and T
  jumps to the nearest generated village, town and city in turn, then home.

**Art tools.** Derived art is generated by scripts in `tools/art/` and
committed: `water.py` builds the animated water tileset (`water.png`/`.tsj`)
from Kenney's Tiny Battle water tiles, moving the wave highlights and
keeping shore foam still, and adds deep water with a full set of
deep-to-shallow transitions (4 edges, 4 rounded outer corners, 4 inner
corners, named after where the shallow water is: `deep n`, `deep nw`,
`deep inner nw`, ...). The script prints the tile index of each name; the
coastline autotiler of the generator will use these sets. `worlds_sheet.py`
draws the project's own tiles and writes `worlds.tsj`: bicycle, scooter,
lamp post, and the rail and pier sets, generated for all 16 combinations of
connected sides (named `rail <mask>`, `pier <mask>`, N=1, E=2, S=4,
W=8). `prefabs.py` writes the
starting set of village prefabs (`assets/maps/prefabs`), which are ordinary
Tiled maps from then on.

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

### Infrastructure: settlements and networks

Settlements, roads, rail lines, power lines and cities are generated in
layers. Implemented (package `infra`, drawn by `cmd/worldmap -infra`):
settlements, the network graphs with their routes, and paths and roads as
tiles, villages, rail lines, harbours, and towns and cities. Planned:
highways as tiles, districts with their own block grids, city transit,
power lines. Each layer is a pure function of the seed, the terrain preview and
the layers above it, so any place can still be computed on its own and
chunks stay independent.

**1. Settlements (world level).** A hierarchy of grids, one per tier:

| Tier | One candidate per | Apart on foot | On the planet | Across | Gets |
|---|---|---|---|---|---|
| Metropolis | 131072² tiles | 10 h | 28 | up to 2,000 tiles (63 chunks) | airport, harbour, rail hub, a metro line |
| City | 65536² tiles | 5 h | 87 | 800–1,200 tiles | rail station, harbour on the coast, trams |
| Town | 16384² tiles | 75 min | 1,229 | 300–600 tiles | bus stop, maybe a ferry |
| Village | 4096² tiles | 20 min | 16,405 | 100–200 tiles | dirt road |
| Point of interest | 1024² tiles | 5 min | about 150,000 | a few chunks at most | farm, camp, ruin, cave entrance, shrine |

A tile is taken as about a metre (a person fills one), so a megacity is
about 2 km across: 9 minutes to cross on foot. Settlements are dense
islands in wide countryside.

Walking covers 3.75 tiles per second, so villages are about 20 minutes
apart on foot (10 by bike); beyond that, public transport is the way to
travel. The counts are those of the shipped seed (52% of the planet is
land). Points of
interest keep the walk between villages from being empty: small prefabs
(and later quest hooks) on a dirt path or none, not part of the transport
networks. Pool patches (ponds, islets) stay at their finer grid.

- **Choosing a site:** each grid cell derives six candidate spots from the
  seed hash, kept 15% away from the cell border (so a tier's settlements
  are at least 30% of a cell apart), and scores them from the elevation:
  dry centre, at least 13 of 16 samples around the footprint on land, flat,
  a bonus by the coast. The best one wins, or none; small tiers also leave
  some cells empty by chance (points of interest 30%, villages 10%).
  Later: rivers, valleys, climate.
- **Cost:** "which settlements are near (x, y)" is O(1) anywhere.
- **Footprints:** a settlement has a centre, a size from its tier and seed,
  and an outline distorted by noise and clipped by coast and steep terrain.
  A bigger settlement wins over smaller candidates closer than both radii
  plus a quarter of the smaller tier's cell (bigger tiers are looked up
  first, recursively, and cached). Later, villages inside a city's
  footprint become old-town districts instead.
- **Hand-made towns** register as fixed settlements of a tier with the
  patch property `settlement` (Brookhaven: `village`), centred on the patch,
  and suppress generated candidates of their tier and smaller nearby.

**2. Networks (area level).** One graph per network, each a *relative
neighbourhood graph* over its tiers: two settlements connect unless a third
lies closer to both. It looks natural (no criss-cross shortcuts), contains
the minimum spanning tree (so it is connected), and with a maximum edge
length per network every edge can be computed from nearby settlements only.
- **Highways** between cities; **roads** from towns; **dirt roads** to
  villages.
- **Rail** between cities and metropolises.
- **Sea lanes** between harbours, routed over water. They connect islands
  and continents where the edge length cap ends the road graph.
- **Flight paths** between airports: straight lines on the torus.

Maximum edge lengths: paths 10,240 tiles, roads 40,960, highways
163,840, rail 131,072, sea lanes 40,960; flights any. An edge also needs a
route, so land networks end at deep sea. A sea lane is only kept where at
least 60% of the straight line is water (along a coast, roads do the job).

**Route shape:** A* on a grid of 32 tiles (paths), 64 (roads) or 128
(highways, rail, sea lanes), 8 directions. The result is a polyline per
edge, keyed by its two settlements, computed on demand and cached.
- **Costs:** flat land 100 per unit of length; shallow water 6–12 times
  that (bridges); deep water impassable for land networks except at the
  ends (a town on the shore). Slopes are measured on the relief's smooth
  height (only the octaves of 2,048 tiles and up, without the coastline
  detail) and cost extra percent per unit climbed per tile: paths 18, roads
  26, highways 35, rail 70. Sea lanes prefer deep water and may cross land
  only near their harbours.
- **Rail** is searched in 4 directions only (with the Manhattan
  heuristic) and not smoothed: lines run straight and turn at right angles,
  which the track art can draw. A step off the grid to an end turns a
  corner instead of running diagonally.
- **Meandering:** a random extra cost from the seed by absolute position,
  mostly per block of 16 grid cells, a little per cell (paths up to 60%,
  rail 5%), so routes bend instead of running in straight grid lines, and
  every edge crossing a place sees the same costs.
- **Speed:** routes longer than 64 grid cells are first searched on a grid
  8 times coarser, then on their own grid only within 3 coarse cells of the
  coarse route. The heuristic is weighted by 1.2 (weighted A*, no
  reopening): routes are at most 20% costlier than the best, the searches
  far smaller. A path takes about 1–100 ms, a highway up to about 0.2 s;
  the planet's long-distance networks take about 8 s on 4 cores.
- **Patches:** routes go round hand-made patches (grid cells within half a
  cell of one, and steps whose middle is close to one, are impassable),
  except where they end at a patch's gate.
- **Ways by block:** the routes that cross a block of 1024×1024 tiles are
  found from the graph edges with an end within the maximum edge length
  of it, filtered cheaply first (the search box, or for long routes the
  corridor of the coarse route) so only routes that can come near the block
  are computed. Routes are smoothed into *ways* and cached per edge, the
  ways per block.
- **Background:** computing a new block takes up to about 0.2 s (desktop).
  When the party enters another chunk, `World.Prefetch` asks the generator
  (`world.Prefetcher`) to prepare the 3×3 blocks around it on a background
  goroutine, so chunks find their ways cached. In the browser, where Go
  has one thread and only hands control back to the browser when every
  goroutine waits, that goroutine sleeps for a millisecond every 2 ms of
  work. A block that isn't ready is computed when a chunk needs it.
- **Layering is allowed, cycles are not:** roads may be computed after
  highways and treat them as cheap (so they merge into them), but a network
  never depends on itself, or the result would depend on which chunk asked
  first.
- A fingerprint test covers settlements and route polylines, like the
  terrain one.

**3. Tiles (chunk level).**
- **Paths and roads (implemented):** ways are smoothed with two rounds of
  Chaikin's corner cutting in integer arithmetic (1/16 tile fixed point),
  so every CPU gets the same tiles. A cell is on the way if its centre is
  within the half width of the line: 1.1 tiles for paths (2 wide), 1.6 for
  roads (3 wide). The earth generator paints them before its cleanup:
  - shallow water under a way becomes land, a causeway (bridges need art);
    deep water and painted patch cells are never painted;
  - way cells must come in 2×2 blocks, which the dirt art needs;
  - paths are Tiny Town dirt, autotiled from which neighbours are ways
    (edges, outer corners 12–38, inner corners 39–42); roads are cobbles
    (tile 43, as in Brookhaven); roads paint over paths;
  - no trees on ways.
- **Rail (implemented):** a chain of single cells along the line, track
  tiles on the deco layer chosen by which neighbours are track (straights,
  curves, junctions, buffer stops: one tile per combination of sides).
  The tiles are transparent round the track, so a line over a path or road
  is a level crossing without extra art. Shallow water under the track
  becomes an embankment; no trees on it; village layouts keep houses,
  lanes and footpaths off it. Track is walkable.
- **Harbours (implemented):** a coastal town or bigger with sea lanes gets
  a wooden pier where the shore is nearest its centre along one of the four
  axes: from the last land cell out over the water for up to 10 tiles (at
  least 3), with open water beyond. Sea lanes start off its tip. Pier tiles
  replace the water on the ground layer (walkable), chosen by which sides
  continue the pier or the shore; the cells stay water for the shore art
  around them.
- Highways (with RPG Urban art): bands of fixed width, with curves,
  junctions, bridges and level crossings from the autotiler.
- Power lines: pylons at fixed distances along a line's length (so they
  line up across chunk borders), wires on the overhead layer. They follow
  highways or run straight between cities. Decoration only.
- Painted patch cells win over generated tiles.

**4. Cities: procedural, at every size.** A megacity covers about 4,000
chunks (4 million tiles). Rather than computing a whole city at once, the
layout is a hierarchy where every level can be evaluated locally, from the
seed and the level above, so a chunk only pays for what it overlaps:
1. **City** (O(1)): footprint, centre, and a density that falls from the
   centre outwards (towers downtown, houses in the suburbs, farms at the
   edge).
2. **Districts:** cells of a jittered grid inside the footprint (about
   250–500 tiles), each a Voronoi region with a type chosen from its
   position, terrain and density: downtown, residential, industrial, port
   (on the coast), parks, old town (an absorbed village), the station
   quarter.
3. **Arterial roads:** a relative neighbourhood graph over the district
   centres, plus ring roads at fixed distances from the centre, routed like
   the world networks and cached per edge. Highways and rail lines entering
   the footprint join them.
4. **Streets and blocks:** each district has its own block size (small in
   the old town, large in industrial areas) and offset. Streets are
   axis-aligned (4-directional movement), between arterials; the blocks
   between them are computed per district cell, so a chunk only looks at
   the few blocks it overlaps.
5. **Plots:** a block is split recursively along its longer side down to
   the plot sizes of its district type.
6. **Buildings:** plots are filled from pools of hand-made building
   prefabs per district type and density: towers with tall façades (tops on
   the overhead layer), shops, houses, warehouses, parks and plazas.
7. **Interiors:** buildings are detail-map instances numbered by position.
   Most doors are just locked; enterable buildings (shops, stations, homes
   with quests, apartment blocks with a generated floor) are chosen by rule
   and density, so the content stays manageable.
8. **Landmarks** by rule: the central station where the main rail line
   enters, the harbour on the shore, the airport at the edge, a town hall
   and market per district.

The chunk level draws streets, sidewalks and buildings from the plots and
blocks overlapping the chunk, with the autotiler for junctions and kerbs.
- **Getting around:** a megacity takes 9 minutes to cross on foot (4 by
  bike), so walking works. Transit adds variety more than necessity, and
  is generated from the same graph: trams or buses along arterials, and in
  a metropolis a metro line with underground stations (detail maps reached
  by stairs). Taxis everywhere.
- **Life (pedestrians implemented, `scene/crowd.go`):** pedestrians are
  ambient, not scheduled (only named NPCs have schedules). `infra.Footfall`
  says how busy a public tile is: 4 on plazas, downtown and in old
  quarters, 2 in residential streets and parks, 1 in suburbs, industrial
  estates and village lanes (the square 2); pedestrians walk sidewalks,
  plazas, park paths, minor streets, crosswalks (not arterials elsewhere),
  and the square, lanes and footpaths of villages.
  - Every 60 ticks the scene samples 24 tiles round the party; up to a
    third of their footfall (at most 16) people are about.
  - They turn up every 20 ticks, 13–22 tiles away (out of sight), on a busy
    tile (more likely the busier), and are gone beyond 30 tiles or when the
    party goes indoors.
  - They stroll a little slower than walking, mostly straight on, turning
    now and then, pausing, never back unless they must; they keep off the
    party and each other.
  - City people are the six RPG Urban characters, with four-direction walk
    frames (`entity.Sprite.Walk`; animated sprites don't hop); villagers
    are Tiny Dungeon people. Each has a random line (`assets/dialog/crowd.yaml`).
  Traffic comes with public transport.
- **Navigation:** the long-range planner's corridor search must know about
  cities: the preview for city cells comes from the block level (street or
  building), which is cheap to evaluate.
- **Villages and towns** use the same pipeline with one or a few districts
  and organic streets, so there is one generator, not one per size.

**Towns and cities (implemented, `infra/city.go`).** Towns, cities and
metropolises are laid out on a grid of superblocks every A tiles (36, 40,
44 by tier) around their centre, evaluated tile by tile, so a chunk only
looks at the superblocks it overlaps (each worked out once and cached).
Arterials form one grid per city; districts vary what is inside it.
1. **Arterials:** 3 lanes between superblocks: edge lines on the outer
   lanes, a dashed centre line (RPG Urban 406/433/460 east–west,
   461/462/463 north–south), crosswalks next to the junctions, where at
   least one neighbouring superblock belongs to the city. Junctions, minor
   streets and yards are plain asphalt (441).
2. **Superblocks** belong to the city if their centre is within the radius
   in that direction (78–100% of it, varying smoothly over 16 sectors, so
   the outline is irregular) and their centre and corners are dry and away
   from hand-made patches. The centre one is the plaza: with the station on
   its north side for cities, a square for towns; rows of trees, benches
   and lamps.
3. **Districts:** superblocks belong to the nearest seed of a jittered grid
   of 3×3 superblocks; the seed's density (100% at the centre, 0% at the
   radius) and a roll give the district's character:
   - *core* (cities, dense): downtown towers, paved yards, lamps, one
     material favoured;
   - *old quarter*: small blocks (8 tall) in 3 columns, terraces without
     gaps, four times the shops, red brick, paved yards;
   - *residential grid*: blocks 9 tall in 2 columns, gardens, sidewalk
     trees;
   - *garden suburb*: suburban houses with wide gaps, trees in the yards,
     1 or 2 columns;
   - *industrial* (cities): warehouses with loading bays, paved yards with a
     parking strip at the back;
   - *green belt*: one in three superblocks a park (elsewhere one in 9, in
     the core one in 14).
   Within the character, density still picks downtown, residential or
   suburb, and block heights vary by a row.
4. **Blocks:** minor streets (2 wide) cut a superblock into 1–3 columns
   (at least 12 tiles wide) and as many rows as fit. Each block has a
   kerbed sidewalk round it (the RPG Urban 3×3 set by kerb sides) and a row
   of buildings along its bottom, centred, facing the sidewalk below.
5. **Buildings** are prefabs of kind `building` with a `style`, `material`
   and `shop` flag, picked by weight to fit (the district's material three
   times as likely, shops by its factor); none on a rail line.
6. **Rail** keeps out of station plazas: lines end at a buffer stop on the
   plaza's north edge, behind the station, coming from the arterial north
   of it; the search treats the plaza as impassable and the last legs go
   round it at right angles.
7. In chunks, city tiles replace the terrain, paths and roads (rail stays,
   over the asphalt); wild trees give way to the city's own.

**Villages (implemented, `infra/site.go`).** A generated village's layout
is planned once on a grid around its centre (radius plus 8 tiles) and
cached; hand-made settlements keep their own.
1. Footprint: the cells of the circle that are dry (elevation at least 48
   above sea level) and not within 2 tiles of a hand-made patch.
2. Streets: the ways crossing it (paths and roads end at the centre).
3. The square: 7×7 cobbles at the centre, the `well` prefab in the middle,
   a lamp post on each corner.
4. Lanes: 2-wide dirt lanes from the square, 30–45% of the radius long, in
   directions no way leaves it, until at least two directions have a
   street (and sometimes more).
5. Houses and gardens: 10–16 prefabs by weight, each tried a few tiles from
   a random street cell within 55% of the radius; the prefab and a ring
   round it must be open ground, and a house's front step must reach a
   street, lane, the square or another footpath within 14 steps
   (breadth-first, 4 directions). The footpath is cobbles.
6. In chunks: cobbles for the square and footpaths (no 2×2 rule, unlike
   dirt), dirt for lanes, land under buildings and yards, prefab tiles on
   top, a quarter of the trees elsewhere in the footprint and none next to
   buildings. Prefab objects become generated objects: `solid` blocks the
   cell (`ChunkBuilder.Block`), `bump` gives it a bump dialog
   (`ChunkBuilder.Bump`, `World.Bump`), which Explore shows like those of
   patch objects, and confirm opens.

**Prefabs** are hand-made Tiled maps in `assets/maps/prefabs`
(`prefabs:` in `world.yaml`). Map properties: `kind` (`house`, `garden`,
`well`, or `building` for cities, with `style`: `downtown`, `residential`,
`suburb` or `station`) and `weight`. A city building's door is in its
bottom row, opening onto the sidewalk. A house has one object of type `door` (with
`solid` and `bump: house_locked`) in the row above the bottom row; the
cell below it is the front step. Empty cells keep the generated ground.
The starting set: red and grey houses 3–5 wide in Brookhaven's style, a
garden with beehives, the well. Doors are locked until interiors become
instances (step 6).

**5. Hand-made content.** Patches mark their gates with `road_end`
objects on the patch border (implemented; later also `rail_end`). A route
to a hand-made settlement ends at the gate nearest the other end, arriving
head on: the search starts 4 tiles outside the gate, in the direction the
border faces. Brookhaven's north and south gaps are its gates. Later:
station and harbour objects in a patch register stops.

**6. Public transport** runs on these networks: buses on road edges, trains
on rail edges, ferries on sea lanes, planes on flight paths. Stops are the
settlements' landmarks; timetables follow from edge lengths.

`cmd/worldmap` draws settlements and networks over the planet preview, so
spacing and tiers can be tuned before any tile exists.

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

**Trains (implemented: `transit`, `scene/trains.go`).**
- **Lines:** every rail edge between two cities is a line with one train of
  4 cars shuttling between the buffer stops behind the two stations. Where
  it is comes from the clock alone (like NPC schedules): it waits 10 game
  minutes at a platform, leaves gently (up to half a tile per tick over the
  first 60 tiles), speeds up to about 2,000 tiles a game minute (120 km/h),
  and arrives the same way; each line's cycle starts at an offset from the
  seed. A 100,000-tile line takes under an hour of game time.
- **Exterior:** the lines near the party are drawn on the main map, the
  locomotive (heading where the train goes next) and its cars at exact
  positions between ticks; their tiles block the way. Walking into, or
  confirming at, a waiting train boards it.
- **Stations:** the station door (bump `station`) names the city (cities
  get names from the seed, `infra.Name`; hand-made places take the patch
  property `title`). Confirm opens the timetable, built by the game: for
  each line "boarding now", or the time of the next train with the offer to
  wait for it (the clock moves on). Dialog choices can carry an action
  (`do`) for this.
- **Interior:** the `train` detail map (map property `vehicle`, written by
  `tools/art/train_interior.py`): seats, two passengers, sliding doors
  (`train_exit`). The doors open at the destination ("Now arriving at
  …"), or before departure back at the start; when the train sets off
  back, everyone is put off. Confirming at a seat offers to rest until the
  next station. Leaving puts the party at the station's front door.
- **Window view:** window tiles have transparent glass; behind them, the
  main map is drawn around the train's place, the windows side by side
  like a panorama. Near stations (150 tiles) that is the real place; at
  speed, the real place would be a blur, so the track passes at 0.6 tiles
  per tick instead, cutting back to the real place as the train slows into
  the station. Chunk streaming follows the view.
- Not yet: the `forward` mapping of the scroll direction, tickets, several
  trains per line, ferries, buses, taxis and aircraft.

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
- **World clock (implemented):** `sim.Clock` counts ticks; one game minute
  passes per real second (a day takes 24 real minutes). A new game starts on
  day 1 at 08:00. The clock runs with the exploration scene and stands still
  while a dialog or menu is open and during door fades.
- **Schedules (implemented):** an NPC object with a `schedule` property
  follows a daily routine, e.g. `06:00 lina_home; 08:30 square;
  11:00 smithy/counter`. Places are spawn points: plain names on the main
  map, `template/name` in detail maps. An NPC leaves for the next place at
  the given time and walks there on foot, through doors if need be.
- **Two lives (implemented):** an NPC is *active* while it is on the party's
  map and, on the main map, within 64 tiles (Chebyshev; 72 before an active
  one is dropped, so it doesn't flicker at the border). Active NPCs walk
  their routine tile by tile, block the way and keep off the party's tiles.
  Out of reach they aren't simulated at all: `sim.Where` computes from the
  clock where the routine puts them (the route from the previous place,
  as far as the time since leaving allows). Inactive NPCs are checked every
  half second and placed there when they come into reach. That's far
  outside the screen while walking or riding; after a door the check runs
  at once, while the screen is black. Both directions are therefore
  invisible to the player.
- **Still to come:** vehicles on routes use the same idea. `Where` plans
  the route each time it is asked; with many NPCs it should cache the route
  per schedule leg.

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
  sim/               world clock, schedules, abstract positions
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
- **Light:** the light map of the time of day, multiplied over the world.
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
   ends), abstract simulation (**done**: world clock, NPC schedules,
   activation in reach, `sim.Where` out of reach; Lina in Brookhaven walks
   her routine between home, the square, the smithy and the pond). Day and
   night (**done**: ambient light by time, light sources, lamp posts).
5. Infrastructure (see Infrastructure: settlements and networks):
   1. **done:** settlements and network graphs, shown by `cmd/worldmap`
      only (`-infra`, `-nets`); Brookhaven is a fixed village on the path
      network; tests for spacing, determinism, graph symmetry and
      connectivity, routes across the seam and around deep sea, and a
      fingerprint;
   2. **done:** paths (dirt) and roads (cobbles) as tiles, with causeways
      through shallow water; Brookhaven's gates connected to its two
      neighbour villages; routes round patches; ways cached per block and
      prepared in the background; tests for the paths from the gates and a
      chunk fingerprint with a path;
   3. **done:** generated villages: square with well and lamps, lanes,
      houses and gardens from prefabs with footpaths, locked doors with a
      message; tests for the layouts, reachability of every door in the
      real world, the door notice, and a chunk fingerprint with a village;
   4. **done:** rail lines (4-direction routes, track art of our own for
      every combination of sides, level crossings, embankments) and
      harbours (piers where sea lanes start); tests for straight rail, the
      track tiles along a real line, a walkable pier, and chunk
      fingerprints with track and a pier;
   5. **done:** towns and cities: superblock grid with arterials, minor
      streets, blocks with sidewalks, buildings from RPG Urban prefabs in
      four styles, parks, the station plaza where rail lines end; tests for
      the layout (buildings match their tiles, doors open onto sidewalks)
      and the doors of a real city reachable from its plaza. Then districts
      (core, old quarter, grid, garden suburb, industrial, green belt) and
      a noisy outline. Still open: city transit (with step 6).
6. Public transport on the networks: routes, stops and timetables; dynamic
   warps; interiors as instances; window view; taxis. **Trains done:**
   timetables from the clock, trains on the map, station timetables,
   boarding, the carriage with its window view, arrival and getting off;
   tests for the timetable and motion along a real line and a whole
   journey. Next: ferries on the sea lanes, buses, taxis, aircraft.

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

- Settlement spacing is set by villages about 20 minutes apart on foot;
  the other tiers are derived from that and may need tuning once
  `cmd/worldmap` draws them.
- Urban art: Kenney's RPG Urban Pack (`rpgurban`, 486 tiles) is chosen
  for towns and cities: streets with markings and crossings, sidewalks,
  building façades and roofs, street furniture, taxis, buses and cars (for
  public transport), and characters with four-direction walk animations.
  Its style is softer than Tiny Town (no dark outlines); villages and
  hand-made places keep Tiny Town. `tools/art/urban.py` adds its tile
  properties (solid walls, roofs and furniture, two-tile street lamps that
  light up at night, lit shop windows) and writes the city building
  prefabs: red or orange brick, tan or grey roofs, downtown (5–8 wide, 6–7
  tall), shops with awnings, residential, suburban, and the station.

- Art: proper deep water with transitions to shallow water, rails, public
  transport vehicles, and riding sprites (the bicycle and scooter are
  placeholders drawn for this project). Owning bikes (inventory) is not
  modelled yet: the party can always ride.
