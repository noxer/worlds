package gen

import (
	"fmt"
	"sync/atomic"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/world"
)

// Earth generates the main map: oceans and continents from a tileable
// elevation field, coastlines drawn with Kenney's shore tiles, deep water with
// the project's deep-to-shallow transitions, and grassland (grass, flowers,
// trees) on land. Hand-made patches are constraints: their painted terrain is
// fixed, and patches can require land or ocean around themselves
// ("surround"), which also lets the world place pool islands in the open sea.
//
// Generation runs in levels:
//   - world level: elevation from the coarse octaves of the noise field,
//     evaluable anywhere in O(1);
//   - area level: pool patches placed by the world (see world.Placer);
//   - chunk level: per-cell terrain classes, cleaned up so that every
//     configuration has art, then tiles and decoration.
type Earth struct {
	elevation *Field
	land      *Grassland
	shore     map[string]world.Tile // Kenney Tiny Battle water tiles, by name
	deep      map[string]world.Tile // deep water and transitions, by name
	infra     *infra.Infra          // settlements and networks; nil for odd world sizes
	dirt      [16]world.Tile        // dirt path tiles by grass edges (see dirtTile)
	dirtInner [4]world.Tile         // dirt with grass in one corner: nw, ne, se, sw
	cobble    world.Tile            // road
	prefabs   []*world.Prefab       // by the index of infra.Building.Prefab
	lamp      [2]world.Tile         // lamp post and lantern (the tile above)
}

// Sea levels on the elevation scale [0, MaxValue).
const (
	SeaLevel  = 32300 // below: water
	DeepLevel = 32000 // below (and away from land): deep water
)

// margin is how far beyond its chunk the generator looks: the cleanup
// passes and the autotiler each need one more ring of neighbours.
const margin = 6

// fallbacks counts water cells whose neighbourhood has no exact tile; tests
// keep it rare.
var fallbacks atomic.Int64

// NewEarth creates the earth generator.
func NewEarth(info world.GenInfo) (world.Generator, error) {
	elev, err := NewField(info.W, info.H, 0x656c6576,
		Octave{Cell: 131072, Amp: 64}, // continents and oceans
		Octave{Cell: 32768, Amp: 32},
		Octave{Cell: 8192, Amp: 16}, // seas, peninsulas
		Octave{Cell: 2048, Amp: 8},
		Octave{Cell: 512, Amp: 5}, // bays, lakes
		Octave{Cell: 128, Amp: 3},
		Octave{Cell: 32, Amp: 2}, // coastline detail
		Octave{Cell: 8, Amp: 1},
	)
	if err != nil {
		return nil, fmt.Errorf("earth: %w", err)
	}
	land, err := NewGrassland(info)
	if err != nil {
		return nil, err
	}
	e := &Earth{elevation: elev, land: land.(*Grassland), shore: map[string]world.Tile{}, deep: map[string]world.Tile{}}
	for _, n := range []int{18, 19, 20, 36, 37, 38, 54, 55, 56, 39, 57, 72, 73, 74, 75, 90, 91, 92, 93} {
		name := fmt.Sprintf("battle %d", n)
		if e.shore[name], err = info.Tilesets.Named("water", name); err != nil {
			return nil, fmt.Errorf("earth: %w", err)
		}
	}
	for _, n := range []string{"deep", "deep n", "deep s", "deep w", "deep e", "deep nw", "deep ne", "deep sw", "deep se",
		"deep inner nw", "deep inner ne", "deep inner sw", "deep inner se"} {
		if e.deep[n], err = info.Tilesets.Named("water", n); err != nil {
			return nil, fmt.Errorf("earth: %w", err)
		}
	}
	// Tiny Town's dirt: the index of the tile by which sides have grass
	// (bits n=1, e=2, s=4, w=8), and the inner corners.
	town := func(i int) world.Tile {
		t, e2 := info.Tilesets.Tile("tinytown", i)
		if e2 != nil && err == nil {
			err = e2
		}
		return t
	}
	for mask, i := range map[int]int{0: 25, 1: 13, 2: 26, 4: 37, 8: 24, 1 | 8: 12, 1 | 2: 14, 4 | 8: 36, 4 | 2: 38} {
		e.dirt[mask] = town(i)
	}
	e.dirtInner = [4]world.Tile{town(39), town(40), town(41), town(42)}
	e.cobble = town(43)
	if err != nil {
		return nil, fmt.Errorf("earth: %w", err)
	}
	if t, err2 := info.Tilesets.Named("worlds", "lamp_post"); err2 == nil {
		top, _ := info.Tilesets.Above(t)
		e.lamp = [2]world.Tile{t, top}
	}
	specs, err := prefabSpecs(info.Prefabs)
	if err != nil {
		return nil, err
	}
	e.prefabs = info.Prefabs
	if infra.Supports(info.W, info.H) {
		fixed, err := infra.FixedFrom(info.Patches)
		if err != nil {
			return nil, err
		}
		var avoid []infra.Rect
		for _, p := range info.Patches {
			avoid = append(avoid, infra.Rect{X: p.X, Y: p.Y, W: p.W, H: p.H})
		}
		if e.infra, err = infra.New(infra.Config{Seed: info.Seed, W: info.W, H: info.H, Relief: e, Fixed: fixed, Avoid: avoid, Prefabs: specs}); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// Prefetch implements world.Prefetcher: it prepares the paths and roads
// around the party in the background.
func (e *Earth) Prefetch(x, y int64) {
	if e.infra != nil {
		e.infra.Prefetch(x, y)
	}
}

// prefabSpecs describes the prefabs for village layouts, checking the
// conventions: the map property "kind" (house, garden or well), "weight",
// and for houses a "door" object in the row above the bottom row.
func prefabSpecs(ps []*world.Prefab) ([]infra.PrefabSpec, error) {
	var out []infra.PrefabSpec
	for _, p := range ps {
		s := infra.PrefabSpec{Name: string(p.Name), Kind: p.Props.String("kind"), W: p.W, H: p.H, Weight: p.Props.Int("weight", 1)}
		if s.Weight <= 0 {
			return nil, fmt.Errorf("prefab %s: weight must be positive", p.Name)
		}
		doors := 0
		for _, o := range p.Objects {
			if o.Type == "door" {
				doors++
				s.DoorX = o.Pos.X
				if o.Pos.Y != p.H-2 {
					return nil, fmt.Errorf("prefab %s: the door must be in the row above the bottom row", p.Name)
				}
			}
		}
		switch s.Kind {
		case "house":
			if doors != 1 {
				return nil, fmt.Errorf("prefab %s: a house needs exactly one door", p.Name)
			}
		case "garden", "well":
		default:
			return nil, fmt.Errorf("prefab %s: kind must be house, garden or well, not %q", p.Name, s.Kind)
		}
		out = append(out, s)
	}
	return out, nil
}

// Infra returns the world's settlements and networks (nil if the world
// size doesn't allow them).
func (e *Earth) Infra() *infra.Infra { return e.infra }

// Terrain classes during generation.
const (
	cLand uint8 = iota
	cWater
	cDeep
)

// region is the chunk plus a margin, in which classes are computed.
type region struct {
	x0, y0 int64 // main-map position of cell (0, 0)
	n      int   // edge length
	class  []uint8
	fixed  []bool  // painted by a patch: never changed by cleanup
	raw    []bool  // elevation (or surroundings) asks for deep water
	way    []uint8 // 0, or 1 + the network of a path or road on the cell
	site   []infra.SiteCell
	sites  []*infra.Site // villages overlapping the region
}

// siteAt is the village layout's kind of a cell (Outside if none).
func (r *region) siteAt(x, y int) infra.SiteCell {
	x, y = min(max(x, 0), r.n-1), min(max(y, 0), r.n-1)
	return r.site[y*r.n+x]
}

func (r *region) wayAt(x, y int) bool {
	x, y = min(max(x, 0), r.n-1), min(max(y, 0), r.n-1)
	return r.way[y*r.n+x] != 0
}

func (r *region) at(x, y int) uint8 {
	x, y = min(max(x, 0), r.n-1), min(max(y, 0), r.n-1)
	return r.class[y*r.n+x]
}

// Generate implements world.Generator.
func (e *Earth) Generate(seed uint64, b *world.ChunkBuilder) {
	r := e.classify(seed, b)
	ox, oy := margin, margin
	for ly := 0; ly < world.ChunkSize; ly++ {
		for lx := 0; lx < world.ChunkSize; lx++ {
			x, y := ox+lx, oy+ly
			wx, wy := b.Wrap(r.x0+int64(x), r.y0+int64(y))
			way := r.way[y*r.n+x]
			switch r.at(x, y) {
			case cLand:
				switch way {
				case 0:
					e.land.ground(seed, b, lx, ly, wx, wy)
				case uint8(infra.Road) + 1:
					b.Set(world.LayerGround, lx, ly, e.cobble)
				default:
					b.Set(world.LayerGround, lx, ly, e.dirtTile(r, x, y))
				}
			case cWater:
				b.Set(world.LayerGround, lx, ly, e.shoreTile(r, x, y))
			case cDeep:
				b.Set(world.LayerGround, lx, ly, e.deepTile(r, x, y))
			}
			// Trees grow on land, off the ways and village buildings, and
			// sparsely in villages; a tall one needs free land above it too.
			free := func(dy int) bool {
				if r.at(x, y+dy) != cLand || r.wayAt(x, y+dy) {
					return false
				}
				c := r.siteAt(x, y+dy)
				return c == infra.Outside || c == infra.Open || c == infra.Water
			}
			grows := func(dy int) bool {
				if !free(dy) {
					return false
				}
				if r.siteAt(x, y+dy) != infra.Open {
					return true
				}
				tx, ty := b.Wrap(wx, wy+int64(dy))
				return Hash(seed^0x76696c6c, tx, ty)%4 == 0 // "vill": a quarter of the trees
			}
			if grows(0) {
				if bottom, _, ok := e.land.treeAt(seed, b, wx, wy, free(-1)); ok {
					b.Set(world.LayerDeco, lx, ly, bottom)
				}
			}
			if free(0) && grows(1) {
				bx, by := b.Wrap(wx, wy+1)
				if _, top, ok := e.land.treeAt(seed, b, bx, by, true); ok && !top.Empty() {
					b.Set(world.LayerOverhead, lx, ly, top)
				}
			}
		}
	}
	e.buildings(b, r)
}

// buildings puts the villages' prefabs, doors and lamps into the chunk.
func (e *Earth) buildings(b *world.ChunkBuilder, r *region) {
	cx, cy := b.CX*world.ChunkSize, b.CY*world.ChunkSize
	// local converts a main-map position to chunk coordinates.
	local := func(x, y int64) (int, int, bool) {
		lx, ly := geom.WrapDelta(cx, x, b.W()), geom.WrapDelta(cy, y, b.H())
		return int(lx), int(ly), lx >= 0 && ly >= 0 && lx < world.ChunkSize && ly < world.ChunkSize
	}
	for _, s := range r.sites {
		for _, bd := range s.Buildings {
			p := e.prefabs[bd.Prefab]
			for py := int64(0); py < p.H; py++ {
				for px := int64(0); px < p.W; px++ {
					lx, ly, ok := local(bd.X+px, bd.Y+py)
					if !ok {
						continue
					}
					for l := 0; l < world.NumLayers; l++ {
						if t := p.Tile(l, px, py); !t.Empty() {
							b.Set(l, lx, ly, t)
						}
					}
				}
			}
			for _, o := range p.Objects {
				lx, ly, ok := local(bd.X+o.Pos.X, bd.Y+o.Pos.Y)
				if !ok {
					continue
				}
				if o.Props.Bool("solid") {
					b.Block(lx, ly)
				}
				if d := o.Props.String("bump"); d != "" {
					b.Bump(lx, ly, d)
				}
			}
		}
		if e.lamp[0].Empty() {
			continue
		}
		for _, l := range s.Lamps {
			if lx, ly, ok := local(l.X, l.Y); ok {
				b.Set(world.LayerDeco, lx, ly, e.lamp[0])
			}
			if lx, ly, ok := local(l.X, l.Y-1); ok {
				b.Set(world.LayerOverhead, lx, ly, e.lamp[1])
			}
		}
	}
}

// classify computes the cleaned-up terrain classes of the chunk and its
// margin.
func (e *Earth) classify(seed uint64, b *world.ChunkBuilder) *region {
	n := world.ChunkSize + 2*margin
	r := &region{
		x0: b.CX*world.ChunkSize - margin, y0: b.CY*world.ChunkSize - margin, n: n,
		class: make([]uint8, n*n), fixed: make([]bool, n*n), raw: make([]bool, n*n), way: make([]uint8, n*n),
		site: make([]infra.SiteCell, n*n),
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := y*n + x
			wx, wy := b.Wrap(r.x0+int64(x), r.y0+int64(y))
			if t, ok := b.PaintedTerrain(wx, wy); ok {
				r.fixed[i] = true
				switch t {
				case world.Deep:
					r.class[i] = cDeep
				case world.Shallow:
					r.class[i] = cWater
				default:
					r.class[i] = cLand
				}
				continue
			}
			h := e.elevation.At(seed, wx, wy)
			if kind, _, ok := b.Surround(wx, wy); ok {
				// The patch decides land or water; the elevation still decides
				// how deep the water is, so an island in the shallows can be
				// reached by swimming and one at sea cannot.
				if kind == world.SurroundLand {
					r.class[i] = cLand
				} else {
					r.class[i], r.raw[i] = cWater, h < DeepLevel
				}
				continue
			}
			switch {
			case h < DeepLevel:
				r.class[i], r.raw[i] = cWater, true
			case h < SeaLevel:
				r.class[i] = cWater
			default:
				r.class[i] = cLand
			}
		}
	}
	// 0. Paths and roads, on land; through shallow water as causeways.
	if e.infra != nil {
		e.infra.Paint(r.x0, r.y0, n, func(i int, net infra.Network) {
			if !r.fixed[i] && !r.raw[i] {
				r.way[i] = uint8(net) + 1
				r.class[i] = cLand
			}
		})
	}
	// Villages: the square and footpaths are cobbles; buildings and their
	// yards need land.
	if e.infra != nil {
		r.sites = e.infra.SitesIn(r.x0, r.y0, int64(n), int64(n))
		for _, s := range r.sites {
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					i := y*n + x
					wx, wy := b.Wrap(r.x0+int64(x), r.y0+int64(y))
					dx, dy := geom.WrapDelta(s.X0, wx, b.W()), geom.WrapDelta(s.Y0, wy, b.H())
					c := s.At(dx, dy)
					if c == infra.Outside || r.fixed[i] {
						continue
					}
					r.site[i] = c
					switch c {
					case infra.Square, infra.Foot:
						r.way[i] = uint8(infra.Road) + 1
						r.class[i] = cLand
					case infra.Lane:
						if r.way[i] == 0 {
							r.way[i] = uint8(infra.Path) + 1
						}
						r.class[i] = cLand
					case infra.Built, infra.Yard:
						r.class[i] = cLand
					}
				}
			}
		}
	}
	// 1. Water must come in 2x2 blocks: no one-tile channels or ponds.
	r.open(func(c uint8) bool { return c != cLand }, cLand)
	// 2. Land likewise: no one-tile spits or islets.
	r.open(func(c uint8) bool { return c == cLand }, cWater)
	// 3. Deep water only where all eight neighbours are water, so there is
	// always shallow water between land and deep water.
	next := append([]uint8(nil), r.class...)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := y*n + x
			if r.fixed[i] || r.class[i] == cLand || !r.raw[i] {
				continue
			}
			ok := true
			for dy := -1; dy <= 1 && ok; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if r.at(x+dx, y+dy) == cLand {
						ok = false
						break
					}
				}
			}
			if ok {
				next[i] = cDeep
			}
		}
	}
	r.class = next
	// 4. Deep water in 2x2 blocks too.
	r.open(func(c uint8) bool { return c == cDeep }, cWater)
	// 5. Ways only on land, and in 2x2 blocks, which the dirt art needs.
	for i := range r.way {
		if r.class[i] != cLand {
			r.way[i] = 0
		}
	}
	r.openWays()
	return r
}

// openWays keeps a way cell only if some 2x2 block containing it is all
// way, like open does for terrain.
func (r *region) openWays() {
	n := r.n
	next := append([]uint8(nil), r.way...)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if w := r.way[y*n+x]; w == 0 || w == uint8(infra.Road)+1 {
				continue // cobbles need no 2x2 blocks
			}
			keep := false
			for _, d := range [4][2]int{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
				bx, by := x+d[0], y+d[1]
				if r.wayAt(bx, by) && r.wayAt(bx+1, by) && r.wayAt(bx, by+1) && r.wayAt(bx+1, by+1) {
					keep = true
					break
				}
			}
			if !keep {
				next[y*n+x] = 0
			}
		}
	}
	r.way = next
}

// dirtTile picks the dirt tile for a path cell from which neighbours are
// ways too.
func (e *Earth) dirtTile(r *region, x, y int) world.Tile {
	mask := 0
	for bit, d := range [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		if !r.wayAt(x+d[0], y+d[1]) {
			mask |= 1 << bit
		}
	}
	if mask == 0 {
		// Grass in a single corner: an inner corner.
		for k, d := range [4][2]int{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			if !r.wayAt(x+d[0], y+d[1]) {
				return e.dirtInner[k]
			}
		}
	}
	if t := e.dirt[mask]; !t.Empty() {
		return t
	}
	return e.dirt[0] // no art for this shape (the 2x2 rule makes it rare)
}

// open keeps a non-fixed cell matching `in` only if some 2x2 block containing
// it matches entirely; otherwise the cell becomes `out`.
func (r *region) open(in func(uint8) bool, out uint8) {
	n := r.n
	next := append([]uint8(nil), r.class...)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := y*n + x
			if r.fixed[i] || !in(r.class[i]) {
				continue
			}
			keep := false
			for _, d := range [4][2]int{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
				bx, by := x+d[0], y+d[1]
				if in(r.at(bx, by)) && in(r.at(bx+1, by)) && in(r.at(bx, by+1)) && in(r.at(bx+1, by+1)) {
					keep = true
					break
				}
			}
			if !keep {
				next[i] = out
			}
		}
	}
	r.class = next
}

// neighbours reports which of the 8 neighbours match: orthogonal N, E, S, W
// and diagonal NE, SE, SW, NW (the diagonals only where no orthogonal
// neighbour next to them matches, since the edge art already covers those).
type neighbours struct{ n, e, s, w, ne, se, sw, nw bool }

func (r *region) neighbours(x, y int, match func(uint8) bool) neighbours {
	m := func(dx, dy int) bool { return match(r.at(x+dx, y+dy)) }
	nb := neighbours{n: m(0, -1), e: m(1, 0), s: m(0, 1), w: m(-1, 0)}
	nb.ne = m(1, -1) && !nb.n && !nb.e
	nb.se = m(1, 1) && !nb.s && !nb.e
	nb.sw = m(-1, 1) && !nb.s && !nb.w
	nb.nw = m(-1, -1) && !nb.n && !nb.w
	return nb
}

func (nb neighbours) orth() int {
	c := 0
	for _, b := range []bool{nb.n, nb.e, nb.s, nb.w} {
		if b {
			c++
		}
	}
	return c
}

// shoreTile picks Kenney's water tile for a shallow cell from where the
// land is.
func (e *Earth) shoreTile(r *region, x, y int) world.Tile {
	nb := r.neighbours(x, y, func(c uint8) bool { return c == cLand })
	t := func(n int) world.Tile { return e.shore[fmt.Sprintf("battle %d", n)] }
	switch nb.orth() {
	case 0:
		switch nb {
		case neighbours{}:
			return t(37)
		case neighbours{nw: true}:
			return t(90)
		case neighbours{ne: true}:
			return t(91)
		case neighbours{se: true}:
			return t(92)
		case neighbours{sw: true}:
			return t(93)
		case neighbours{nw: true, ne: true}:
			return t(75)
		case neighbours{sw: true, se: true}:
			return t(39)
		case neighbours{ne: true, se: true}:
			return t(72)
		case neighbours{nw: true, sw: true}:
			return t(74)
		}
	case 1:
		switch {
		case nb.n && nb.sw && nb.se:
			return t(73)
		case nb.n && !nb.sw && !nb.se:
			return t(19)
		case nb.s && !nb.nw && !nb.ne:
			return t(55)
		case nb.e && !nb.nw && !nb.sw:
			return t(38)
		case nb.w && !nb.ne && !nb.se:
			return t(36)
		}
	case 2:
		switch {
		case nb.n && nb.w && !nb.se:
			return t(18)
		case nb.n && nb.e && !nb.sw:
			return t(20)
		case nb.s && nb.w && !nb.ne:
			return t(54)
		case nb.s && nb.e && !nb.nw:
			return t(56)
		case nb.e && nb.w:
			return t(57)
		}
	}
	return e.fallbackShore(nb)
}

// fallbackShore handles neighbourhoods without exact art (rare after the
// cleanup): it drops diagonals, then picks the closest edge or corner.
func (e *Earth) fallbackShore(nb neighbours) world.Tile {
	fallbacks.Add(1)
	t := func(n int) world.Tile { return e.shore[fmt.Sprintf("battle %d", n)] }
	switch {
	case nb.n && nb.w:
		return t(18)
	case nb.n && nb.e:
		return t(20)
	case nb.s && nb.w:
		return t(54)
	case nb.s && nb.e:
		return t(56)
	case nb.n:
		return t(19)
	case nb.s:
		return t(55)
	case nb.e:
		return t(38)
	case nb.w:
		return t(36)
	case nb.nw:
		return t(90)
	case nb.ne:
		return t(91)
	case nb.se:
		return t(92)
	case nb.sw:
		return t(93)
	}
	return t(37)
}

// deepTile picks the deep-water tile for a deep cell from where the shallow
// water is.
func (e *Earth) deepTile(r *region, x, y int) world.Tile {
	nb := r.neighbours(x, y, func(c uint8) bool { return c != cDeep })
	name := ""
	switch nb.orth() {
	case 0:
		switch nb {
		case neighbours{}:
			name = "deep"
		case neighbours{nw: true}:
			name = "deep inner nw"
		case neighbours{ne: true}:
			name = "deep inner ne"
		case neighbours{se: true}:
			name = "deep inner se"
		case neighbours{sw: true}:
			name = "deep inner sw"
		}
	case 1:
		switch {
		case nb.n && !nb.sw && !nb.se:
			name = "deep n"
		case nb.s && !nb.nw && !nb.ne:
			name = "deep s"
		case nb.e && !nb.nw && !nb.sw:
			name = "deep e"
		case nb.w && !nb.ne && !nb.se:
			name = "deep w"
		}
	case 2:
		switch {
		case nb.n && nb.w && !nb.se:
			name = "deep nw"
		case nb.n && nb.e && !nb.sw:
			name = "deep ne"
		case nb.s && nb.w && !nb.ne:
			name = "deep sw"
		case nb.s && nb.e && !nb.nw:
			name = "deep se"
		}
	}
	if name == "" {
		fallbacks.Add(1)
		switch {
		case nb.n && nb.w:
			name = "deep nw"
		case nb.n && nb.e:
			name = "deep ne"
		case nb.s && nb.w:
			name = "deep sw"
		case nb.s && nb.e:
			name = "deep se"
		case nb.n:
			name = "deep n"
		case nb.s:
			name = "deep s"
		case nb.e:
			name = "deep e"
		case nb.w:
			name = "deep w"
		case nb.nw:
			name = "deep inner nw"
		case nb.ne:
			name = "deep inner ne"
		case nb.se:
			name = "deep inner se"
		case nb.sw:
			name = "deep inner sw"
		default:
			name = "deep"
		}
	}
	return e.deep[name]
}

// Suitable implements world.Placer: the rectangle plus margin must be deep
// open ocean according to the elevation alone.
func (e *Earth) Suitable(seed uint64, x, y, w, h, margin int64) bool {
	for dy := -margin - 2; dy <= h+margin+2; dy += 2 {
		for dx := -margin - 2; dx <= w+margin+2; dx += 2 {
			px, py := mod(x+dx, e.elevation.W), mod(y+dy, e.elevation.H)
			if e.elevation.At(seed, px, py) >= DeepLevel {
				return false
			}
		}
	}
	return true
}

func mod(a, n int64) int64 {
	m := a % n
	if m < 0 {
		m += n
	}
	return m
}

// Elevation returns the world-level elevation at a main-map position, in
// [0, MaxValue); see Levels for the water levels.
func (e *Earth) Elevation(seed uint64, x, y int64) int64 {
	return e.elevation.At(seed, mod(x, e.elevation.W), mod(y, e.elevation.H))
}

// Height returns the smooth large-scale height at a main-map position
// (hills and valleys, without the coastline detail), on the scale of
// Elevation. Infrastructure routes use it for slopes.
func (e *Earth) Height(seed uint64, x, y int64) int64 {
	return e.elevation.AtCoarse(seed, mod(x, e.elevation.W), mod(y, e.elevation.H), 2048)
}

// Levels returns the sea and deep-water levels of Elevation.
func (e *Earth) Levels() (sea, deep int64) { return SeaLevel, DeepLevel }

// Preview returns the terrain at a main-map position from the elevation
// alone (the world level of generation), without patches, cleanup or chunk
// generation. It is cheap enough for overview maps of the whole planet.
func (e *Earth) Preview(seed uint64, x, y int64) world.Terrain {
	h := e.elevation.At(seed, mod(x, e.elevation.W), mod(y, e.elevation.H))
	switch {
	case h < DeepLevel:
		return world.Deep
	case h < SeaLevel:
		return world.Shallow
	}
	return world.Land
}
