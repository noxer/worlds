// Package nav finds paths through the world. Paths may cross map edges
// (inside a region, maps share one coordinate space) and warps (doors, stairs,
// cave entrances), which connect regions.
//
// Planning is hierarchical:
//
//  1. Within a region, A* runs on the tile grid in region coordinates.
//  2. Across regions, Dijkstra runs on a small abstract graph whose nodes are
//     warp entries and exits. Edge weights between nodes in the same region are
//     static A* distances, computed lazily and cached until the world's
//     passability version changes.
//
// Only the current leg (up to the first warp, or to the goal) is refined into
// concrete moves, taking dynamic blockers such as NPCs into account. After a
// warp the caller plans again; that keeps plans valid when the world changes
// while the player is on the move.
package nav

import (
	"container/heap"

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

// DefaultWarpCost is the cost of using a warp, in steps. A transition takes
// time, so short walks are preferred over hopping through buildings.
const DefaultWarpCost = 8

// maxCache bounds the static distance cache.
const maxCache = 8192

// Planner plans paths in a world. It is not safe for concurrent use.
type Planner struct {
	w        *world.World
	WarpCost int

	version uint64
	cache   map[[2]world.Pos]int
	byReg   map[*world.Region][]int // warp indices by region of their trigger tile
}

// NewPlanner creates a planner for w.
func NewPlanner(w *world.World) *Planner {
	p := &Planner{w: w, WarpCost: DefaultWarpCost, cache: map[[2]world.Pos]int{}, byReg: map[*world.Region][]int{}}
	for i, wp := range w.Warps {
		r, _, _ := w.ToRegion(wp.From)
		p.byReg[r] = append(p.byReg[r], i)
	}
	p.version = w.Version()
	return p
}

// Plan returns the moves of the current leg from `from` towards goal: either
// all moves to goal, or the moves up to and including the step onto the first
// warp of the route (that move has Warp set). ok is false if goal is
// unreachable.
//
// The leg avoids dynamic blockers if possible. If blockers cut the leg off
// completely, the statically valid leg is returned; the mover should wait and
// plan again.
func (p *Planner) Plan(from, goal world.Pos, blocked Blocked) (moves []Move, ok bool) {
	if from == goal {
		return nil, true
	}
	if p.w.Solid(goal) || p.w.Map(from.Map) == nil {
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
		moves, ok = p.local(from, target, nil)
		if !ok {
			return nil, false
		}
	}
	if warp != nil && len(moves) > 0 {
		moves[len(moves)-1].Warp = warp
	}
	return moves, true
}

// Distance returns the static walking distance between two tiles of the same
// region, ignoring warps and dynamic blockers. It is -1 if unreachable.
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

// firstLeg runs Dijkstra over the abstract warp graph and returns the target
// of the first leg: goal itself, or the trigger tile of the first warp used.
func (p *Planner) firstLeg(from, goal world.Pos) (target world.Pos, warp *world.Warp, ok bool) {
	warps := p.w.Warps
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
	region := func(q world.Pos) *world.Region {
		r, _, _ := p.w.ToRegion(q)
		return r
	}
	goalRegion := region(goal)

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
			// Warp trigger: the only edge is the warp itself.
			relax(cur.n+1, p.WarpCost)
			continue
		}
		at := pos(cur.n)
		r := region(at)
		if r == goalRegion {
			relax(gl, p.Distance(at, goal))
		}
		for _, i := range p.byReg[r] {
			relax(2+2*i, p.Distance(at, warps[i].From))
		}
	}
	if !done[gl] {
		return world.Pos{}, nil, false
	}
	// Walk back to the node right after start.
	n := gl
	for prev[n] != start {
		n = prev[n]
	}
	if n == gl {
		return goal, nil, true
	}
	return pos(n), warps[(n-2)/2], true
}

// local runs A* inside the region of from. Warp tiles are impassable unless
// they are the target, so a path never teleports by accident.
func (p *Planner) local(from, to world.Pos, blocked Blocked) ([]Move, bool) {
	r, fx, fy := p.w.ToRegion(from)
	rt, tx, ty := p.w.ToRegion(to)
	if r == nil || r != rt {
		return nil, false
	}
	if from == to {
		return nil, true
	}
	if p.w.Solid(to) {
		return nil, false
	}
	idx := func(x, y int) int { return (y-r.MinY)*r.W + (x - r.MinX) }
	n := r.W * r.H
	g := make([]int32, n)
	parent := make([]int32, n)
	for i := range g {
		g[i] = -1
	}
	h := func(x, y int) int { return abs(x-tx) + abs(y-ty) }

	si, ti := idx(fx, fy), idx(tx, ty)
	g[si] = 0
	parent[si] = -1
	open := &nodeHeap{{n: si, d: h(fx, fy)}}
	for open.Len() > 0 {
		cur := heap.Pop(open).(nodeItem)
		if cur.n == ti {
			break
		}
		cx, cy := cur.n%r.W+r.MinX, cur.n/r.W+r.MinY
		cg := g[cur.n]
		if int(cg)+h(cx, cy) < cur.d {
			continue // stale entry
		}
		for _, d := range world.Dirs {
			dx, dy := d.Delta()
			nx, ny := cx+dx, cy+dy
			np, ok := r.At(nx, ny)
			if !ok {
				continue
			}
			ni := idx(nx, ny)
			if g[ni] >= 0 && g[ni] <= cg+1 {
				continue
			}
			if ni != ti {
				if p.w.Solid(np) || p.w.WarpAt(np) != nil || (blocked != nil && blocked(np)) {
					continue
				}
			} else if blocked != nil && blocked(np) {
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
		cx, cy := c%r.W+r.MinX, c/r.W+r.MinY
		px, py := pi%r.W+r.MinX, pi/r.W+r.MinY
		to, _ := r.At(cx, cy)
		moves[i] = Move{Dir: dirOf(cx-px, cy-py), To: to}
		c = pi
	}
	return moves, true
}

func dirOf(dx, dy int) world.Dir {
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
