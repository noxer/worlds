package infra

import "github.com/noxer/worlds/internal/geom"

// Harbours: a coastal town (or bigger) with sea lanes gets a wooden pier
// where the shore is nearest its centre, along one of the four axes, so
// the pier art is straight. Sea lanes start at the pier's tip instead of
// the town's centre.

// Pier is a harbour's walkway out into the water.
type Pier struct {
	Root   Point   // the last land cell before the pier
	DX, DY int64   // the direction out to the water
	Cells  []Point // the pier, from the shore outwards
	Tip    Point   // open water off the end of the pier: where sea lanes start
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
		for t := int64(1); t <= reach; t++ {
			x, y := s.X+d[0]*t, s.Y+d[1]*t
			if !water(x, y) {
				continue
			}
			if bestT >= 0 && t >= bestT {
				break
			}
			p := Pier{Root: in.norm(x-d[0], y-d[1]), DX: d[0], DY: d[1]}
			k := int64(0)
			for ; k < pierMax && water(x+d[0]*k, y+d[1]*k); k++ {
				p.Cells = append(p.Cells, in.norm(x+d[0]*k, y+d[1]*k))
			}
			// The water must go on beyond the pier, so boats can reach it.
			if k < pierMin || !water(x+d[0]*(k+1), y+d[1]*(k+1)) {
				break
			}
			p.Tip = in.norm(x+d[0]*(k+1), y+d[1]*(k+1))
			best, bestT = p, t
			break
		}
	}
	return best
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
