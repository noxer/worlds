package infra

import (
	"container/heap"

	"github.com/noxer/worlds/internal/geom"
)

// Ferries sail the sea lanes between two harbours, from pier tip to pier
// tip. A lane's route is searched on a coarse grid that may cut across
// headlands and, near its ends, runs through town; the ferry's route keeps
// to the water: grid points on land are dropped, and legs that touch land
// are searched again on a fine grid.

const (
	ferryGrid   = 4     // tiles, of the fine search
	ferryExpand = 60000 // cells, per fine search
	ferryLead   = 4     // tiles straight out from a pier
)

// SetObstacles tells ferry routes about cells they can't sail through
// beyond what infra knows (islands the world places at sea from its pool
// of patches). f must be safe for concurrent use; it doesn't change the
// world (nothing drawn depends on ferry routes). Call it before any ferry
// route is asked for.
func (in *Infra) SetObstacles(f func(x, y int64) bool) {
	in.mu.Lock()
	in.obstacle = f
	clear(in.ferry)
	in.mu.Unlock()
}

// FerryRoute returns the route of the ferry on sea lane e, from A's pier
// tip to B's, in tiles (each point the shortest way round the world from
// the one before); nil if either end has no pier or there is no route.
func (in *Infra) FerryRoute(e Edge) []Point { return in.ferryRoute(e, nil) }

// FerryRouteReady returns the ferry route of e if it has been computed
// (FerryRoute, or in the background by Prefetch).
func (in *Infra) FerryRouteReady(e Edge) (pts []Point, ok bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	pts, ok = in.ferry[e.key()]
	return pts, ok
}

func (in *Infra) ferryRoute(e Edge, y *yielder) []Point {
	if pts, ok := in.FerryRouteReady(e); ok {
		return pts
	}
	pts := in.findFerryRoute(e, y)
	in.mu.Lock()
	if len(in.ferry) >= routeCacheLimit {
		clear(in.ferry)
	}
	in.ferry[e.key()] = pts
	in.mu.Unlock()
	return pts
}

// ferriesIn computes the ferry routes from harbours near the square of
// tiles at (x, y) of side n.
func (in *Infra) ferriesIn(x, y, n int64, yl *yielder) {
	const m = 1024
	for _, s := range in.InRect(x-m, y-m, n+2*m, n+2*m, networks[SeaLane].min) {
		if _, ok := in.Harbour(s); !ok {
			continue
		}
		for _, e := range in.rng(SeaLane, s) {
			in.ferryRoute(e, yl)
		}
	}
}

func (in *Infra) findFerryRoute(e Edge, y *yielder) []Point {
	if e.Net != SeaLane {
		return nil
	}
	pa, okA := in.Harbour(e.A)
	pb, okB := in.Harbour(e.B)
	if !okA || !okB {
		return nil
	}
	r := in.routeWith(e, y)
	if r == nil || len(r.Points) < 2 {
		return nil
	}
	// The open water: not land, nor the piers, nor hand-made patches (an
	// island placed at sea).
	pier := map[Point]bool{}
	for _, c := range append(append([]Point{}, pa.Cells...), pb.Cells...) {
		pier[c] = true
	}
	in.mu.Lock()
	obstacle := in.obstacle
	in.mu.Unlock()
	sea := func(x, y int64) bool {
		p := in.norm(x, y)
		return in.water(p.X, p.Y) && !pier[p] && (len(in.avoid) == 0 || !in.avoided(p.X, p.Y, 2)) &&
			(obstacle == nil || !obstacle(p.X, p.Y))
	}
	// Unwrapped, from A's tip; grid points on land dropped. At each end,
	// the ferry comes straight out from the pier first (so it lies in
	// line with the pier while moored).
	out := func(p Pier) []Point {
		pts := []Point{p.Tip}
		q := Point{p.Tip.X + p.DX*ferryLead, p.Tip.Y + p.DY*ferryLead}
		if waterLeg(sea, p.Tip, q) {
			pts = append(pts, q)
		}
		return pts
	}
	keep := out(pa)
	for _, p := range r.Points[1 : len(r.Points)-1] {
		if sea(p.X, p.Y) {
			keep = append(keep, in.unwrap(keep[len(keep)-1], p))
		}
	}
	tail := out(pb)
	for k := len(tail) - 1; k >= 0; k-- {
		keep = append(keep, in.unwrap(keep[len(keep)-1], tail[k]))
	}
	route := []Point{keep[0]}
	for i := 1; i < len(keep); i++ {
		a, b := keep[i-1], keep[i]
		if !waterLeg(sea, a, b) {
			path := in.waterPath(sea, a, b, ferryGrid, 16, false, y)
			if path == nil { // round an island: wider, coarser
				path = in.waterPath(sea, a, b, 2*ferryGrid, 96, false, y)
			}
			if path != nil {
				route = append(route, path[1:]...)
				continue
			}
		}
		route = append(route, b)
	}
	return squareUp(sea, in.axisAligned(sea, straighten(sea, route), y))
}

// axisAligned turns the route's diagonal legs into right angles over the
// water (as rail does), so the ferry always heads along an axis: a corner
// either way round, or the leg split in two, or a search on the water in
// four directions.
func (in *Infra) axisAligned(sea func(x, y int64) bool, pts []Point, yl *yielder) []Point {
	out := []Point{pts[0]}
	var leg func(a, b Point, depth int)
	leg = func(a, b Point, depth int) {
		if a.X == b.X || a.Y == b.Y {
			out = append(out, b)
			return
		}
		for _, c := range []Point{{b.X, a.Y}, {a.X, b.Y}} {
			if waterLeg(sea, a, c) && waterLeg(sea, c, b) {
				out = append(out, c, b)
				return
			}
		}
		if depth < 6 {
			if m := (Point{a.X + (b.X-a.X)/2, a.Y + (b.Y-a.Y)/2}); m != a && sea(m.X, m.Y) {
				leg(a, m, depth+1)
				leg(m, b, depth+1)
				return
			}
		}
		if depth < 7 {
			if path := in.waterPath(sea, a, b, ferryGrid, 16, true, yl); path != nil {
				for k := 1; k < len(path); k++ {
					leg(path[k-1], path[k], 7)
				}
				return
			}
		}
		out = append(out, Point{b.X, a.Y}, b) // no way round: rare
	}
	for k := 1; k < len(pts); k++ {
		leg(pts[k-1], pts[k], 0)
	}
	return out
}

// squareUp replaces runs of corners (a staircase) by a single corner where
// that stays on the water, keeping the legs out from the piers, and drops
// points in the middle of straight legs.
func squareUp(sea func(x, y int64) bool, pts []Point) []Point {
	n := len(pts)
	if n < 5 {
		return pts
	}
	out := []Point{pts[0], pts[1]}
	for i := 1; i < n-2; {
		done := false
		for j := min(i+16, n-2); j >= i+2 && !done; j-- {
			a, b := pts[i], pts[j]
			if a.X == b.X || a.Y == b.Y {
				if waterLeg(sea, a, b) {
					out, i, done = append(out, b), j, true
				}
				continue
			}
			for _, c := range []Point{{b.X, a.Y}, {a.X, b.Y}} {
				if !done && waterLeg(sea, a, c) && waterLeg(sea, c, b) {
					out, i, done = append(out, c, b), j, true
				}
			}
		}
		if !done {
			out = append(out, pts[i+1])
			i++
		}
	}
	out = append(out, pts[n-1])
	// Drop points between two legs in line.
	clean := []Point{out[0]}
	for k := 1; k < len(out); k++ {
		p := out[k]
		if p == clean[len(clean)-1] {
			continue
		}
		if m := len(clean); m >= 2 { // (a pier's leg stays straight out)
			q, r := clean[m-2], clean[m-1]
			if (q.X == r.X && r.X == p.X) || (q.Y == r.Y && r.Y == p.Y) {
				clean[m-1] = p
				continue
			}
		}
		clean = append(clean, p)
	}
	return clean
}

// unwrap returns p as seen from q: the shortest way round the world.
func (in *Infra) unwrap(q, p Point) Point {
	dx, dy := in.delta(q.X, q.Y, p.X, p.Y)
	return Point{q.X + dx, q.Y + dy}
}

func (in *Infra) water(x, y int64) bool {
	return in.elev(geom.Mod(x, in.w), geom.Mod(y, in.h)) < in.sea
}

// waterLeg reports whether the straight leg from a to b (unwrapped) is on
// open water (sea) all the way, sampled every tile.
func waterLeg(sea func(x, y int64) bool, a, b Point) bool {
	dx, dy := b.X-a.X, b.Y-a.Y
	n := max(geom.Abs(dx), geom.Abs(dy))
	for k := int64(0); k <= n; k++ {
		if n > 0 && !sea(a.X+roundDiv(dx*k, n), a.Y+roundDiv(dy*k, n)) {
			return false
		}
	}
	return n > 0 || sea(a.X, a.Y)
}

// waterPath searches a way over the water from a to b (unwrapped) on a
// grid of g tiles, in a box around the leg at least margin cells wider;
// nil if there is none.
func (in *Infra) waterPath(sea func(x, y int64) bool, a, b Point, g, margin int64, four bool, yl *yielder) []Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	gx, gy := roundDiv(dx, g), roundDiv(dy, g)
	m := max(margin, (geom.Abs(gx)+geom.Abs(gy))/2)
	minX, maxX := min(0, gx)-m, max(0, gx)+m
	minY, maxY := min(0, gy)-m, max(0, gy)+m
	cols := maxX - minX + 1
	idx := func(i, j int64) int64 { return (j-minY)*cols + (i - minX) }
	pt := func(i, j int64) Point {
		if i == gx && j == gy {
			return b
		}
		return Point{a.X + i*g, a.Y + j*g}
	}
	heur := func(i, j int64) int64 {
		ax, ay := geom.Abs(gx-i), geom.Abs(gy-j)
		if four {
			return 100 * 10 * (ax + ay) * heuristicWeight / 100
		}
		return 100 * (10*max(ax, ay) + 4*min(ax, ay)) * heuristicWeight / 100
	}
	start, goal := idx(0, 0), idx(gx, gy)
	dist := map[int64]int64{start: 0}
	prev := map[int64]int64{}
	closed := map[int64]bool{}
	open := &pq{}
	seq := int64(0)
	heap.Push(open, pqItem{f: heur(0, 0), i: 0, j: 0})
	dirs := [][3]int64{{1, 0, 10}, {-1, 0, 10}, {0, 1, 10}, {0, -1, 10}, {1, 1, 14}, {1, -1, 14}, {-1, 1, 14}, {-1, -1, 14}}
	if four {
		dirs = dirs[:4]
	}
	for expanded := 0; open.Len() > 0; {
		it := heap.Pop(open).(pqItem)
		k := idx(it.i, it.j)
		if closed[k] {
			continue
		}
		closed[k] = true
		if expanded++; expanded > ferryExpand {
			return nil
		}
		if expanded%256 == 0 {
			yl.maybeYield()
		}
		if k == goal {
			var path []Point
			for {
				path = append(path, pt(k%cols+minX, k/cols+minY))
				if k == start {
					break
				}
				k = prev[k]
			}
			for l, r := 0, len(path)-1; l < r; l, r = l+1, r-1 {
				path[l], path[r] = path[r], path[l]
			}
			return path
		}
		from := pt(it.i, it.j)
		for _, d := range dirs {
			ni, nj := it.i+d[0], it.j+d[1]
			if ni < minX || ni > maxX || nj < minY || nj > maxY {
				continue
			}
			nk := idx(ni, nj)
			if closed[nk] {
				continue
			}
			to := pt(ni, nj)
			if !waterLeg(sea, from, to) {
				continue
			}
			c := d[2] * 100
			if in.elev(geom.Mod(to.X, in.w), geom.Mod(to.Y, in.h)) >= in.deep {
				c = d[2] * 130 // the shallows, only where needed
			}
			nd := dist[k] + c
			if old, ok := dist[nk]; ok && old <= nd {
				continue
			}
			dist[nk], prev[nk] = nd, k
			seq++
			heap.Push(open, pqItem{f: nd + heur(ni, nj), seq: seq, i: ni, j: nj})
		}
	}
	return nil
}

// straighten drops corners the ferry can sail past in a straight line
// (looking a few corners ahead).
func straighten(sea func(x, y int64) bool, pts []Point) []Point {
	if len(pts) < 3 {
		return pts
	}
	out := []Point{pts[0]}
	for i := 0; i < len(pts)-1; {
		j := min(i+8, len(pts)-1)
		// The legs out from the piers stay as they are.
		if i == 0 {
			j = 1
		} else if j == len(pts)-1 && i < len(pts)-2 {
			j = len(pts) - 2
		}
		for j > i+1 && !waterLeg(sea, pts[i], pts[j]) {
			j--
		}
		out = append(out, pts[j])
		i = j
	}
	return out
}
