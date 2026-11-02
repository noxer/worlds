package infra

import (
	"sync"

	"github.com/noxer/worlds/internal/geom"
)

// Cities: towns, cities and metropolises are laid out on a grid of
// superblocks around their centre, evaluated locally: any tile can be asked
// what it is without laying out the whole city.
//
//   - Arterial roads (3 lanes wide, centre line, crosswalks next to the
//     junctions) run between superblocks every A tiles.
//   - A superblock belongs to the city if its centre is within the radius,
//     on dry land and away from hand-made patches.
//   - Its style follows the distance from the centre: downtown, residential
//     or suburb, some are parks, and the centre one is the station plaza
//     (cities) or the square (towns).
//   - Inside, minor streets (2 wide) cut it into blocks: a sidewalk round a
//     row of buildings that face the sidewalk below.

// CityKind is what a city tile is.
type CityKind uint8

const (
	CNone   CityKind = iota // not part of the city
	CRoad                   // asphalt
	CLineH                  // asphalt with the centre line of an east-west road
	CLineV                  // ... of a north-south road
	CCrossH                 // crosswalk across an east-west road (Lane: which of its 3 rows)
	CCrossV                 // crosswalk across a north-south road (Lane: which of its 3 columns)
	CWalk                   // sidewalk round a block (Edge: the sides that meet the road)
	CPaved                  // plaza, paved yard
	CGrass                  // lawn, park
	CBuilt                  // a building (its prefab is drawn there)
)

// City decorations.
const (
	DNone uint8 = iota
	DTree
	DLamp // the post; the lantern is drawn in the tile above
	DBench
)

// CityTile describes a tile of a city.
type CityTile struct {
	Kind CityKind
	Edge uint8 // CWalk: N=1, E=2, S=4, W=8 where the kerb is
	Lane uint8 // CCrossH/CCrossV: 0, 1 or 2
	Deco uint8
}

// Superblock styles.
const (
	sbOut uint8 = iota
	sbDowntown
	sbResidential
	sbSuburb
	sbPark
	sbStation // the centre of a city: plaza and station
	sbSquare  // the centre of a town: a square
)

const arterial = 3 // width of arterial roads

// CityPlan is the layout of one town or city.
type CityPlan struct {
	S      Settlement
	A      int64 // superblock period
	ox, oy int64 // top-left of superblock (0, 0), normalised
	in     *Infra

	mu  sync.Mutex
	sbs map[[2]int64]*superblock
}

type superblock struct {
	style uint8
	rails map[[2]int64]bool // rail cells, relative to the superblock's corner
	plots map[[3]int64][]plot
}

// plot is a building in a block, in interior coordinates of the block.
type plot struct {
	x, w, h int64
	prefab  int
}

// CitiesIn returns the cities whose grids overlap the rectangle of tiles.
func (in *Infra) CitiesIn(x, y, w, h int64) []*CityPlan {
	if len(in.prefabs) == 0 {
		return nil
	}
	r := tierRadius[NumTiers-1][1] + 2*cityPeriod(Metropolis)
	var out []*CityPlan
	for _, s := range in.InRect(x-r, y-r, w+2*r, h+2*r, Town) {
		if s.Fixed != "" {
			continue
		}
		c := in.city(s)
		reach := s.Radius + 2*c.A
		dx, dy := in.delta(s.X, s.Y, x, y) // the rectangle relative to the centre
		if dx < reach && dx+w > -reach && dy < reach && dy+h > -reach {
			out = append(out, c)
		}
	}
	return out
}

func cityPeriod(t Tier) int64 {
	switch t {
	case Metropolis:
		return 44
	case City:
		return 40
	}
	return 36
}

func (in *Infra) city(s Settlement) *CityPlan {
	in.mu.Lock()
	defer in.mu.Unlock()
	if c, ok := in.cities[s.ID]; ok {
		return c
	}
	a := cityPeriod(s.ID.Tier)
	c := &CityPlan{S: s, A: a, in: in, sbs: map[[2]int64]*superblock{},
		ox: geom.Mod(s.X-a/2, in.w), oy: geom.Mod(s.Y-a/2, in.h)}
	if len(in.cities) >= siteCacheLimit {
		clear(in.cities)
	}
	in.cities[s.ID] = c
	return c
}

// grid returns superblock coordinates and the offset within it of tile (x, y).
func (c *CityPlan) grid(x, y int64) (i, j, u, v int64) {
	dx, dy := c.in.delta(c.ox, c.oy, x, y)
	i, j = geom.FloorDiv(dx, c.A), geom.FloorDiv(dy, c.A)
	return i, j, dx - i*c.A, dy - j*c.A
}

// sb returns superblock (i, j), working it out on first use.
func (c *CityPlan) sb(i, j int64) *superblock {
	k := [2]int64{i, j}
	c.mu.Lock()
	s, ok := c.sbs[k]
	c.mu.Unlock()
	if ok {
		return s
	}
	s = c.newSuperblock(i, j)
	c.mu.Lock()
	c.sbs[k] = s
	c.mu.Unlock()
	return s
}

func (c *CityPlan) newSuperblock(i, j int64) *superblock {
	in := c.in
	s := &superblock{plots: map[[3]int64][]plot{}}
	x0, y0 := c.ox+i*c.A, c.oy+j*c.A
	mx, my := x0+c.A/2, y0+c.A/2
	dx, dy := in.delta(c.S.X, c.S.Y, mx, my)
	d2 := dx*dx + dy*dy
	if d2 > c.S.Radius*c.S.Radius {
		return s
	}
	// Dry land and away from hand-made patches, at the centre and corners.
	for _, p := range [5][2]int64{{c.A / 2, c.A / 2}, {1, 1}, {c.A - 1, 1}, {1, c.A - 1}, {c.A - 1, c.A - 1}} {
		x, y := geom.Mod(x0+p[0], in.w), geom.Mod(y0+p[1], in.h)
		if in.elev(x, y) < in.sea+48 || in.avoided(x, y, c.A/4) {
			return s
		}
	}
	h := hash(in.seed^salt, 0x63697479, int64(c.S.ID.Tier), c.S.ID.CX, c.S.ID.CY, i, j) // "city"
	d := 100 - isqrt(d2)*100/max(c.S.Radius, 1)                                         // density, 100 at the centre
	switch {
	case i == 0 && j == 0 && c.S.ID.Tier >= City:
		s.style = sbStation
	case i == 0 && j == 0:
		s.style = sbSquare
	case h%9 == 0:
		s.style = sbPark
	case d > 55 && c.S.ID.Tier >= City:
		s.style = sbDowntown
	case d > 25:
		s.style = sbResidential
	default:
		s.style = sbSuburb
	}
	// Rail lines through it: buildings keep off them.
	in.Paint(geom.Mod(x0, in.w), geom.Mod(y0, in.h), int(c.A), func(k int, n Network) {
		if n == Rail {
			if s.rails == nil {
				s.rails = map[[2]int64]bool{}
			}
			s.rails[[2]int64{int64(k) % c.A, int64(k) / c.A}] = true
		}
	})
	return s
}

// included reports whether superblock (i, j) is part of the city.
func (c *CityPlan) included(i, j int64) bool { return c.sb(i, j).style != sbOut }

// At returns what tile (x, y) is in the city.
func (c *CityPlan) At(x, y int64) CityTile {
	i, j, u, v := c.grid(x, y)
	vert, hor := u < arterial, v < arterial
	if vert || hor {
		switch {
		case vert && hor:
			if c.included(i, j) || c.included(i-1, j) || c.included(i, j-1) || c.included(i-1, j-1) {
				return CityTile{Kind: CRoad}
			}
		case vert:
			if c.included(i, j) || c.included(i-1, j) {
				if v == arterial || v == c.A-1 { // next to a junction
					return CityTile{Kind: CCrossV, Lane: uint8(u)}
				}
				if u == 1 {
					return CityTile{Kind: CLineV}
				}
				return CityTile{Kind: CRoad}
			}
		default:
			if c.included(i, j) || c.included(i, j-1) {
				if u == arterial || u == c.A-1 {
					return CityTile{Kind: CCrossH, Lane: uint8(v)}
				}
				if v == 1 {
					return CityTile{Kind: CLineH}
				}
				return CityTile{Kind: CRoad}
			}
		}
		return CityTile{}
	}
	sb := c.sb(i, j)
	p, q := u-arterial, v-arterial // interior coordinates
	S := c.A - arterial
	switch sb.style {
	case sbOut:
		return CityTile{}
	case sbPark:
		return c.park(i, j, p, q, S)
	case sbStation, sbSquare:
		return c.plaza(sb, p, q, S)
	}
	return c.blocks(i, j, sb, p, q, S)
}

// park: lawn with trees, two paved paths crossing in the middle, benches.
func (c *CityPlan) park(i, j, p, q, S int64) CityTile {
	m := S/2 - 1
	if p == m || p == m+1 || q == m || q == m+1 {
		t := CityTile{Kind: CPaved}
		if (p == m-1 || p == m+2) && q%6 == 3 {
			t.Kind, t.Deco = CGrass, DBench
		}
		return t
	}
	t := CityTile{Kind: CGrass}
	h := hash(c.in.seed^salt, 0x7061726b, c.S.ID.CX, c.S.ID.CY, i, j, p, q) // "park"
	if h%7 == 0 && p > 0 && q > 0 && p < S-1 && q < S-1 {
		t.Deco = DTree
	}
	return t
}

// plaza: the paved centre, with the station (cities) at its north side,
// lamps and trees.
func (c *CityPlan) plaza(sb *superblock, p, q, S int64) CityTile {
	t := CityTile{Kind: CPaved}
	if sb.style == sbStation {
		if st, ok := c.in.prefabStyle("station", S, S); ok {
			pf := c.in.prefabs[st]
			x0 := S/2 - pf.W/2
			if p >= x0 && p < x0+pf.W && q >= 1 && q < 1+pf.H {
				return CityTile{Kind: CBuilt}
			}
		}
	}
	// Rows of trees with benches beside them, lamps between, a clear
	// space in front of the station and along the middle.
	top := int64(3)
	if sb.style == sbStation {
		top = 11 // below the station and its forecourt
	}
	mx := S / 2
	switch {
	case q < top || q > S-3 || p < 2 || p > S-3 || (p >= mx-2 && p <= mx+1):
	case (p-2)%6 == 0 && (q-top)%6 == 0:
		t.Deco = DTree
	case (p-2)%6 == 1 && (q-top)%6 == 0:
		t.Deco = DBench
	case (p-2)%6 == 3 && (q-top)%6 == 3:
		t.Deco = DLamp
	}
	return t
}

// The rows of a superblock: block heights (sidewalk included) per style,
// minor streets 2 wide between them and one north-south in the middle.
func blockHeight(style uint8) int64 {
	switch style {
	case sbDowntown:
		return 10
	case sbResidential:
		return 9
	}
	return 8
}

const minor = 2 // width of minor streets

func (c *CityPlan) blocks(i, j int64, sb *superblock, p, q, S int64) CityTile {
	// Columns: [0, mid) and [mid+minor, S).
	mid := S/2 - minor/2
	var col, bx, bw int64
	switch {
	case p < mid:
		col, bx, bw = 0, 0, mid
	case p < mid+minor:
		return CityTile{Kind: CRoad}
	default:
		col, bx, bw = 1, mid+minor, S-mid-minor
	}
	// Rows: as many blocks as fit, the spare rows shared out from the top.
	hb := blockHeight(sb.style)
	n := max((S+minor)/(hb+minor), 1)
	spare := S + minor - n*(hb+minor)
	y := int64(0)
	for r := int64(0); r < n; r++ {
		h := hb + spare/n
		if r < spare%n {
			h++
		}
		if q < y+h {
			return c.block(i, j, sb, col, r, p-bx, q-y, bw, h)
		}
		y += h
		if q < y+minor {
			return CityTile{Kind: CRoad}
		}
		y += minor
	}
	return CityTile{Kind: CRoad}
}

// block: a sidewalk round the edge, buildings along the bottom of the
// inside facing the sidewalk below, a yard behind them.
func (c *CityPlan) block(i, j int64, sb *superblock, col, row, bu, bv, bw, bh int64) CityTile {
	var edge uint8
	if bv == 0 {
		edge |= 1
	}
	if bu == bw-1 {
		edge |= 2
	}
	if bv == bh-1 {
		edge |= 4
	}
	if bu == 0 {
		edge |= 8
	}
	if edge != 0 {
		t := CityTile{Kind: CWalk, Edge: edge}
		switch {
		case edge == 1|8 || edge == 1|2 || edge == 4|8 || edge == 4|2:
			if sb.style == sbDowntown {
				t.Deco = DLamp
			}
		case edge == 1 && sb.style == sbResidential && bu%5 == 2:
			t.Deco = DTree
		}
		return t
	}
	iu, iv := bu-1, bv-1
	iw, ih := bw-2, bh-2
	yard := CityTile{Kind: CGrass}
	if sb.style == sbDowntown {
		yard.Kind = CPaved
	}
	for _, pl := range c.plots(i, j, sb, col, row, iw, ih) {
		if iu >= pl.x && iu < pl.x+pl.w && iv >= ih-pl.h {
			return CityTile{Kind: CBuilt}
		}
	}
	return yard
}

// plots lays out the buildings of a block's row, from the left, cached.
func (c *CityPlan) plots(i, j int64, sb *superblock, col, row, iw, ih int64) []plot {
	k := [3]int64{col, row, 0}
	c.mu.Lock()
	ps, ok := sb.plots[k]
	c.mu.Unlock()
	if ok {
		return ps
	}
	style := map[uint8]string{sbDowntown: "downtown", sbResidential: "residential", sbSuburb: "suburb"}[sb.style]
	gap := map[uint8]int64{sbDowntown: 0, sbResidential: 1, sbSuburb: 2}[sb.style]
	r := &rng{s: hash(c.in.seed^salt, 0x706c6f74, c.S.ID.CX, c.S.ID.CY, i, j, col, row)} // "plot"
	// The block's corner relative to the superblock, for the rail check.
	mid := (c.A-arterial)/2 - minor/2
	bx := arterial + []int64{0, mid + minor}[col]
	by := arterial + c.rowTop(sb, row)
	var line []plot
	x := int64(0)
	for x < iw {
		pi, ok := c.in.pickStyle(r, style, iw-x, ih)
		if !ok {
			break
		}
		pf := c.in.prefabs[pi]
		line = append(line, plot{x: x, w: pf.W, h: pf.H, prefab: pi})
		x += pf.W + gap + r.intn(gap+1)
	}
	// Centre the row in the block, then leave out buildings on rail lines.
	if n := len(line); n > 0 {
		shift := (iw - (line[n-1].x + line[n-1].w)) / 2
		for _, pl := range line {
			pl.x += shift
			if c.railFree(sb, bx+1+pl.x, by+1+ih-pl.h, pl.w, pl.h) {
				ps = append(ps, pl)
			}
		}
	}
	c.mu.Lock()
	sb.plots[k] = ps
	c.mu.Unlock()
	return ps
}

// rowTop returns the first interior row of block row r of a superblock.
func (c *CityPlan) rowTop(sb *superblock, row int64) int64 {
	S := c.A - arterial
	hb := blockHeight(sb.style)
	n := max((S+minor)/(hb+minor), 1)
	spare := S + minor - n*(hb+minor)
	y := int64(0)
	for r := int64(0); r < row; r++ {
		h := hb + spare/n
		if r < spare%n {
			h++
		}
		y += h + minor
	}
	return y
}

func (c *CityPlan) railFree(sb *superblock, x, y, w, h int64) bool {
	for v := y; v < y+h; v++ {
		for u := x; u < x+w; u++ {
			if sb.rails[[2]int64{u, v}] {
				return false
			}
		}
	}
	return true
}

// Buildings returns the buildings of the city with a tile in the
// rectangle (x, y, w, h).
func (c *CityPlan) Buildings(x, y, w, h int64) []Building {
	i0, j0, _, _ := c.grid(x, y)
	i1, j1, _, _ := c.grid(x+w-1, y+h-1)
	var out []Building
	add := func(b Building, pw, ph int64) {
		dx, dy := c.in.delta(x, y, b.X, b.Y)
		if dx < w && dx+pw > 0 && dy < h && dy+ph > 0 {
			out = append(out, b)
		}
	}
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			sb := c.sb(i, j)
			x0, y0 := c.ox+i*c.A+arterial, c.oy+j*c.A+arterial
			S := c.A - arterial
			switch sb.style {
			case sbOut, sbPark, sbSquare:
				continue
			case sbStation:
				if st, ok := c.in.prefabStyle("station", S, S); ok {
					pf := c.in.prefabs[st]
					b := Building{Prefab: st, X: geom.Mod(x0+S/2-pf.W/2, c.in.w), Y: geom.Mod(y0+1, c.in.h)}
					add(b, pf.W, pf.H)
				}
				continue
			}
			mid := S/2 - minor/2
			hb := blockHeight(sb.style)
			n := max((S+minor)/(hb+minor), 1)
			spare := S + minor - n*(hb+minor)
			for col := int64(0); col < 2; col++ {
				bx, bw := int64(0), mid
				if col == 1 {
					bx, bw = mid+minor, S-mid-minor
				}
				for r := int64(0); r < n; r++ {
					bh := hb + spare/n
					if r < spare%n {
						bh++
					}
					top := c.rowTop(sb, r)
					iw, ih := bw-2, bh-2
					for _, pl := range c.plots(i, j, sb, col, r, iw, ih) {
						b := Building{Prefab: pl.prefab,
							X: geom.Mod(x0+bx+1+pl.x, c.in.w), Y: geom.Mod(y0+top+1+ih-pl.h, c.in.h)}
						add(b, pl.w, pl.h)
					}
				}
			}
		}
	}
	return out
}

// pickStyle picks a building of a style that fits w x h, by weight.
func (in *Infra) pickStyle(r *rng, style string, w, h int64) (int, bool) {
	total := 0
	for _, p := range in.prefabs {
		if p.Kind == "building" && p.Style == style && p.W <= w && p.H <= h {
			total += p.Weight
		}
	}
	if total == 0 {
		return 0, false
	}
	k := int(r.intn(int64(total)))
	for i, p := range in.prefabs {
		if p.Kind == "building" && p.Style == style && p.W <= w && p.H <= h {
			if k < p.Weight {
				return i, true
			}
			k -= p.Weight
		}
	}
	return 0, false
}

// prefabStyle returns the first building of a style that fits w x h.
func (in *Infra) prefabStyle(style string, w, h int64) (int, bool) {
	for i, p := range in.prefabs {
		if p.Kind == "building" && p.Style == style && p.W <= w && p.H <= h {
			return i, true
		}
	}
	return 0, false
}
