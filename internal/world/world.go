package world

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
)

// Tileset is a tile sheet referenced by maps.
type Tileset struct {
	Name      string
	Image     string // path of the sheet image inside the asset FS
	Columns   int
	TileCount int
	Margin    int
	Spacing   int
	solid     []bool
}

// Solid reports whether tile index i is marked as blocking.
func (t *Tileset) Solid(i int) bool { return i >= 0 && i < len(t.solid) && t.solid[i] }

// Tile is a resolved tile reference. Set is nil for empty cells.
type Tile struct {
	Set          *Tileset
	Index        int
	FlipH, FlipV bool
}

// Empty reports whether the cell holds no tile.
func (t Tile) Empty() bool { return t.Set == nil }

// Layer is a tile layer. Overhead layers are drawn above characters and do
// not contribute to collision (roof ridges, tree tops).
type Layer struct {
	Name     string
	Overhead bool
	Tiles    []Tile // W*H, row-major
}

// Object is an object from a Tiled object layer, converted to tile units.
type Object struct {
	ID         int
	Name, Type string
	X, Y, W, H int  // tile rectangle covered by the object
	Tile       Tile // sprite for tile objects (NPCs), empty otherwise
	Props      Props
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

// Map is one Tiled map placed inside a region.
type Map struct {
	ID      MapID
	W, H    int
	Layers  []Layer
	Objects []Object
	Props   Props

	Region           *Region
	OriginX, OriginY int // offset of the map's top-left tile in region coordinates

	solid   []bool // static collision from tiles
	blocked []int  // dynamic blockers (objects, puzzle state); counts so they can stack
	warps   map[[2]int]*Warp
	spawns  map[string]Spawn
}

func (m *Map) in(x, y int) bool { return x >= 0 && y >= 0 && x < m.W && y < m.H }

// Spawn returns the named spawn point.
func (m *Map) Spawn(name string) (Spawn, bool) {
	s, ok := m.spawns[name]
	return s, ok
}

// Region is a set of edge-connected maps sharing one coordinate space. A map
// that is not part of any .world file forms a region of its own.
type Region struct {
	ID   string
	Maps []*Map
	// Bounding box of all maps in region tile coordinates.
	MinX, MinY, W, H int
	cells            []int16 // index into Maps per bounding-box cell, -1 = void
}

// At converts region coordinates to a map position. It returns false for void.
func (r *Region) At(rx, ry int) (Pos, bool) {
	x, y := rx-r.MinX, ry-r.MinY
	if x < 0 || y < 0 || x >= r.W || y >= r.H {
		return Pos{}, false
	}
	i := r.cells[y*r.W+x]
	if i < 0 {
		return Pos{}, false
	}
	m := r.Maps[i]
	return Pos{Map: m.ID, X: rx - m.OriginX, Y: ry - m.OriginY}, true
}

// World is the complete, loaded set of maps and regions.
type World struct {
	Maps    map[MapID]*Map
	Regions map[string]*Region
	Warps   []*Warp

	version uint64
}

// Map returns a map by id, or nil.
func (w *World) Map(id MapID) *Map { return w.Maps[id] }

// Version changes whenever passability changes (puzzle state, doors...).
// Navigation caches are keyed on it.
func (w *World) Version() uint64 { return w.version }

// ToRegion converts a map position to region coordinates.
func (w *World) ToRegion(p Pos) (r *Region, rx, ry int) {
	m := w.Maps[p.Map]
	if m == nil {
		return nil, 0, 0
	}
	return m.Region, p.X + m.OriginX, p.Y + m.OriginY
}

// Neighbor returns the tile one step from p in direction d. Stepping off a
// map edge lands on the adjacent map of the same region. Warps are not
// followed. It returns false when the step leaves the region.
func (w *World) Neighbor(p Pos, d Dir) (Pos, bool) {
	r, rx, ry := w.ToRegion(p)
	if r == nil {
		return Pos{}, false
	}
	dx, dy := d.Delta()
	return r.At(rx+dx, ry+dy)
}

// DirBetween returns the direction from a to an adjacent tile b, taking map
// edges into account.
func (w *World) DirBetween(a, b Pos) (Dir, bool) {
	ra, ax, ay := w.ToRegion(a)
	rb, bx, by := w.ToRegion(b)
	if ra == nil || ra != rb {
		return Down, false
	}
	for _, d := range Dirs {
		dx, dy := d.Delta()
		if ax+dx == bx && ay+dy == by {
			return d, true
		}
	}
	return Down, false
}

// Solid reports whether p blocks movement (static tiles or blocking objects).
// Positions outside any map are solid.
func (w *World) Solid(p Pos) bool {
	m := w.Maps[p.Map]
	if m == nil || !m.in(p.X, p.Y) {
		return true
	}
	i := p.Y*m.W + p.X
	return m.solid[i] || m.blocked[i] > 0
}

// SetBlocked adds or removes a dynamic blocker at p (doors, pushed blocks).
func (w *World) SetBlocked(p Pos, blocked bool) {
	m := w.Maps[p.Map]
	if m == nil || !m.in(p.X, p.Y) {
		return
	}
	i := p.Y*m.W + p.X
	if blocked {
		m.blocked[i]++
	} else if m.blocked[i] > 0 {
		m.blocked[i]--
	}
	w.version++
}

// WarpAt returns the warp whose trigger tile is p, or nil.
func (w *World) WarpAt(p Pos) *Warp {
	m := w.Maps[p.Map]
	if m == nil {
		return nil
	}
	return m.warps[[2]int{p.X, p.Y}]
}

// Load reads every *.tmj map and *.world file in dir.
func Load(fsys fs.FS, dir string) (*World, error) {
	w := &World{Maps: map[MapID]*Map{}, Regions: map[string]*Region{}}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	tilesets := map[string]*Tileset{}
	var worldFiles []string
	for _, e := range entries {
		name := path.Join(dir, e.Name())
		switch path.Ext(name) {
		case ".tmj":
			m, err := loadMap(fsys, name, tilesets)
			if err != nil {
				return nil, err
			}
			w.Maps[m.ID] = m
		case ".world":
			worldFiles = append(worldFiles, name)
		}
	}
	if len(w.Maps) == 0 {
		return nil, fmt.Errorf("no maps found in %s", dir)
	}
	for _, name := range worldFiles {
		if err := w.loadRegion(fsys, name); err != nil {
			return nil, err
		}
	}
	// Every map outside a .world file is a region of its own.
	for _, m := range w.Maps {
		if m.Region == nil {
			if err := w.addRegion(string(m.ID), []*Map{m}); err != nil {
				return nil, err
			}
		}
	}
	if err := w.resolveObjects(); err != nil {
		return nil, err
	}
	return w, nil
}

func loadMap(fsys fs.FS, name string, cache map[string]*Tileset) (*Map, error) {
	var tm tiledMap
	if err := readJSON(fsys, name, &tm); err != nil {
		return nil, err
	}
	if tm.Infinite {
		return nil, fmt.Errorf("%s: infinite maps are not supported", name)
	}
	if tm.TileWidth != TileSize || tm.TileHeight != TileSize {
		return nil, fmt.Errorf("%s: tiles must be %dx%d", name, TileSize, TileSize)
	}
	m := &Map{
		ID:      MapID(strings.TrimSuffix(path.Base(name), path.Ext(name))),
		W:       tm.Width,
		H:       tm.Height,
		Props:   toProps(tm.Properties),
		solid:   make([]bool, tm.Width*tm.Height),
		blocked: make([]int, tm.Width*tm.Height),
		warps:   map[[2]int]*Warp{},
		spawns:  map[string]Spawn{},
	}

	type ref struct {
		first uint32
		ts    *Tileset
	}
	var refs []ref
	for _, r := range tm.Tilesets {
		if r.Source == "" {
			return nil, fmt.Errorf("%s: embedded tilesets are not supported, export them as .tsj", name)
		}
		p := path.Join(path.Dir(name), r.Source)
		ts := cache[p]
		if ts == nil {
			var err error
			if ts, err = loadTileset(fsys, p); err != nil {
				return nil, err
			}
			cache[p] = ts
		}
		refs = append(refs, ref{r.FirstGID, ts})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].first < refs[j].first })
	resolve := func(gid uint32) (Tile, error) {
		id := gid & gidMask
		if id == 0 {
			return Tile{}, nil
		}
		for i := len(refs) - 1; i >= 0; i-- {
			if id >= refs[i].first {
				idx := int(id - refs[i].first)
				if idx >= refs[i].ts.TileCount {
					break
				}
				return Tile{Set: refs[i].ts, Index: idx, FlipH: gid&gidFlipH != 0, FlipV: gid&gidFlipV != 0}, nil
			}
		}
		return Tile{}, fmt.Errorf("%s: unknown gid %d", name, id)
	}

	var walk func(ls []tiledLayer) error
	walk = func(ls []tiledLayer) error {
		for _, l := range ls {
			switch l.Type {
			case "group":
				if err := walk(l.Layers); err != nil {
					return err
				}
			case "tilelayer":
				if len(l.Data) != m.W*m.H {
					return fmt.Errorf("%s: layer %q must use uncompressed CSV data of the map's size", name, l.Name)
				}
				props := toProps(l.Properties)
				layer := Layer{Name: l.Name, Overhead: props.Bool("overhead"), Tiles: make([]Tile, len(l.Data))}
				for i, gid := range l.Data {
					t, err := resolve(gid)
					if err != nil {
						return err
					}
					layer.Tiles[i] = t
					if !layer.Overhead && !t.Empty() && t.Set.Solid(t.Index) {
						m.solid[i] = true
					}
				}
				m.Layers = append(m.Layers, layer)
			case "objectgroup":
				for _, o := range l.Objects {
					obj, err := convertObject(o, resolve)
					if err != nil {
						return fmt.Errorf("%s: object %d: %w", name, o.ID, err)
					}
					m.Objects = append(m.Objects, obj)
				}
			}
		}
		return nil
	}
	if err := walk(tm.Layers); err != nil {
		return nil, err
	}
	return m, nil
}

func convertObject(o tiledObject, resolve func(uint32) (Tile, error)) (Object, error) {
	obj := Object{ID: o.ID, Name: o.Name, Type: o.Type, Props: toProps(o.Properties)}
	if obj.Type == "" {
		obj.Type = o.Class
	}
	x, y, w, h := o.X, o.Y, o.Width, o.Height
	if o.GID != 0 {
		t, err := resolve(o.GID)
		if err != nil {
			return obj, err
		}
		obj.Tile = t
		y -= h // tile objects are anchored at their bottom-left corner
	}
	obj.X = int(math.Floor(x / TileSize))
	obj.Y = int(math.Floor(y / TileSize))
	obj.W = max(1, int(math.Ceil((x+w)/TileSize))-obj.X)
	obj.H = max(1, int(math.Ceil((y+h)/TileSize))-obj.Y)
	return obj, nil
}

func (w *World) loadRegion(fsys fs.FS, name string) error {
	var tw tiledWorld
	if err := readJSON(fsys, name, &tw); err != nil {
		return err
	}
	var maps []*Map
	for _, e := range tw.Maps {
		id := MapID(strings.TrimSuffix(path.Base(e.FileName), path.Ext(e.FileName)))
		m := w.Maps[id]
		if m == nil {
			return fmt.Errorf("%s: unknown map %q", name, e.FileName)
		}
		if m.Region != nil {
			return fmt.Errorf("%s: map %q is already part of region %q", name, id, m.Region.ID)
		}
		if e.X%TileSize != 0 || e.Y%TileSize != 0 {
			return fmt.Errorf("%s: map %q is not aligned to the tile grid", name, id)
		}
		m.OriginX, m.OriginY = e.X/TileSize, e.Y/TileSize
		maps = append(maps, m)
	}
	return w.addRegion(strings.TrimSuffix(path.Base(name), path.Ext(name)), maps)
}

func (w *World) addRegion(id string, maps []*Map) error {
	if _, dup := w.Regions[id]; dup {
		return fmt.Errorf("duplicate region %q", id)
	}
	r := &Region{ID: id, Maps: maps}
	minX, minY, maxX, maxY := math.MaxInt, math.MaxInt, math.MinInt, math.MinInt
	for _, m := range maps {
		minX, minY = min(minX, m.OriginX), min(minY, m.OriginY)
		maxX, maxY = max(maxX, m.OriginX+m.W), max(maxY, m.OriginY+m.H)
	}
	r.MinX, r.MinY, r.W, r.H = minX, minY, maxX-minX, maxY-minY
	r.cells = make([]int16, r.W*r.H)
	for i := range r.cells {
		r.cells[i] = -1
	}
	for i, m := range maps {
		m.Region = r
		for y := 0; y < m.H; y++ {
			for x := 0; x < m.W; x++ {
				c := (m.OriginY+y-r.MinY)*r.W + (m.OriginX + x - r.MinX)
				if r.cells[c] >= 0 {
					return fmt.Errorf("region %q: maps %q and %q overlap", id, maps[r.cells[c]].ID, m.ID)
				}
				r.cells[c] = int16(i)
			}
		}
	}
	w.Regions[id] = r
	return nil
}

// resolveObjects processes spawns, warps and blocking objects once all maps
// are known, and validates cross-map references.
func (w *World) resolveObjects() error {
	ids := make([]MapID, 0, len(w.Maps))
	for id := range w.Maps {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		m := w.Maps[id]
		for _, o := range m.Objects {
			if o.Type != "spawn" {
				continue
			}
			if o.Name == "" {
				return fmt.Errorf("map %s: spawn object %d has no name", id, o.ID)
			}
			if _, dup := m.spawns[o.Name]; dup {
				return fmt.Errorf("map %s: duplicate spawn %q", id, o.Name)
			}
			s := Spawn{Pos: Pos{Map: id, X: o.X, Y: o.Y}}
			s.Facing, s.HasFacing = ParseDir(o.Props.String("facing"))
			m.spawns[o.Name] = s
		}
	}
	var errs []error
	for _, id := range ids {
		m := w.Maps[id]
		for _, o := range m.Objects {
			if o.Props.Bool("solid") {
				for y := o.Y; y < o.Y+o.H; y++ {
					for x := o.X; x < o.X+o.W; x++ {
						if m.in(x, y) {
							m.blocked[y*m.W+x]++
						}
					}
				}
			}
			if o.Type != "warp" {
				continue
			}
			target := w.Maps[MapID(o.Props.String("to_map"))]
			if target == nil {
				errs = append(errs, fmt.Errorf("map %s: warp %d targets unknown map %q", id, o.ID, o.Props.String("to_map")))
				continue
			}
			spawn, ok := target.spawns[o.Props.String("to_spawn")]
			if !ok {
				errs = append(errs, fmt.Errorf("map %s: warp %d targets unknown spawn %q on %s", id, o.ID, o.Props.String("to_spawn"), target.ID))
				continue
			}
			for y := o.Y; y < o.Y+o.H; y++ {
				for x := o.X; x < o.X+o.W; x++ {
					if !m.in(x, y) {
						continue
					}
					wp := &Warp{From: Pos{id, x, y}, To: spawn.Pos, Facing: spawn.Facing, HasFacing: spawn.HasFacing}
					m.warps[[2]int{x, y}] = wp
					w.Warps = append(w.Warps, wp)
				}
			}
		}
	}
	for _, wp := range w.Warps {
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
