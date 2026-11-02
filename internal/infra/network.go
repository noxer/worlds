package infra

import (
	"container/heap"
	"fmt"

	"github.com/noxer/worlds/internal/geom"
)

// Network is a kind of connection between settlements.
type Network uint8

const (
	Path    Network = iota // dirt paths between villages and bigger places
	Road                   // roads between towns and bigger
	Highway                // highways between cities
	Rail                   // rail lines between cities
	SeaLane                // ferries between coastal towns and bigger
	Flight                 // flights between metropolises
	NumNetworks
)

type networkSpec struct {
	name    string
	min     Tier  // smallest tier connected
	maxLen  int64 // longest edge in tiles; 0 = any
	grid    int64 // routing grid cell in tiles; 0 = straight line
	water   int64 // cost (percent of flat land) of crossing shallow water; 0 = water only (sea lanes)
	slope   int64 // extra cost (percent) per unit of smooth height climbed per tile
	wiggle  int64 // random extra cost (percent, up to) per grid cell: paths meander
	harbour bool  // only coastal settlements
}

var networks = [NumNetworks]networkSpec{
	Path:    {name: "path", min: Village, maxLen: 10240, grid: 32, water: 600, slope: 18, wiggle: 60},
	Road:    {name: "road", min: Town, maxLen: 40960, grid: 64, water: 800, slope: 26, wiggle: 30},
	Highway: {name: "highway", min: City, maxLen: 163840, grid: 128, water: 1000, slope: 35, wiggle: 10},
	Rail:    {name: "rail", min: City, maxLen: 131072, grid: 128, water: 1200, slope: 70, wiggle: 5},
	SeaLane: {name: "sea lane", min: Town, maxLen: 40960, grid: 128, harbour: true},
	Flight:  {name: "flight", min: Metropolis},
}

func (n Network) String() string {
	if n < NumNetworks {
		return networks[n].name
	}
	return fmt.Sprintf("network(%d)", n)
}

// MinTier is the smallest settlement tier the network connects.
func (n Network) MinTier() Tier { return networks[n].min }

// MaxLen is the longest edge of the network in tiles (0: unlimited).
func (n Network) MaxLen() int64 { return networks[n].maxLen }

// Edge is a connection between two settlements; A sorts before B.
type Edge struct {
	Net  Network
	A, B Settlement
}

type edgeKey struct {
	net  Network
	a, b ID
}

func (e Edge) key() edgeKey { return edgeKey{e.Net, e.A.ID, e.B.ID} }

// member reports whether s takes part in network n.
func member(n Network, s Settlement) bool {
	return s.ID.Tier >= networks[n].min && (!networks[n].harbour || s.Coastal)
}

// Edges returns the edges of network n at settlement s.
//
// The edges form a relative neighbourhood graph: s and t are connected
// unless a third member lies closer to both of them, and they are no
// further apart than the network's maximum edge length. Any third member
// that could prevent an edge lies within that length of s, so the graph can
// be computed locally. An edge also needs a route (roads don't cross deep
// sea, sea lanes don't cross land).
func (in *Infra) Edges(n Network, s Settlement) []Edge {
	var out []Edge
	for _, e := range in.rng(n, s) {
		if in.Route(e) != nil {
			out = append(out, e)
		}
	}
	return out
}

// rng returns the relative neighbourhood graph edges at s, before routing.
func (in *Infra) rng(n Network, s Settlement) []Edge {
	if !member(n, s) {
		return nil
	}
	maxLen := networks[n].maxLen
	if maxLen == 0 {
		maxLen = max(in.w, in.h)
	}
	var cands []Settlement
	for _, c := range in.Near(s.X, s.Y, maxLen, networks[n].min) {
		if member(n, c) && c.ID != s.ID {
			cands = append(cands, c)
		}
	}
	var out []Edge
	for _, t := range cands {
		d := in.dist2(s, t)
		if d > maxLen*maxLen {
			continue
		}
		blocked := false
		for _, c := range cands {
			if c.ID != t.ID && in.dist2(s, c) < d && in.dist2(t, c) < d {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		e := Edge{Net: n, A: s, B: t}
		if t.ID.less(s.ID) {
			e.A, e.B = t, s
		}
		if n == SeaLane && !in.acrossWater(e.A, e.B) {
			continue // along the coast, roads do the job
		}
		out = append(out, e)
	}
	return out
}

// acrossWater reports whether at least 60% of the straight line between
// two settlements is water: a sea lane is worth it.
func (in *Infra) acrossWater(a, b Settlement) bool {
	dx, dy := in.delta(a.X, a.Y, b.X, b.Y)
	const n = 32
	water := 0
	for i := int64(1); i < n; i++ {
		if in.elev(a.X+dx*i/n, a.Y+dy*i/n) < in.sea {
			water++
		}
	}
	return water*10 >= (n-1)*6
}

// EdgesIn returns the edges of network n with at least one end in the
// rectangle (x, y, w, h), each once.
func (in *Infra) EdgesIn(n Network, x, y, w, h int64) []Edge {
	seen := map[edgeKey]bool{}
	var out []Edge
	for _, s := range in.InRect(x, y, w, h, networks[n].min) {
		for _, e := range in.Edges(n, s) {
			if !seen[e.key()] {
				seen[e.key()] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Point is a main-map position, normalised.
type Point struct{ X, Y int64 }

// Route is the shape of an edge: a polyline from A to B. Consecutive points
// are one routing grid cell apart (8 directions), except for flights,
// which are straight.
type Route struct {
	Points []Point
	Cost   int64
}

// noRoute marks edges without a route in the cache.
var noRoute = &Route{}

// Route returns the route of an edge, or nil if there is none.
func (in *Infra) Route(e Edge) *Route {
	k := e.key()
	in.mu.Lock()
	r, ok := in.routes[k]
	in.mu.Unlock()
	if !ok {
		r = in.route(e)
		if r == nil {
			r = noRoute
		}
		in.mu.Lock()
		if len(in.routes) >= cacheLimit {
			clear(in.routes)
		}
		in.routes[k] = r
		in.mu.Unlock()
	}
	if r == noRoute {
		return nil
	}
	return r
}

// maxExpand is a safety limit per search. The search area is bounded
// anyway: a box around the straight line for short routes, a corridor
// around the coarse route for long ones.
const maxExpand = 1_000_000

// Long routes are searched hierarchically: first on a grid coarseFactor
// times coarser, then on the network's grid only within corridor coarse
// cells of the coarse route.
const (
	coarseFactor = 8
	coarseFrom   = 64 // fine cells: shorter routes are searched directly
	corridor     = 3  // coarse cells either side
)

// heuristicWeight (in percent) trades a slightly costlier route for a much
// smaller search. It is part of the world: changing it changes routes.
const heuristicWeight = 120

// route finds the route of an edge on the network's grid with A*.
func (in *Infra) route(e Edge) *Route {
	spec := networks[e.Net]
	dx, dy := in.delta(e.A.X, e.A.Y, e.B.X, e.B.Y)
	if spec.grid == 0 {
		return &Route{Points: []Point{{e.A.X, e.A.Y}, {e.B.X, e.B.Y}}, Cost: isqrt(dx*dx + dy*dy)}
	}
	cells, cost, ok := in.search(e, spec.grid)
	if !ok {
		return nil
	}
	pts := make([]Point, len(cells))
	for n, c := range cells {
		pts[n] = Point{geom.Mod(e.A.X+c[0]*spec.grid, in.w), geom.Mod(e.A.Y+c[1]*spec.grid, in.h)}
	}
	// The grid point nearest B is replaced by B itself.
	pts[len(pts)-1] = Point{e.B.X, e.B.Y}
	return &Route{Points: pts, Cost: cost}
}

// search finds the cheapest path of grid cells (offsets from A in cells of
// g tiles) from A to the cell nearest B. Grid offsets cross the seams like
// any others.
func (in *Infra) search(e Edge, g int64) ([][2]int64, int64, bool) {
	spec := networks[e.Net]
	dx, dy := in.delta(e.A.X, e.A.Y, e.B.X, e.B.Y)
	gx, gy := roundDiv(dx, g), roundDiv(dy, g)

	// The search area: a box around the straight line, narrowed to a
	// corridor around the coarse route for long routes.
	m := max(16, (geom.Abs(gx)+geom.Abs(gy))/3)
	minX, maxX := min(0, gx)-m, max(0, gx)+m
	minY, maxY := min(0, gy)-m, max(0, gy)+m
	cols, rows := maxX-minX+1, maxY-minY+1
	var allowed []bool
	if max(geom.Abs(gx), geom.Abs(gy)) > coarseFrom {
		coarse, _, ok := in.search(e, g*coarseFactor)
		if !ok {
			return nil, 0, false
		}
		allowed = make([]bool, cols*rows)
		r := int64(corridor * coarseFactor)
		for _, c := range coarse {
			ci, cj := c[0]*coarseFactor, c[1]*coarseFactor
			for j := max(cj-r, minY); j <= min(cj+r, maxY); j++ {
				for i := max(ci-r, minX); i <= min(ci+r, maxX); i++ {
					allowed[(j-minY)*cols+(i-minX)] = true
				}
			}
		}
	}
	idx := func(i, j int64) int64 { return (j-minY)*cols + (i - minX) }

	type sample struct{ elev, height, wiggle int64 }
	samples := map[int64]sample{}
	at := func(i, j int64) sample {
		k := idx(i, j)
		if v, ok := samples[k]; ok {
			return v
		}
		x, y := geom.Mod(e.A.X+i*g, in.w), geom.Mod(e.A.Y+j*g, in.h)
		v := sample{elev: in.elev(x, y), height: in.relief.Height(in.seed, x, y)}
		if spec.wiggle > 0 {
			// By absolute position, so every edge crossing it sees the
			// same: mostly by blocks of 16 cells (routes bend around
			// them), a little per cell.
			w := uint64(spec.wiggle)
			v.wiggle = int64(hash(in.seed^salt, int64(e.Net), x/(16*g), y/(16*g))%(w+1) +
				hash(in.seed^salt, int64(e.Net), x/g, y/g)%(w/3+1))
		}
		samples[k] = v
		return v
	}
	// Near the ends, sea lanes may cross land (the harbour is in town) and
	// land routes may cross water (a town on the shore).
	endR := func(s Settlement) int64 { return s.Radius/g + 2 }
	ra, rb := endR(e.A), endR(e.B)
	nearEnd := func(i, j int64) bool {
		return i*i+j*j <= ra*ra || (i-gx)*(i-gx)+(j-gy)*(j-gy) <= rb*rb
	}
	// stepCost is the cost of entering (i, j) from smooth height from, for
	// a step of length 10 (straight) or 14 (diagonal), or -1 if impassable.
	// Flat land costs 100 per unit of length.
	stepCost := func(i, j, from, l int64) int64 {
		s := at(i, j)
		h := s.elev
		if spec.water == 0 { // sea lane
			switch {
			case h < in.deep:
				return l * 100
			case h < in.sea:
				return l * 150
			case nearEnd(i, j):
				return l * 400
			}
			return -1
		}
		if h < in.deep && !nearEnd(i, j) {
			return -1
		}
		c := l * (100 + s.wiggle)
		if h < in.sea {
			c = l * spec.water
		}
		return c + l*spec.slope*geom.Abs(s.height-from)/g
	}
	dirs := [8][3]int64{{1, 0, 10}, {-1, 0, 10}, {0, 1, 10}, {0, -1, 10}, {1, 1, 14}, {1, -1, 14}, {-1, 1, 14}, {-1, -1, 14}}
	heur := func(i, j int64) int64 {
		ax, ay := geom.Abs(gx-i), geom.Abs(gy-j)
		return 100 * (10*max(ax, ay) + 4*min(ax, ay)) * heuristicWeight / 100
	}
	start, goal := idx(0, 0), idx(gx, gy)
	dist := map[int64]int64{start: 0}
	prev := map[int64]int64{}
	// With a weighted heuristic a cell can be reached cheaper after it was
	// expanded; it isn't expanded again (weighted A* without reopening).
	closed := map[int64]bool{}
	open := &pq{}
	seq := int64(0)
	heap.Push(open, pqItem{f: heur(0, 0), seq: seq, i: 0, j: 0})
	for expanded := 0; open.Len() > 0; {
		it := heap.Pop(open).(pqItem)
		k := idx(it.i, it.j)
		d := dist[k]
		if it.f-heur(it.i, it.j) > d || closed[k] {
			continue // stale, or already expanded
		}
		closed[k] = true
		expanded++
		if expanded > maxExpand {
			return nil, 0, false
		}
		if k == goal {
			var cells [][2]int64
			for {
				cells = append(cells, [2]int64{k%cols + minX, k/cols + minY})
				if k == start {
					break
				}
				k = prev[k]
			}
			for a, b := 0, len(cells)-1; a < b; a, b = a+1, b-1 {
				cells[a], cells[b] = cells[b], cells[a]
			}
			return cells, d, true
		}
		h0 := at(it.i, it.j).height
		for _, dd := range dirs {
			ni, nj := it.i+dd[0], it.j+dd[1]
			if ni < minX || ni > maxX || nj < minY || nj > maxY {
				continue
			}
			nk := idx(ni, nj)
			if closed[nk] || (allowed != nil && !allowed[nk]) {
				continue
			}
			c := stepCost(ni, nj, h0, dd[2])
			if c < 0 {
				continue
			}
			nd := d + c
			if old, ok := dist[nk]; ok && old <= nd {
				continue
			}
			dist[nk], prev[nk] = nd, k
			seq++
			heap.Push(open, pqItem{f: nd + heur(ni, nj), seq: seq, i: ni, j: nj})
		}
	}
	return nil, 0, false
}

type pqItem struct {
	f, seq int64
	i, j   int64
}

// pq orders by cost estimate, then insertion order, so results never
// depend on the heap's internals.
type pq []pqItem

func (q pq) Len() int { return len(q) }
func (q pq) Less(a, b int) bool {
	if q[a].f != q[b].f {
		return q[a].f < q[b].f
	}
	return q[a].seq < q[b].seq
}
func (q pq) Swap(a, b int) { q[a], q[b] = q[b], q[a] }
func (q *pq) Push(x any)   { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any     { o := *q; it := o[len(o)-1]; *q = o[:len(o)-1]; return it }

func roundDiv(a, b int64) int64 {
	if a < 0 {
		return -((-a + b/2) / b)
	}
	return (a + b/2) / b
}

func isqrt(v int64) int64 {
	if v <= 0 {
		return 0
	}
	r := int64(1)
	for r*r <= v {
		r <<= 1
	}
	lo, hi := r>>1, r
	for lo < hi-1 {
		mid := (lo + hi) / 2
		if mid*mid <= v {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}
