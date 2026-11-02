package world

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"sync"

	"github.com/noxer/worlds/internal/geom"
)

// Config describes how to assemble a world.
type Config struct {
	Seed          uint64
	Width, Height int64 // main map size in tiles; multiples of ChunkSize

	// Tilesets lists tileset files to load in addition to those referenced
	// by maps (generators may use tilesets no patch uses).
	Tilesets []string
	// Patches is a Tiled .world file placing hand-made patches on the main
	// map. Offsets may be negative or beyond the size; they wrap.
	Patches string
	// Details is a directory of detail-map templates (*.tmj).
	Details string
	// Pool is a directory of pool patches (*.tmj): hand-made pieces such as
	// small islands that the world places by itself, at most one per area of
	// AreaSize x AreaSize tiles, where the generator reports a suitable spot.
	Pool string
	// Generator creates the procedural generator once tilesets are loaded.
	// Nil means the main map is empty (void) outside patches.
	Generator func(GenInfo) (Generator, error)
	// CacheChunks bounds the number of resolved main-map chunks kept in
	// memory. Default 1024 (about 13 MB).
	CacheChunks int
}

// Warp moves an entity that steps on From to To.
type Warp struct {
	From      Pos
	To        Pos
	Facing    Dir // facing on arrival, valid if HasFacing
	HasFacing bool
}

// Spawn is a named arrival or start point.
type Spawn struct {
	Pos       Pos
	Facing    Dir
	HasFacing bool
}

type patch struct {
	src      *sourceMap
	x, y     int64  // normalised origin on the main map
	surround string // what the generator must put around it ("" = anything)
	margin   int64  // width of the surroundings in tiles
	pool     bool   // placed by the world from the pool
}

type template struct {
	src    *sourceMap
	chunks map[[2]int64]*Chunk
}

type warpKey struct {
	m    MapID
	x, y int64
}

// World is the complete, loaded world.
type World struct {
	Seed uint64
	W, H int64

	tilesets    *Tilesets
	gen         Generator
	patches     []*patch
	patchIdx    map[[2]int64][]*patch // fixed patches by chunk they paint
	surroundIdx map[[2]int64][]*patch // fixed patches by chunk their surroundings reach
	pool        []poolEntry
	poolCache   map[[2]int64][]*patch
	poolMu      sync.Mutex
	stream      *streamer
	templates   map[MapID]*template
	cache       *chunkCache

	spawns  map[MapID]map[string]Spawn
	warps   map[warpKey]*Warp
	allWarp []*Warp
	objects []Object
	blocked map[Pos]int
	version uint64
	stats   Stats
}

// Tilesets returns the tileset registry.
func (w *World) Tilesets() *Tilesets { return w.tilesets }

// Templates returns the ids of all detail-map templates, sorted.
func (w *World) Templates() []MapID {
	ids := make([]MapID, 0, len(w.templates))
	for id := range w.templates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Size returns the size of a map in tiles and whether it wraps.
func (w *World) Size(m MapRef) (width, height int64, wraps bool) {
	if m.IsMain() {
		return w.W, w.H, true
	}
	if t := w.templates[m.Template]; t != nil {
		return t.src.w, t.src.h, false
	}
	return 0, 0, false
}

// Normalize wraps a main-map position into range. For detail maps it reports
// whether the position is inside the map.
func (w *World) Normalize(p Pos) (Pos, bool) {
	if p.Map.IsMain() {
		p.X, p.Y = geom.Mod(p.X, w.W), geom.Mod(p.Y, w.H)
		return p, true
	}
	t := w.templates[p.Map.Template]
	if t == nil || p.X < 0 || p.Y < 0 || p.X >= t.src.w || p.Y >= t.src.h {
		return p, false
	}
	return p, true
}

// Offset returns p moved by (dx, dy), wrapped on the main map.
func (w *World) Offset(p Pos, dx, dy int64) (Pos, bool) {
	return w.Normalize(Pos{Map: p.Map, X: p.X + dx, Y: p.Y + dy})
}

// Neighbor returns the tile one step from p in direction d. On the main map
// this wraps around the world; on detail maps it fails at the border. Warps
// are not followed.
func (w *World) Neighbor(p Pos, d Dir) (Pos, bool) {
	dx, dy := d.Delta()
	return w.Offset(p, dx, dy)
}

// Delta returns the shortest offset from a to b. ok is false if they are on
// different maps.
func (w *World) Delta(a, b Pos) (dx, dy int64, ok bool) {
	if a.Map != b.Map {
		return 0, 0, false
	}
	if a.Map.IsMain() {
		return geom.WrapDelta(a.X, b.X, w.W), geom.WrapDelta(a.Y, b.Y, w.H), true
	}
	return b.X - a.X, b.Y - a.Y, true
}

// DirBetween returns the direction from a to an adjacent tile b.
func (w *World) DirBetween(a, b Pos) (Dir, bool) {
	dx, dy, ok := w.Delta(a, b)
	if !ok {
		return Down, false
	}
	for _, d := range Dirs {
		if ddx, ddy := d.Delta(); ddx == dx && ddy == dy {
			return d, true
		}
	}
	return Down, false
}

// chunk returns the chunk holding the normalised position p, resolving it if
// necessary. It returns nil outside detail maps.
func (w *World) chunk(p Pos) (c *Chunk, lx, ly int) {
	cx, cy := geom.FloorDiv(p.X, ChunkSize), geom.FloorDiv(p.Y, ChunkSize)
	lx, ly = int(p.X-cx*ChunkSize), int(p.Y-cy*ChunkSize)
	key := [2]int64{cx, cy}
	if !p.Map.IsMain() {
		t := w.templates[p.Map.Template]
		if t == nil {
			return nil, 0, 0
		}
		return t.chunks[key], lx, ly
	}
	if c = w.cache.get(key); c == nil {
		// Not prefetched in time (or streaming is off): build it now.
		c = w.resolve(cx, cy)
		w.cache.put(key, c)
		w.stats.SyncBuilt++
	}
	return c, lx, ly
}

// resolve builds a main-map chunk: procedural base, then patches on top.
func (w *World) resolve(cx, cy int64) *Chunk {
	c := &Chunk{}
	if w.gen != nil {
		w.gen.Generate(w.Seed, &ChunkBuilder{CX: cx, CY: cy, c: c, w: w, ww: w.W, wh: w.H})
	}
	for _, p := range w.patchesNear(cx, cy, false) {
		for ly := int64(0); ly < ChunkSize; ly++ {
			py := geom.Mod(cy*ChunkSize+ly-p.y, w.H)
			if py >= p.src.h {
				continue
			}
			for lx := int64(0); lx < ChunkSize; lx++ {
				px := geom.Mod(cx*ChunkSize+lx-p.x, w.W)
				if px >= p.src.w || !p.src.owns(px, py) {
					continue
				}
				for l := 0; l < NumLayers; l++ {
					c.layers[l][ly*ChunkSize+lx] = p.src.tile(l, px, py)
				}
			}
		}
	}
	c.computeDerived(w.tilesets)
	return c
}

// CachedChunks returns the number of main-map chunks currently in memory.
func (w *World) CachedChunks() int { return w.cache.len() }

// Tile returns the tile of a layer at p (empty outside maps).
func (w *World) Tile(p Pos, layer int) Tile {
	p, ok := w.Normalize(p)
	if !ok {
		return 0
	}
	c, lx, ly := w.chunk(p)
	if c == nil {
		return 0
	}
	return c.Tile(layer, lx, ly)
}

// Solid reports whether p blocks movement: static tiles or dynamic blockers.
// Positions outside detail maps are solid.
func (w *World) Solid(p Pos) bool {
	p, ok := w.Normalize(p)
	if !ok {
		return true
	}
	c, lx, ly := w.chunk(p)
	if c == nil || c.solid[ly*ChunkSize+lx] {
		return true
	}
	return w.blocked[p] > 0
}

// Terrain returns the terrain at p. Positions outside detail maps are land
// (they are solid anyway).
func (w *World) Terrain(p Pos) Terrain {
	p, ok := w.Normalize(p)
	if !ok {
		return Land
	}
	c, lx, ly := w.chunk(p)
	if c == nil {
		return Land
	}
	return c.terrain[ly*ChunkSize+lx]
}

// SetBlocked adds or removes a dynamic blocker at p (doors, pushed blocks).
func (w *World) SetBlocked(p Pos, blocked bool) {
	p, ok := w.Normalize(p)
	if !ok {
		return
	}
	if blocked {
		w.blocked[p]++
	} else if w.blocked[p] > 1 {
		w.blocked[p]--
	} else {
		delete(w.blocked, p)
	}
	w.version++
}

// Version changes whenever dynamic passability changes. Navigation caches
// are keyed on it.
func (w *World) Version() uint64 { return w.version }

// WarpAt returns the warp whose trigger tile is p, or nil.
func (w *World) WarpAt(p Pos) *Warp {
	p, ok := w.Normalize(p)
	if !ok {
		return nil
	}
	return w.warps[warpKey{p.Map.Template, p.X, p.Y}]
}

// Warps returns all static warps.
func (w *World) Warps() []*Warp { return w.allWarp }

// Spawn returns a named spawn point of a map (MainMap or a template id).
func (w *World) Spawn(m MapID, name string) (Spawn, bool) {
	s, ok := w.spawns[m][name]
	return s, ok
}

// Objects returns all objects of all patches and templates, with positions
// on their maps (default instances for templates).
func (w *World) Objects() []Object { return w.objects }

// PatchOrigin returns the main-map position of a patch's top-left corner.
func (w *World) PatchOrigin(name MapID) (Pos, bool) {
	for _, p := range w.patches {
		if p.src.id == name {
			return Pos{Map: Main, X: p.x, Y: p.y}, true
		}
	}
	return Pos{}, false
}

// Load assembles a world from its configuration.
func Load(fsys fs.FS, cfg Config) (*World, error) {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width%ChunkSize != 0 || cfg.Height%ChunkSize != 0 {
		return nil, fmt.Errorf("world size %dx%d must be positive multiples of %d", cfg.Width, cfg.Height, ChunkSize)
	}
	if cfg.CacheChunks <= 0 {
		cfg.CacheChunks = 1024
	}
	w := &World{
		Seed: cfg.Seed, W: cfg.Width, H: cfg.Height,
		tilesets:    newTilesets(),
		patchIdx:    map[[2]int64][]*patch{},
		surroundIdx: map[[2]int64][]*patch{},
		poolCache:   map[[2]int64][]*patch{},
		templates:   map[MapID]*template{},
		cache:       newChunkCache(cfg.CacheChunks),
		spawns:      map[MapID]map[string]Spawn{},
		warps:       map[warpKey]*Warp{},
		blocked:     map[Pos]int{},
	}
	for _, ts := range cfg.Tilesets {
		if _, err := w.tilesets.load(fsys, ts); err != nil {
			return nil, err
		}
	}
	if cfg.Details != "" {
		if err := w.loadTemplates(fsys, cfg.Details); err != nil {
			return nil, err
		}
	}
	if cfg.Patches != "" {
		if err := w.loadPatches(fsys, cfg.Patches); err != nil {
			return nil, err
		}
	}
	if cfg.Generator != nil {
		g, err := cfg.Generator(GenInfo{Tilesets: w.tilesets, W: w.W, H: w.H})
		if err != nil {
			return nil, fmt.Errorf("generator: %w", err)
		}
		w.gen = g
	}
	if cfg.Pool != "" {
		if err := w.loadPool(fsys, cfg.Pool); err != nil {
			return nil, err
		}
	}
	if err := w.resolveObjects(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *World) loadTemplates(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if path.Ext(e.Name()) != ".tmj" {
			continue
		}
		src, err := loadSource(fsys, path.Join(dir, e.Name()), w.tilesets)
		if err != nil {
			return err
		}
		if src.id == MainMap {
			return fmt.Errorf("detail map may not be called %q", MainMap)
		}
		if _, err := ParseLighting(src.props.String("lighting")); err != nil {
			return fmt.Errorf("detail map %s: %w", src.id, err)
		}
		t := &template{src: src, chunks: map[[2]int64]*Chunk{}}
		for cy := int64(0); cy*ChunkSize < src.h; cy++ {
			for cx := int64(0); cx*ChunkSize < src.w; cx++ {
				c := &Chunk{}
				for ly := int64(0); ly < ChunkSize; ly++ {
					for lx := int64(0); lx < ChunkSize; lx++ {
						x, y := cx*ChunkSize+lx, cy*ChunkSize+ly
						if x >= src.w || y >= src.h {
							continue
						}
						for l := 0; l < NumLayers; l++ {
							c.layers[l][ly*ChunkSize+lx] = src.tile(l, x, y)
						}
					}
				}
				c.computeDerived(w.tilesets)
				t.chunks[[2]int64{cx, cy}] = c
			}
		}
		w.templates[src.id] = t
	}
	return nil
}

func (w *World) loadPatches(fsys fs.FS, name string) error {
	var tw tiledWorld
	if err := readJSON(fsys, name, &tw); err != nil {
		return err
	}
	owner := map[[2]int64]MapID{}
	for _, e := range tw.Maps {
		if e.X%TileSize != 0 || e.Y%TileSize != 0 {
			return fmt.Errorf("%s: patch %q is not aligned to the tile grid", name, e.FileName)
		}
		src, err := loadSource(fsys, path.Join(path.Dir(name), e.FileName), w.tilesets)
		if err != nil {
			return err
		}
		for _, q := range w.patches {
			if q.src.id == src.id {
				return fmt.Errorf("%s: duplicate patch %q", name, src.id)
			}
		}
		if src.w > w.W || src.h > w.H {
			return fmt.Errorf("%s: patch %q is larger than the world", name, src.id)
		}
		p := &patch{src: src, x: geom.Mod(int64(e.X)/TileSize, w.W), y: geom.Mod(int64(e.Y)/TileSize, w.H)}
		// Overlapping patches are fine as long as they don't paint the same cell.
		for y := int64(0); y < src.h; y++ {
			for x := int64(0); x < src.w; x++ {
				if !src.owns(x, y) {
					continue
				}
				key := [2]int64{geom.Mod(p.x+x, w.W), geom.Mod(p.y+y, w.H)}
				if other, dup := owner[key]; dup {
					return fmt.Errorf("%s: patches %q and %q both paint tile (%d,%d)", name, other, src.id, key[0], key[1])
				}
				owner[key] = src.id
			}
		}
		if p.surround, p.margin, err = patchSurround(src, ""); err != nil {
			return err
		}
		w.indexPatch(p)
		w.patches = append(w.patches, p)
	}
	return nil
}

// resolveObjects places objects on their maps, then processes spawns, warps
// and blockers and validates cross-map references.
func (w *World) resolveObjects() error {
	type placed struct {
		m   MapID
		obj Object
	}
	var all []placed
	for _, p := range w.patches {
		for _, o := range p.src.objects {
			o.Pos, _ = w.Normalize(Pos{Map: Main, X: p.x + o.Pos.X, Y: p.y + o.Pos.Y})
			all = append(all, placed{MainMap, o})
		}
	}
	for _, id := range w.Templates() {
		for _, o := range w.templates[id].src.objects {
			o.Pos.Map = Detail(id)
			all = append(all, placed{id, o})
		}
	}

	var errs []error
	for _, pl := range all {
		o := pl.obj
		w.objects = append(w.objects, o)
		if o.Type != "spawn" {
			continue
		}
		if o.Name == "" {
			errs = append(errs, fmt.Errorf("map %s: spawn object %d has no name", pl.m, o.ID))
			continue
		}
		if w.spawns[pl.m] == nil {
			w.spawns[pl.m] = map[string]Spawn{}
		}
		if _, dup := w.spawns[pl.m][o.Name]; dup {
			errs = append(errs, fmt.Errorf("map %s: duplicate spawn %q", pl.m, o.Name))
			continue
		}
		s := Spawn{Pos: o.Pos}
		s.Facing, s.HasFacing = ParseDir(o.Props.String("facing"))
		w.spawns[pl.m][o.Name] = s
	}
	for _, pl := range all {
		o := pl.obj
		cells := func(fn func(Pos)) {
			for y := int64(0); y < o.H; y++ {
				for x := int64(0); x < o.W; x++ {
					if p, ok := w.Offset(o.Pos, x, y); ok {
						fn(p)
					}
				}
			}
		}
		if o.Props.Bool("solid") {
			cells(func(p Pos) { w.blocked[p]++ })
		}
		if o.Type != "warp" {
			continue
		}
		target := MapID(o.Props.String("to_map"))
		spawn, ok := w.spawns[target][o.Props.String("to_spawn")]
		if !ok {
			errs = append(errs, fmt.Errorf("map %s: warp %d targets unknown spawn %q on map %q", pl.m, o.ID, o.Props.String("to_spawn"), target))
			continue
		}
		cells(func(p Pos) {
			wp := &Warp{From: p, To: spawn.Pos, Facing: spawn.Facing, HasFacing: spawn.HasFacing}
			w.warps[warpKey{p.Map.Template, p.X, p.Y}] = wp
			w.allWarp = append(w.allWarp, wp)
		})
	}
	for _, wp := range w.allWarp {
		if w.Solid(wp.From) {
			errs = append(errs, fmt.Errorf("warp %s: trigger tile is solid", wp.From))
		}
		if w.Solid(wp.To) {
			errs = append(errs, fmt.Errorf("warp %s -> %s: destination is solid", wp.From, wp.To))
		}
		if w.WarpAt(wp.To) != nil {
			errs = append(errs, fmt.Errorf("warp %s -> %s: destination is itself a warp", wp.From, wp.To))
		}
	}
	return errors.Join(errs...)
}
