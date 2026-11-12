package infra

import (
	"container/heap"

	"github.com/noxer/worlds/internal/geom"
)

// Harbours: a coastal town (or bigger) with sea lanes gets a wooden pier
// where the shore is nearest its centre, along one of the four axes, so
// the pier art is straight. Sea lanes start at the pier's tip instead of
// the town's centre.

// Pier is a harbour's walkway out into the water.
type Pier struct {
	Root   Point   // the last land cell before the pier
	DX, DY int64   // the direction out to the water
	Cells  []Point // the pier, from the shore outwards
	Tip    Point   // open water right off the end of the pier: where ferries moor and sea lanes start
}

const (
	pierMax = 10 // tiles
	pierMin = 3
)

// Harbour returns the pier of settlement s, if it has one.
func (in *Infra) Harbour(s Settlement) (Pier, bool) {
	if !member(SeaLane, s) {
		return Pier{}, false
	}
	in.mu.Lock()
	p, ok := in.piers[s.ID]
	in.mu.Unlock()
	if ok {
		return p, len(p.Cells) > 0
	}
	if len(in.rng(SeaLane, s)) > 0 {
		p = in.pier(s)
	}
	in.mu.Lock()
	if len(in.piers) >= siteCacheLimit {
		clear(in.piers)
	}
	in.piers[s.ID] = p
	in.mu.Unlock()
	return p, len(p.Cells) > 0
}

func (in *Infra) pier(s Settlement) Pier {
	water := func(x, y int64) bool { return in.elev(x, y) < in.sea }
	reach := 2*s.Radius + 64
	best, bestT := Pier{}, int64(-1)
	for _, d := range [4][2]int64{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		// Each shore along the axis (land, then water) in turn, until one
		// takes a pier out to the open sea (not into a lake).
		for t := int64(1); t <= reach && (bestT < 0 || t < bestT); t++ {
			x, y := s.X+d[0]*t, s.Y+d[1]*t
			if !water(x, y) || water(x-d[0], y-d[1]) {
				continue
			}
			p := Pier{Root: in.norm(x-d[0], y-d[1]), DX: d[0], DY: d[1]}
			k := int64(0)
			for ; k < pierMax && water(x+d[0]*k, y+d[1]*k); k++ {
				p.Cells = append(p.Cells, in.norm(x+d[0]*k, y+d[1]*k))
			}
			// The water must go on beyond the pier, out to sea, so boats
			// can reach it.
			// right off the end of the pier
			tip := in.norm(x+d[0]*k, y+d[1]*k)
			if k < pierMin || !water(x+d[0]*k, y+d[1]*k) || !water(x+d[0]*(k+1), y+d[1]*(k+1)) || !in.openSea(tip) {
				continue
			}
			p.Tip = tip
			best, bestT = p, t
			break
		}
	}
	return best
}

// Open sea: water that reaches openSeaDist tiles from a point.
const (
	openSeaDist   = 384
	openSeaGrid   = 4
	openSeaExpand = 6000
)

// openSea reports whether the water at p opens out to sea rather than
// being a lake: a search over the water, always going on from the cell
// farthest out, gets openSeaDist tiles away.
func (in *Infra) openSea(p Point) bool {
	const g = openSeaGrid
	water := func(i, j int64) bool {
		return in.elev(geom.Mod(p.X+i*g, in.w), geom.Mod(p.Y+j*g, in.h)) < in.sea
	}
	seen := map[[2]int64]bool{{0, 0}: true}
	open := &pq{}
	seq := int64(0)
	heap.Push(open, pqItem{})
	for expanded := 0; open.Len() > 0 && expanded < openSeaExpand; expanded++ {
		it := heap.Pop(open).(pqItem)
		if (it.i*it.i+it.j*it.j)*g*g >= openSeaDist*openSeaDist {
			return true
		}
		for _, d := range [4][2]int64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			c := [2]int64{it.i + d[0], it.j + d[1]}
			if seen[c] {
				continue
			}
			seen[c] = true
			if water(c[0], c[1]) {
				seq++
				heap.Push(open, pqItem{f: -(c[0]*c[0] + c[1]*c[1]), seq: seq, i: c[0], j: c[1]})
			}
		}
	}
	return false
}

func (in *Infra) norm(x, y int64) Point { return Point{geom.Mod(x, in.w), geom.Mod(y, in.h)} }

// PiersIn returns the piers with a cell in the rectangle of tiles (x, y,
// w, h).
func (in *Infra) PiersIn(x, y, w, h int64) []Pier {
	r := 2*tierRadius[NumTiers-1][1] + 64 + pierMax
	var out []Pier
	for _, s := range in.InRect(x-r, y-r, w+2*r, h+2*r, networks[SeaLane].min) {
		p, ok := in.Harbour(s)
		if !ok {
			continue
		}
		for _, c := range p.Cells {
			dx, dy := in.delta(x, y, c.X, c.Y)
			if dx >= 0 && dy >= 0 && dx < w && dy < h {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
