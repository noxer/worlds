// Package nav finds paths through the world: across chunk borders and the
// wrap seam of the main map, and through warps between the main map and
// detail maps.
//
// Planning is hierarchical:
//
//  1. Within one map, A* runs on the tile grid inside a window around start
//     and goal (the main map is far too big to search as a whole). Coordinates
//     in the window are offsets from the start, so the seam is invisible.
//  2. Across maps, Dijkstra runs on a small graph whose nodes are warp
//     entries and exits. Edge weights between nodes on the same map are
//     static A* distances, computed lazily and cached until the world's
//     passability version changes. On the main map only warps within
//     MaxRange are considered.
//
// Only the current leg (up to the first warp, or to the goal) is refined into
// concrete moves, avoiding dynamic blockers such as NPCs. After a warp the
// caller plans again.
//
// Long-distance travel on the main map (beyond MaxRange) will use a chunk
// portal graph; see ARCHITECTURE.md.
package nav

import (
	"container/heap"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/world"
)

// Move is one step of a path.
type Move struct {
	Dir  world.Dir
	To   world.Pos   // tile reached by the step
	Warp *world.Warp // set if reaching To triggers this warp
}

// Blocked reports dynamic obstacles (other actors). It may be nil.
type Blocked func(world.Pos) bool

const (
	// DefaultWarpCost is the cost of using a warp, in steps.
	DefaultWarpCost = 8
	// DefaultMaxRange is the largest Manhattan distance a local search covers.
	DefaultMaxRange = 256
	// margin is how far the search window extends beyond start and goal.
	margin = 24
	// maxCache bounds the static distance cache.
	maxCache = 8192
)

// Planner plans paths in a world. It is not safe for concurrent use.
type Planner struct {
	w        *world.World
	WarpCost int
	MaxRange int64

	version uint64
	cache   map[[2]world.Pos]int
}

// NewPlanner creates a planner for w.
func NewPlanner(w *world.World) *Planner {
	return &Planner{w: w, WarpCost: DefaultWarpCost, MaxRange: DefaultMaxRange,
		cache: map[[2]world.Pos]int{}, version: w.Version()}
}

// Plan returns the moves of the current leg from `from` towards goal: either
// all moves to goal, or the moves up to and including the step onto the first
// warp of the route (that move has Warp set). ok is false if goal is
// unreachable within range.
//
// The leg avoids dynamic blockers if possible. If blockers cut it off
// completely, the statically valid leg is returned; the mover should wait and
// plan again.
func (p *Planner) Plan(from, goal world.Pos, blocked Blocked) (moves []Move, ok bool) {
	if from == goal {
		return nil, true
	}
	if p.w.Solid(goal) {
		return nil, false
	}
	if p.version != p.w.Version() || len(p.cache) > maxCache {
		clear(p.cache)
		p.version = p.w.Version()
	}
	target, warp, ok := p.firstLeg(from, goal)
	if !ok {
		return nil, false
	}
	moves, ok = p.local(from, target, blocked)
	if !ok {
		if moves, ok = p.local(from, target, nil); !ok {
			return nil, false
		}
	}
	if warp != nil && len(moves) > 0 {
		moves[len(moves)-1].Warp = warp
	}
	return moves, true
}

// Distance returns the static walking distance between two tiles of the same
// map, ignoring warps and dynamic blockers. It is -1 if unreachable within
// range.
func (p *Planner) Distance(from, to world.Pos) int {
	key := [2]world.Pos{from, to}
	if d, ok := p.cache[key]; ok {
		return d
	}
	d := -1
	if moves, ok := p.local(from, to, nil); ok {
		d = len(moves)
	}
	p.cache[key] = d
	return d
}

// inRange reports whether a local search between a and b is allowed.
func (p *Planner) inRange(a, b world.Pos) bool {
	dx, dy, ok := p.w.Delta(a, b)
	return ok && geom.Abs(dx)+geom.Abs(dy) <= p.MaxRange
}

// firstLeg runs Dijkstra over the abstract warp graph and returns the target
// of the first leg: goal itself, or the trigger tile of the first warp used.
func (p *Planner) firstLeg(from, goal world.Pos) (target world.Pos, warp *world.Warp, ok bool) {
	warps := p.w.Warps()
	// Node ids: 0 = start, 1 = goal, 2+2i = trigger of warp i, 3+2i = exit of warp i.
	const start, gl = 0, 1
	pos := func(n int) world.Pos {
		switch {
		case n == start:
			return from
		case n == gl:
			return goal
		case n%2 == 0:
			return warps[(n-2)/2].From
		default:
			return warps[(n-3)/2].To
		}
	}

	dist := map[int]int{start: 0}
	prev := map[int]int{}
	done := map[int]bool{}
	pq := &nodeHeap{{n: start}}
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(nodeItem)
		if done[cur.n] {
			continue
		}
		done[cur.n] = true
		if cur.n == gl {
			break
		}
		relax := func(n, cost int) {
			if cost < 0 {
				return
			}
			nd := cur.d + cost
			if d, seen := dist[n]; !seen || nd < d {
				dist[n] = nd
				prev[n] = cur.n
				heap.Push(pq, nodeItem{n: n, d: nd})
			}
		}
		if cur.n >= 2 && cur.n%2 == 0 {
			relax(cur.n+1, p.WarpCost) // the warp itself
			continue
		}
		at := pos(cur.n)
		if p.inRange(at, goal) {
			relax(gl, p.Distance(at, goal))
		}
		for i, wp := range warps {
			if p.inRange(at, wp.From) {
				relax(2+2*i, p.Distance(at, wp.From))
			}
		}
	}
	if !done[gl] {
		return world.Pos{}, nil, false
	}
	n := gl
	for prev[n] != start {
		n = prev[n]
	}
	if n == gl {
		return goal, nil, true
	}
	return pos(n), warps[(n-2)/2], true
}

// local runs A* on one map within a window around from and to. Warp tiles
// are impassable unless they are the target, so a path never teleports by
// accident.
func (p *Planner) local(from, to world.Pos, blocked Blocked) ([]Move, bool) {
	if from == to {
		return nil, true
	}
	tx, ty, ok := p.w.Delta(from, to)
	if !ok || geom.Abs(tx)+geom.Abs(ty) > p.MaxRange || p.w.Solid(to) {
		return nil, false
	}
	// Window in offsets relative to from.
	minX, maxX := min(0, tx)-margin, max(0, tx)+margin
	minY, maxY := min(0, ty)-margin, max(0, ty)+margin
	if mw, mh, wraps := p.w.Size(from.Map); !wraps {
		// Clip to the detail map.
		minX, maxX = max(minX, -from.X), min(maxX, mw-1-from.X)
		minY, maxY = max(minY, -from.Y), min(maxY, mh-1-from.Y)
	} else {
		// Never let the window wrap onto itself.
		if maxX-minX+1 > mw {
			minX, maxX = -mw/2, mw-1-mw/2
		}
		if maxY-minY+1 > mh {
			minY, maxY = -mh/2, mh-1-mh/2
		}
	}
	ww, wh := maxX-minX+1, maxY-minY+1
	idx := func(x, y int64) int { return int((y-minY)*ww + (x - minX)) }
	n := int(ww * wh)
	g := make([]int32, n)
	parent := make([]int32, n)
	for i := range g {
		g[i] = -1
	}
	h := func(x, y int64) int { return int(geom.Abs(x-tx) + geom.Abs(y-ty)) }

	si, ti := idx(0, 0), idx(tx, ty)
	g[si] = 0
	parent[si] = -1
	open := &nodeHeap{{n: si, d: h(0, 0)}}
	for open.Len() > 0 {
		cur := heap.Pop(open).(nodeItem)
		if cur.n == ti {
			break
		}
		cx, cy := int64(cur.n)%ww+minX, int64(cur.n)/ww+minY
		cg := g[cur.n]
		if int(cg)+h(cx, cy) < cur.d {
			continue // stale entry
		}
		for _, d := range world.Dirs {
			dx, dy := d.Delta()
			nx, ny := cx+dx, cy+dy
			if nx < minX || nx > maxX || ny < minY || ny > maxY {
				continue
			}
			ni := idx(nx, ny)
			if g[ni] >= 0 && g[ni] <= cg+1 {
				continue
			}
			np, ok := p.w.Offset(from, nx, ny)
			if !ok {
				continue
			}
			if ni != ti && (p.w.Solid(np) || p.w.WarpAt(np) != nil) {
				continue
			}
			if blocked != nil && blocked(np) {
				continue
			}
			g[ni] = cg + 1
			parent[ni] = int32(cur.n)
			heap.Push(open, nodeItem{n: ni, d: int(cg+1) + h(nx, ny)})
		}
	}
	if g[ti] < 0 {
		return nil, false
	}
	moves := make([]Move, g[ti])
	for i, c := len(moves)-1, ti; i >= 0; i-- {
		pi := int(parent[c])
		cx, cy := int64(c)%ww+minX, int64(c)/ww+minY
		px, py := int64(pi)%ww+minX, int64(pi)/ww+minY
		to, _ := p.w.Offset(from, cx, cy)
		moves[i] = Move{Dir: dirOf(cx-px, cy-py), To: to}
		c = pi
	}
	return moves, true
}

func dirOf(dx, dy int64) world.Dir {
	switch {
	case dx > 0:
		return world.Right
	case dx < 0:
		return world.Left
	case dy < 0:
		return world.Up
	default:
		return world.Down
	}
}

type nodeItem struct{ n, d int }

type nodeHeap []nodeItem

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return h[i].d < h[j].d }
func (h nodeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)        { *h = append(*h, x.(nodeItem)) }
func (h *nodeHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}
