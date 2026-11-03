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
//   - Superblocks are grouped into districts (Voronoi regions round seeds on
//     a jittered grid of 3x3 superblocks), each with a character: the core,
//     an old quarter, a residential grid, a garden suburb, an industrial
//     estate or a green belt. The character and the distance from the
//     centre give each superblock its style and its block layout. The
//     centre one is the station plaza (cities) or the square (towns).
//   - Inside, minor streets (2 wide) cut a superblock into 1 to 3 columns
//     and rows of blocks: a sidewalk round a row of buildings that face the
//     sidewalk below.
//   - The outline is a circle with a noisy radius.

// CityKind is what a city tile is.
type CityKind uint8

const (
	CNone    CityKind = iota // not part of the city
	CRoad                    // plain asphalt: junctions, minor streets, yards
	CArtH                    // a lane of an east-west arterial (Lane: 0 north edge, 1 centre line, 2 south edge)
	CArtV                    // a lane of a north-south arterial (Lane: 0 west edge, 1 centre line, 2 east edge)
	CCrossH                  // crosswalk across an east-west road (Lane: which of its 3 rows)
	CCrossV                  // crosswalk across a north-south road (Lane: which of its 3 columns)
	CWalk                    // sidewalk round a block (Edge: the sides that meet the road)
	CPaved                   // plaza, paved yard
	CGrass                   // lawn, park
	CBuilt                   // a building (its prefab is drawn there)
	CParking                 // asphalt with a parking sign (industrial yards)
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
	style    uint8
	district uint8
	p        sbParams
	cols     []span            // block columns, in interior coordinates
	rows     []span            // block rows
	rails    map[[2]int64]bool // rail cells, relative to the superblock's corner
	plots    map[[3]int64][]plot
}

// span is a stretch of the interior: a column or row of blocks.
type span struct{ at, size int64 }

// District characters.
const (
	dCore uint8 = iota + 1
	dOld
	dGrid
	dGarden
	dIndustrial
	dGreen
)

// sbParams is how a superblock's blocks are built.
type sbParams struct {
	blockH    int64    // block height, sidewalk included
	cols      int64    // block columns
	gap       int64    // between buildings (plus up to as much at random)
	yard      CityKind // behind the buildings: CPaved or CGrass
	style     string   // prefab style
	material  string   // favoured material ("" for none)
	shops     int      // weight factor for shops
	lamps     bool     // lamps on block corners
	walkTrees bool     // trees on the sidewalk behind the buildings
	yardTrees bool     // trees in the yards
	parking   bool     // parking signs in the yard
}

func params(style, district uint8, h uint64) sbParams {
	p := sbParams{blockH: 9, cols: 2, gap: 1, yard: CGrass, style: "residential", shops: 1, walkTrees: true}
	switch style {
	case sbDowntown:
		p = sbParams{blockH: 10, cols: 2, yard: CPaved, style: "downtown", shops: 1, lamps: true}
	case sbSuburb:
		p = sbParams{blockH: 8, cols: 2, gap: 2, yard: CGrass, style: "suburb", shops: 1}
	}
	switch district {
	case dOld: // small dense blocks, many shops, red brick, cobbled feel
		p.blockH, p.cols, p.gap, p.yard, p.shops, p.material, p.lamps, p.walkTrees = 8, 3, 0, CPaved, 4, "red", true, false
		if style == sbDowntown {
			p.style = "residential" // the old quarter is low
		}
	case dGarden: // big plots, trees
		p.blockH, p.cols, p.gap, p.yard, p.yardTrees, p.walkTrees = 9, 1+int64(h%2), 3, CGrass, true, true
		p.style = "suburb"
	case dIndustrial: // wide blocks of warehouses round parking
		p = sbParams{blockH: 9, cols: 1, gap: 2, yard: CPaved, style: "industrial", material: "orange", shops: 1, parking: true}
	case dCore:
		p.material = []string{"", "orange", "red"}[h%3]
	}
	p.blockH += int64(h>>8%3) - 1 // a little variety within a district
	if p.style == "downtown" && p.blockH < 9 {
		p.blockH = 9 // downtown buildings need the height
	}
	return p
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
	if r := c.radiusTowards(dx, dy); d2 > r*r {
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
	d := c.density(d2)
	s.district = c.district(i, j)
	parkOdds := map[uint8]uint64{dCore: 14, dGreen: 3, dIndustrial: 20}[s.district]
	if parkOdds == 0 {
		parkOdds = 9
	}
	switch {
	case i == 0 && j == 0 && c.S.ID.Tier >= City:
		s.style = sbStation
	case i == 0 && j == 0:
		s.style = sbSquare
	case h%parkOdds == 0:
		s.style = sbPark
	case (d > 55 || s.district == dCore) && c.S.ID.Tier >= City && s.district != dGarden && d > 35:
		s.style = sbDowntown
	case d > 25 && s.district != dGarden:
		s.style = sbResidential
	default:
		s.style = sbSuburb
	}
	s.p = params(s.style, s.district, h>>16)
	S := c.A - arterial
	s.cols = spans(S, s.p.cols, s.p.cols*12, minor)
	s.rows = spans(S, max((S+minor)/(s.p.blockH+minor), 1), 0, minor)
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

// density is 100 at the centre and 0 at the radius.
func (c *CityPlan) density(d2 int64) int64 {
	return 100 - isqrt(d2)*100/max(c.S.Radius, 1)
}

// radiusTowards is the city's radius in the direction (dx, dy): 78-100% of
// its radius, varying smoothly round the city (16 sectors), so the outline
// isn't a perfect circle.
func (c *CityPlan) radiusTowards(dx, dy int64) int64 {
	// A "diamond angle" in [0, 4096): integer, the same on every CPU.
	var a int64
	ax, ay := geom.Abs(dx), geom.Abs(dy)
	if ax+ay == 0 {
		return c.S.Radius
	}
	q := ay * 1024 / (ax + ay)
	switch {
	case dx >= 0 && dy >= 0:
		a = q
	case dx < 0 && dy >= 0:
		a = 2048 - q
	case dx < 0:
		a = 2048 + q
	default:
		a = 4096 - q
	}
	const sectors = 16
	k, f := a*sectors/4096, a*sectors%4096
	v := func(k int64) int64 {
		return int64(hash(c.in.seed^salt, 0x65646765, c.S.ID.CX, c.S.ID.CY, geom.Mod(k, sectors)) % 23) // "edge"
	}
	n := v(k) + (v(k+1)-v(k))*f/4096 // 0..22
	return c.S.Radius * (78 + n) / 100
}

// district returns the character of the district superblock (i, j) belongs
// to: the nearest of the seeds on a jittered grid of 3x3 superblocks.
func (c *CityPlan) district(i, j int64) uint8 {
	ci, cj := geom.FloorDiv(i, 3), geom.FloorDiv(j, 3)
	best, bestD := [2]int64{}, int64(-1)
	var bestH uint64
	for dj := int64(-1); dj <= 1; dj++ {
		for di := int64(-1); di <= 1; di++ {
			h := hash(c.in.seed^salt, 0x64697374, c.S.ID.CX, c.S.ID.CY, ci+di, cj+dj) // "dist"
			si, sj := (ci+di)*3+int64(h%3), (cj+dj)*3+int64(h>>8%3)
			d := (i-si)*(i-si) + (j-sj)*(j-sj)
			if bestD < 0 || d < bestD {
				best, bestD, bestH = [2]int64{si, sj}, d, h
			}
		}
	}
	// The character from the seed's distance from the centre.
	dx, dy := best[0]*c.A, best[1]*c.A
	d := c.density(dx*dx + dy*dy)
	roll := bestH >> 16 % 100
	big := c.S.ID.Tier >= City
	switch {
	case big && d > 55:
		if roll < 70 {
			return dCore
		}
		return dOld
	case d > 55:
		return dOld
	case d > 30:
		switch {
		case roll < 50:
			return dGrid
		case roll < 70:
			return dOld
		case roll < 85 && big:
			return dIndustrial
		}
		return dGreen
	}
	switch {
	case roll < 50:
		return dGarden
	case roll < 70:
		return dGrid
	case roll < 85 && big:
		return dIndustrial
	}
	return dGreen
}

// spans divides a length into n parts separated by gaps of g, the spare
// shared out from the start; with minSize > 0, n shrinks until each part
// is at least minSize/n.
func spans(length, n, minSize, g int64) []span {
	for n > 1 && minSize > 0 && (length-(n-1)*g)/n < minSize/n {
		n--
	}
	n = max(n, 1)
	total := length - (n-1)*g
	out := make([]span, n)
	at := int64(0)
	for k := int64(0); k < n; k++ {
		size := total / n
		if k < total%n {
			size++
		}
		out[k] = span{at, size}
		at += size + g
	}
	return out
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
				return CityTile{Kind: CArtV, Lane: uint8(u)}
			}
		default:
			if c.included(i, j) || c.included(i, j-1) {
				if u == arterial || u == c.A-1 {
					return CityTile{Kind: CCrossH, Lane: uint8(v)}
				}
				return CityTile{Kind: CArtH, Lane: uint8(v)}
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

const minor = 2 // width of minor streets

func (c *CityPlan) blocks(i, j int64, sb *superblock, p, q, S int64) CityTile {
	col, row := int64(-1), int64(-1)
	for k, sp := range sb.cols {
		if p >= sp.at && p < sp.at+sp.size {
			col = int64(k)
		}
	}
	for k, sp := range sb.rows {
		if q >= sp.at && q < sp.at+sp.size {
			row = int64(k)
		}
	}
	if col < 0 || row < 0 {
		return CityTile{Kind: CRoad} // a minor street
	}
	cs, rs := sb.cols[col], sb.rows[row]
	return c.block(i, j, sb, col, row, p-cs.at, q-rs.at, cs.size, rs.size)
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
			if sb.p.lamps {
				t.Deco = DLamp
			}
		case edge == 1 && sb.p.walkTrees && bu%5 == 2:
			t.Deco = DTree
		}
		return t
	}
	iu, iv := bu-1, bv-1
	iw, ih := bw-2, bh-2
	for _, pl := range c.plots(i, j, sb, col, row, iw, ih) {
		if iu >= pl.x && iu < pl.x+pl.w && iv >= ih-pl.h {
			return CityTile{Kind: CBuilt}
		}
	}
	yard := CityTile{Kind: sb.p.yard}
	switch {
	case sb.p.parking && iv == 0 && iu%4 == 1:
		yard.Kind = CParking // a parking strip along the back
	case sb.p.parking && iv <= 1:
		yard.Kind = CRoad
	case sb.p.yardTrees && iv == 0 && hash(c.in.seed^salt, 0x79617264, c.S.ID.CX, c.S.ID.CY, i, j, col, row, iu)%5 == 0: // "yard"
		yard.Deco = DTree
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
	gap := sb.p.gap
	r := &rng{s: hash(c.in.seed^salt, 0x706c6f74, c.S.ID.CX, c.S.ID.CY, i, j, col, row)} // "plot"
	// The block's corner relative to the superblock, for the rail check.
	bx := arterial + sb.cols[col].at
	by := arterial + sb.rows[row].at
	var line []plot
	x := int64(0)
	for x < iw {
		pi, ok := c.in.pickStyle(r, sb.p, iw-x, ih)
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
			for col, cs := range sb.cols {
				for r, rs := range sb.rows {
					iw, ih := cs.size-2, rs.size-2
					for _, pl := range c.plots(i, j, sb, int64(col), int64(r), iw, ih) {
						b := Building{Prefab: pl.prefab,
							X: geom.Mod(x0+cs.at+1+pl.x, c.in.w), Y: geom.Mod(y0+rs.at+1+ih-pl.h, c.in.h)}
						add(b, pl.w, pl.h)
					}
				}
			}
		}
	}
	return out
}

// StationFront returns the tile in front of a city's station door, where
// arriving passengers come out.
func (in *Infra) StationFront(s Settlement) (Point, bool) {
	if s.Fixed != "" || s.ID.Tier < City {
		return Point{}, false
	}
	a := cityPeriod(s.ID.Tier)
	S := a - arterial
	st, ok := in.prefabStyle("station", S, S)
	if !ok {
		return Point{}, false
	}
	pf := in.prefabs[st]
	x0, y0 := s.X-a/2+arterial+S/2-pf.W/2, s.Y-a/2+arterial+1 // the station's top-left
	return in.norm(x0+pf.DoorX, y0+pf.H), true
}

// pickStyle picks a building of the superblock's style that fits w x h,
// by weight, favouring its material and (by its factor) shops.
func (in *Infra) pickStyle(r *rng, sp sbParams, w, h int64) (int, bool) {
	weight := func(p PrefabSpec) int {
		if p.Kind != "building" || p.Style != sp.style || p.W > w || p.H > h {
			return 0
		}
		wt := p.Weight
		if sp.material != "" && p.Material == sp.material {
			wt *= 3
		}
		if p.Shop {
			wt *= max(sp.shops, 1)
		}
		return wt
	}
	total := 0
	for _, p := range in.prefabs {
		total += weight(p)
	}
	if total == 0 {
		return 0, false
	}
	k := int(r.intn(int64(total)))
	for i, p := range in.prefabs {
		if wt := weight(p); k < wt {
			return i, true
		} else {
			k -= wt
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
