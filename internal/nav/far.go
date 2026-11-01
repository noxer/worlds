package nav

import (
	"container/heap"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/world"
)

// Long-distance planning on the main map, for goals beyond MaxRange.
//
//  1. Corridor: A* over whole chunks. A chunk's cost comes from sampling its
//     terrain: the real tiles if it is already generated, otherwise the
//     generator's cheap elevation preview, so planning far ahead never
//     generates chunks. Deep water blocks walkers, any water blocks riders.
//  2. Stretch: tile-level A* covers only the next StretchChunks chunks of the
//     corridor, ending on the border to the following chunk. The mover plans
//     again when the stretch is done, exactly as after a warp.
//  3. Learning: if a stretch can't reach its border (something the preview
//     can't see, such as a tree line or a narrow inlet), that chunk-to-chunk
//     step is marked as a dead end and the corridor is planned again.

const (
	// MaxFarRange is the longest trip planned, in tiles (Manhattan). Longer
	// journeys are for public transport.
	MaxFarRange = 4096
	// StretchChunks is how many corridor chunks one stretch covers.
	StretchChunks = 4
	// farRetries bounds how often a corridor is re-planned around dead ends.
	farRetries = 16
	// maxCorridorNodes bounds the chunk-level search.
	maxCorridorNodes = 40000
	// maxCostCache bounds the cache of chunk costs.
	maxCostCache = 1 << 16
)

type chunkKey [2]int64

type costKey struct {
	c chunkKey
	m Mode
}

type costEntry struct {
	cost      int
	generated bool // from real tiles (final), not the preview
}

type edgeKey struct {
	a, b chunkKey
	m    Mode
}

// planFar returns the next stretch towards a far goal on the main map.
func (p *Planner) planFar(from, goal world.Pos, m Mode, blocked Blocked) ([]Move, bool) {
	if _, ok := StepTicks(p.w, goal, m); !ok {
		return nil, false
	}
	fc, gc := p.chunkOf(from), p.chunkOf(goal)
	for attempt := 0; attempt < farRetries; attempt++ {
		cor, ok := p.corridor(fc, gc, m)
		if !ok {
			return nil, false
		}
		last := min(len(cor)-1, StretchChunks)
		if last == len(cor)-1 {
			// The goal is within this stretch: plan to it directly.
			if moves, _, ok := p.local(from, goal, m, blocked); ok {
				return moves, true
			}
			if moves, _, ok := p.local(from, goal, m, nil); ok {
				return moves, true
			}
			if last == 0 {
				return nil, false
			}
			p.deadEnd(cor[last-1], cor[last], m)
			continue
		}
		t, next := cor[last], cor[last+1]
		edge, aim := p.border(t, next, m)
		if len(edge) > 0 {
			isGoal := func(q world.Pos) bool { return edge[q] }
			if moves, _, ok := p.search(from, aim, isGoal, m, blocked); ok && len(moves) > 0 {
				return moves, true
			}
			if moves, _, ok := p.search(from, aim, isGoal, m, nil); ok && len(moves) > 0 {
				return moves, true
			}
		}
		p.deadEnd(t, next, m)
	}
	return nil, false
}

func (p *Planner) chunkOf(q world.Pos) chunkKey {
	return chunkKey{geom.FloorDiv(q.X, world.ChunkSize), geom.FloorDiv(q.Y, world.ChunkSize)}
}

func (p *Planner) deadEnd(a, b chunkKey, m Mode) {
	p.badEdges[edgeKey{a, b, m}] = true
}

// border returns the cells of chunk t on its side facing the adjacent chunk
// next that can be entered and lead into next, and a cell in the middle of
// that side.
func (p *Planner) border(t, next chunkKey, m Mode) (map[world.Pos]bool, world.Pos) {
	cw, ch := p.w.W/world.ChunkSize, p.w.H/world.ChunkSize
	dx := geom.WrapDelta(t[0], next[0], cw)
	dy := geom.WrapDelta(t[1], next[1], ch)
	x0, y0 := t[0]*world.ChunkSize, t[1]*world.ChunkSize
	cell := func(i int64) (int64, int64) {
		switch {
		case dx > 0:
			return x0 + world.ChunkSize - 1, y0 + i
		case dx < 0:
			return x0, y0 + i
		case dy > 0:
			return x0 + i, y0 + world.ChunkSize - 1
		default:
			return x0 + i, y0
		}
	}
	edge := map[world.Pos]bool{}
	for i := int64(0); i < world.ChunkSize; i++ {
		x, y := cell(i)
		q, _ := p.w.Normalize(world.Pos{Map: world.Main, X: x, Y: y})
		across, _ := p.w.Offset(q, dx, dy)
		if _, ok := StepTicks(p.w, q, m); !ok || p.w.WarpAt(q) != nil {
			continue
		}
		if _, ok := StepTicks(p.w, across, m); !ok {
			continue
		}
		edge[q] = true
	}
	mx, my := cell(world.ChunkSize / 2)
	aim, _ := p.w.Normalize(world.Pos{Map: world.Main, X: mx, Y: my})
	return edge, aim
}

// corridor runs A* over chunks from a to b (inclusive), wrap-aware.
func (p *Planner) corridor(a, b chunkKey, m Mode) ([]chunkKey, bool) {
	cw, ch := p.w.W/world.ChunkSize, p.w.H/world.ChunkSize
	norm := func(k chunkKey) chunkKey { return chunkKey{geom.Mod(k[0], cw), geom.Mod(k[1], ch)} }
	a, b = norm(a), norm(b)
	step := int64(world.ChunkSize * m.minTicks())
	h := func(k chunkKey) int {
		return int((geom.Abs(geom.WrapDelta(k[0], b[0], cw)) + geom.Abs(geom.WrapDelta(k[1], b[1], ch))) * step)
	}
	ids := map[chunkKey]int{a: 0}
	keys := []chunkKey{a}
	g := []int{0}
	parent := []int{-1}
	open := &nodeHeap{{n: 0, d: h(a)}}
	found := -1
	for open.Len() > 0 && len(keys) < maxCorridorNodes {
		cur := heap.Pop(open).(nodeItem)
		k := keys[cur.n]
		if g[cur.n]+h(k) < cur.d {
			continue
		}
		if k == b {
			found = cur.n
			break
		}
		for _, d := range world.Dirs {
			dx, dy := d.Delta()
			nk := norm(chunkKey{k[0] + dx, k[1] + dy})
			if p.badEdges[edgeKey{k, nk, m}] {
				continue
			}
			cost := p.chunkCost(nk, m)
			if nk == b || nk == a {
				cost = max(cost, int(step)) // the goal and start chunks are always allowed
			}
			if cost < 0 {
				continue
			}
			ng := g[cur.n] + cost
			ni, seen := ids[nk]
			if seen && g[ni] <= ng {
				continue
			}
			if !seen {
				ni = len(keys)
				ids[nk] = ni
				keys = append(keys, nk)
				g = append(g, ng)
				parent = append(parent, cur.n)
			} else {
				g[ni] = ng
				parent[ni] = cur.n
			}
			heap.Push(open, nodeItem{n: ni, d: ng + h(nk)})
		}
	}
	if found < 0 {
		return nil, false
	}
	var cor []chunkKey
	for i := found; i >= 0; i = parent[i] {
		cor = append(cor, keys[i])
	}
	for i, j := 0, len(cor)-1; i < j; i, j = i+1, j-1 {
		cor[i], cor[j] = cor[j], cor[i]
	}
	return cor, true
}

// chunkCost estimates the ticks to cross a chunk in mode m from 3x3 terrain
// samples, or -1 if the chunk is mostly impassable. Generated chunks use
// their real terrain; others the generator's preview (without generating).
func (p *Planner) chunkCost(k chunkKey, m Mode) int {
	ck := costKey{k, m}
	x0, y0 := k[0]*world.ChunkSize, k[1]*world.ChunkSize
	origin := world.Pos{Map: world.Main, X: x0, Y: y0}
	generated := p.w.IsCached(origin)
	if c, ok := p.costs[ck]; ok && (c.generated || !generated) {
		return c.cost
	}
	total, blocked, n := 0, 0, 0
	for _, oy := range []int64{5, 16, 27} {
		for _, ox := range []int64{5, 16, 27} {
			q, _ := p.w.Normalize(world.Pos{Map: world.Main, X: x0 + ox, Y: y0 + oy})
			var t world.Terrain
			if generated {
				t = p.w.Terrain(q)
			} else if pt, ok := p.w.Preview(q.X, q.Y); ok {
				t = pt
			} else {
				t = world.Land // no preview: assume open land, refine on the way
			}
			n++
			switch {
			case t == world.Land:
				total += m.minTicks()
			case t == world.Shallow && m == Foot:
				total += SwimTicks
			default:
				blocked++
			}
		}
	}
	cost := -1
	if blocked*2 <= n {
		// Passable on the whole; partly blocked chunks are dearer.
		cost = (total + blocked*4*m.minTicks()) * world.ChunkSize / n
	}
	if len(p.costs) > maxCostCache {
		clear(p.costs)
	}
	// Preview estimates are refined once the chunk exists.
	p.costs[ck] = costEntry{cost, generated}
	return cost
}
