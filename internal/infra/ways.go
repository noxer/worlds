package infra

import (
	"sort"

	"github.com/noxer/worlds/internal/geom"
)

// Ways are routes ready to be drawn as tiles: smoothed, in fixed point,
// and indexed by the blocks of the main map they cross.

// sub is the fixed-point scale of way coordinates: 1/16 tile.
const sub = 16

// BlockSize is the edge length in tiles of the blocks ways are cached by.
const BlockSize = 1024

// Drawn lists the networks drawn as tiles on the main map.
var Drawn = []Network{Path, Road, Rail}

// halfWidth is the half width of a network's band of tiles, in 1/16 tile:
// a cell belongs to the way if its centre is that close to the line.
var halfWidth = [NumNetworks]int64{Path: 18, Road: 26}

// Way is a smoothed route.
type Way struct {
	Net    Network
	OX, OY int64      // origin: the route's first point, in tiles (normalised)
	Pts    [][2]int64 // polyline relative to the origin, in 1/16 tile
	min    [2]int64   // bounding box of Pts, in 1/16 tile
	max    [2]int64
}

// newWay smooths a route: two rounds of Chaikin's corner cutting, in
// integer arithmetic so every platform gets the same tiles.
func (in *Infra) newWay(n Network, r *Route) *Way {
	o := r.Points[0]
	pts := make([][2]int64, 0, len(r.Points)+2)
	for i, p := range r.Points {
		dx, dy := in.delta(o.X, o.Y, p.X, p.Y)
		q := [2]int64{dx*sub + sub/2, dy*sub + sub/2}
		if n == Rail && i > 0 {
			// Rail stays axis-aligned: a step off the grid (to an end)
			// turns a corner instead of running diagonally, going north or
			// south first, so it arrives at a station along the plaza's
			// north edge.
			last := pts[len(pts)-1]
			if last[0] != q[0] && last[1] != q[1] {
				pts = append(pts, [2]int64{last[0], q[1]})
			}
		}
		pts = append(pts, q)
	}
	for range 2 {
		if n == Rail {
			break // tracks keep their right angles
		}
		if len(pts) < 3 {
			break
		}
		next := make([][2]int64, 0, 2*len(pts))
		next = append(next, pts[0])
		for i := 0; i+1 < len(pts); i++ {
			a, b := pts[i], pts[i+1]
			q := [2]int64{geom.FloorDiv(3*a[0]+b[0], 4), geom.FloorDiv(3*a[1]+b[1], 4)}
			r := [2]int64{geom.FloorDiv(a[0]+3*b[0], 4), geom.FloorDiv(a[1]+3*b[1], 4)}
			if i > 0 {
				next = append(next, q)
			}
			if i+2 < len(pts) {
				next = append(next, r)
			}
		}
		next = append(next, pts[len(pts)-1])
		pts = next
	}
	w := &Way{Net: n, OX: o.X, OY: o.Y, Pts: pts, min: pts[0], max: pts[0]}
	for _, p := range pts {
		w.min = [2]int64{min(w.min[0], p[0]), min(w.min[1], p[1])}
		w.max = [2]int64{max(w.max[0], p[0]), max(w.max[1], p[1])}
	}
	return w
}

// local returns tile (x, y) relative to the way's origin, the shortest way
// round the world.
func (in *Infra) local(w *Way, x, y int64) (int64, int64) {
	return in.delta(w.OX, w.OY, x, y)
}

// touches reports whether the way's band may reach into the rectangle of
// tiles (x, y, ww, hh).
func (in *Infra) touches(w *Way, x, y, ww, hh int64) bool {
	lx, ly := in.local(w, x, y)
	m := halfWidth[w.Net] + sub
	return lx*sub-m <= w.max[0] && (lx+ww)*sub+m >= w.min[0] &&
		ly*sub-m <= w.max[1] && (ly+hh)*sub+m >= w.min[1]
}

// WaysIn returns the ways of the drawn networks that may cross the
// rectangle of tiles (x, y, w, h).
func (in *Infra) WaysIn(x, y, w, h int64) []*Way {
	var out []*Way
	seen := map[*Way]bool{}
	nbx, nby := in.w/BlockSize, in.h/BlockSize
	for by := geom.FloorDiv(y, BlockSize); by <= geom.FloorDiv(y+h-1, BlockSize); by++ {
		for bx := geom.FloorDiv(x, BlockSize); bx <= geom.FloorDiv(x+w-1, BlockSize); bx++ {
			for _, way := range in.block(geom.Mod(bx, nbx), geom.Mod(by, nby)) {
				if !seen[way] && in.touches(way, x, y, w, h) {
					seen[way] = true
					out = append(out, way)
				}
			}
		}
	}
	return out
}

// block returns the ways crossing a block, computing and caching them.
func (in *Infra) block(bx, by int64) []*Way { return in.blockWith(bx, by, nil) }

func (in *Infra) blockWith(bx, by int64, yl *yielder) []*Way {
	k := [2]int64{bx, by}
	in.mu.Lock()
	ws, ok := in.blocks[k]
	in.mu.Unlock()
	if ok {
		return ws
	}
	x, y := bx*BlockSize, by*BlockSize
	for _, n := range Drawn {
		for _, e := range in.edgesNear(n, x, y, BlockSize, BlockSize) {
			if !in.mayCross(e, x, y, BlockSize, BlockSize, yl) {
				continue
			}
			if way := in.way(e, yl); way != nil && in.touches(way, x, y, BlockSize, BlockSize) {
				ws = append(ws, way)
			}
		}
	}
	in.mu.Lock()
	if len(in.blocks) >= blockCacheLimit {
		clear(in.blocks)
	}
	in.blocks[k] = ws
	in.mu.Unlock()
	return ws
}

// way returns the smoothed route of an edge (nil if it has none), cached so
// that blocks share them.
func (in *Infra) way(e Edge, yl *yielder) *Way {
	k := e.key()
	in.mu.Lock()
	w, ok := in.ways[k]
	in.mu.Unlock()
	if ok {
		return w
	}
	if r := in.routeWith(e, yl); r != nil {
		w = in.newWay(e.Net, r)
	}
	in.mu.Lock()
	if len(in.ways) >= routeCacheLimit {
		clear(in.ways)
	}
	in.ways[k] = w
	in.mu.Unlock()
	return w
}

// blockCacheLimit bounds the block cache (about 50 km² of ways).
const blockCacheLimit = 4096

// edgesNear returns the graph edges (before routing) of network n that
// could cross the rectangle: those with an end within the maximum edge
// length of it. Sorted, so results don't depend on map order.
func (in *Infra) edgesNear(n Network, x, y, w, h int64) []Edge {
	reach := networks[n].maxLen
	seen := map[edgeKey]bool{}
	var out []Edge
	for _, s := range in.InRect(x-reach, y-reach, w+2*reach, h+2*reach, networks[n].min) {
		for _, e := range in.rng(n, s) {
			if !seen[e.key()] {
				seen[e.key()] = true
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].key(), out[j].key()
		if a.a != b.a {
			return a.a.less(b.a)
		}
		return a.b.less(b.b)
	})
	return out
}

// mayCross reports whether the route of e can come near the rectangle,
// without computing the full route: the search stays in a box around the
// straight line, and for long routes in a corridor around the cheap coarse
// route.
func (in *Infra) mayCross(e Edge, x, y, w, h int64, yl *yielder) bool {
	spec := networks[e.Net]
	g := spec.grid
	_, a := in.endpoint(e.A, e.B, e.Net)
	_, b := in.endpoint(e.B, e.A, e.Net)
	dx, dy := in.delta(a.X, a.Y, b.X, b.Y)
	gx, gy := roundDiv(dx, g), roundDiv(dy, g)
	// The rectangle relative to a, in tiles.
	rx, ry := in.delta(a.X, a.Y, x, y)
	hit := func(x0, y0, x1, y1 int64) bool { // a box relative to a, in tiles
		return x0 <= rx+w && x1 >= rx && y0 <= ry+h && y1 >= ry
	}
	if max(geom.Abs(gx), geom.Abs(gy)) <= coarseFrom {
		m := max(16, (geom.Abs(gx)+geom.Abs(gy))/3) * g
		return hit(min(0, dx)-m, min(0, dy)-m, max(0, dx)+m, max(0, dy)+m)
	}
	coarse, _, ok := in.search(e, a, b, g*coarseFactor, yl)
	if !ok {
		return false
	}
	r := (corridor + 1) * coarseFactor * g
	for _, c := range coarse {
		cx, cy := c[0]*coarseFactor*g, c[1]*coarseFactor*g
		if hit(cx-r, cy-r, cx+r, cy+r) {
			return true
		}
	}
	return false
}

// Paint marks the cells of an n x n region with its top-left tile at
// (x, y) that ways cover: set(i, net) for cell i = row*n + column, in the
// order of Drawn (so roads paint over paths).
func (in *Infra) Paint(x, y int64, n int, set func(i int, net Network)) {
	ways := in.WaysIn(x, y, int64(n), int64(n))
	sort.SliceStable(ways, func(i, j int) bool { return ways[i].Net < ways[j].Net })
	for _, w := range ways {
		hw := halfWidth[w.Net]
		lx, ly := in.local(w, x, y) // the region's corner relative to the way
		if w.Net == Rail {
			// A chain of single cells along the axis-aligned segments.
			for k := 0; k+1 < len(w.Pts); k++ {
				a, b := w.Pts[k], w.Pts[k+1]
				ax, ay := geom.FloorDiv(a[0], sub)-lx, geom.FloorDiv(a[1], sub)-ly
				bx, by := geom.FloorDiv(b[0], sub)-lx, geom.FloorDiv(b[1], sub)-ly
				for j := max(min(ay, by), 0); j <= min(max(ay, by), int64(n-1)); j++ {
					for i := max(min(ax, bx), 0); i <= min(max(ax, bx), int64(n-1)); i++ {
						set(int(j)*n+int(i), w.Net)
					}
				}
			}
			continue
		}
		for k := 0; k+1 < len(w.Pts); k++ {
			a, b := w.Pts[k], w.Pts[k+1]
			// Cells whose centres are within hw of the segment.
			x0 := geom.FloorDiv(min(a[0], b[0])-hw, sub) - lx
			x1 := geom.FloorDiv(max(a[0], b[0])+hw, sub) - lx
			y0 := geom.FloorDiv(min(a[1], b[1])-hw, sub) - ly
			y1 := geom.FloorDiv(max(a[1], b[1])+hw, sub) - ly
			x0, y0 = max(x0, 0), max(y0, 0)
			x1, y1 = min(x1, int64(n-1)), min(y1, int64(n-1))
			for j := y0; j <= y1; j++ {
				for i := x0; i <= x1; i++ {
					c := [2]int64{(lx+i)*sub + sub/2, (ly+j)*sub + sub/2}
					if segDist2(c, a, b) <= hw*hw {
						set(int(j)*n+int(i), w.Net)
					}
				}
			}
		}
	}
}

// segDist2 compares as the squared distance from p to segment ab (exact
// for points beyond the ends; within, the squared distance scaled down
// exactly by |ab|², rounded down).
func segDist2(p, a, b [2]int64) int64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	px, py := p[0]-a[0], p[1]-a[1]
	l2 := dx*dx + dy*dy
	t := px*dx + py*dy
	switch {
	case l2 == 0 || t <= 0:
		return px*px + py*py
	case t >= l2:
		qx, qy := p[0]-b[0], p[1]-b[1]
		return qx*qx + qy*qy
	}
	cross := px*dy - py*dx
	return cross * cross / l2
}
