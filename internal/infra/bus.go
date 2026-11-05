package infra

import "math"

// Buses run on the roads between towns and cities. Inside a town the road
// gives way to the town's own streets, so a bus line ends where its road
// meets them: a stop on the road just outside the town, with a pole beside
// the road. The bus follows the road's smoothed line between the two stops.

// BusStop is one end of a bus line.
type BusStop struct {
	S    Settlement
	Stop Point // the road tile where the bus waits
	Pole Point // the stop's sign beside the road (solid)
}

// BusRoute is the line a bus follows: the road between its two stops.
type BusRoute struct {
	Edge   Edge
	A, B   BusStop
	OX, OY int64        // origin, normalised
	Pts    [][2]float64 // the road, in tiles relative to (OX, OY), from A's stop to B's
}

const (
	busStopOut  = 2 // tiles beyond where the road leaves the town
	busPoleSide = 3 // tiles from the road's centre line to the pole
	busClear    = 6 // tiles the road must stay outside the town past the stop
)

// noBus marks roads without a bus line in the cache.
var noBus = &BusRoute{}

// BusRoute returns the bus line of road edge e, or nil if it has none (no
// road, or the two towns' edges too close along it).
func (in *Infra) BusRoute(e Edge) *BusRoute {
	if e.Net != Road {
		return nil
	}
	k := e.key()
	in.mu.Lock()
	r, ok := in.buses[k]
	in.mu.Unlock()
	if !ok {
		r = in.busRoute(e)
		if r == nil {
			r = noBus
		}
		in.mu.Lock()
		if len(in.buses) >= routeCacheLimit {
			clear(in.buses)
		}
		in.buses[k] = r
		in.mu.Unlock()
	}
	if r == noBus {
		return nil
	}
	return r
}

// polyline is a line through points in tiles, with distances.
type polyline struct {
	pts [][2]float64
	cum []float64
}

func newPolyline(pts [][2]float64) polyline {
	l := polyline{pts: pts, cum: make([]float64, len(pts))}
	for i := 1; i < len(pts); i++ {
		l.cum[i] = l.cum[i-1] + math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1])
	}
	return l
}

func (l polyline) length() float64 { return l.cum[len(l.cum)-1] }

// at returns the point at distance s and the unit direction there.
func (l polyline) at(s float64) (x, y, dx, dy float64) {
	k := 1
	for k < len(l.pts)-1 && l.cum[k] < s {
		k++
	}
	a, b := l.pts[k-1], l.pts[k]
	seg := l.cum[k] - l.cum[k-1]
	f := 0.0
	if seg > 0 {
		f = min(max((s-l.cum[k-1])/seg, 0), 1)
		dx, dy = (b[0]-a[0])/seg, (b[1]-a[1])/seg
	}
	return a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f, dx, dy
}

// cut returns the part of the line from distance s0 to s1.
func (l polyline) cut(s0, s1 float64) [][2]float64 {
	x, y, _, _ := l.at(s0)
	out := [][2]float64{{x, y}}
	for i, c := range l.cum {
		if c > s0 && c < s1 {
			out = append(out, l.pts[i])
		}
	}
	x, y, _, _ = l.at(s1)
	return append(out, [2]float64{x, y})
}

func (in *Infra) busRoute(e Edge) *BusRoute {
	w := in.way(e, nil)
	if w == nil || len(w.Pts) < 2 {
		return nil
	}
	pts := make([][2]float64, len(w.Pts))
	for i, p := range w.Pts {
		pts[i] = [2]float64{float64(p[0])/sub - 0.5, float64(p[1])/sub - 0.5} // tile coordinates
	}
	line := newPolyline(pts)
	n := line.length()
	tileAt := func(s float64) Point {
		x, y, _, _ := line.at(s)
		return in.norm(w.OX+int64(math.Round(x)), w.OY+int64(math.Round(y)))
	}
	// The way runs from the end nearer A's route start; find which.
	a, b := e.A, e.B
	if d := in.dist2(Settlement{X: w.OX, Y: w.OY}, e.B); d < in.dist2(Settlement{X: w.OX, Y: w.OY}, e.A) {
		a, b = b, a
	}
	// Where the road leaves a town: going out from its end, the first
	// distance from which it stays outside the town's streets.
	inside := func(s Settlement) func(p Point) bool {
		if s.Fixed != "" || s.ID.Tier < Town || len(in.prefabs) == 0 {
			return nil // a hand-made place: the road starts at its gate
		}
		c := in.city(s)
		return func(p Point) bool { return c.Covers(p.X, p.Y) }
	}
	leave := func(inTown func(Point) bool, from, step float64) (float64, bool) {
		if inTown == nil {
			return from + step*busStopOut, true
		}
		clear, seen := 0, false
		for s := from; s >= 0 && s <= n; s += step {
			if inTown(tileAt(s)) {
				clear, seen = 0, true
				continue
			}
			if !seen && math.Abs(s-from) > 64 {
				return 0, false // no streets: a town in name only
			}
			if clear++; seen && clear > busClear {
				// busClear tiles back was the first one outside.
				return s - step*float64(busClear-busStopOut), true
			}
		}
		return 0, false
	}
	sa, okA := leave(inside(a), 0, 1)
	sb, okB := leave(inside(b), n, -1)
	if !okA || !okB || sb-sa < 32 {
		return nil
	}
	stop := func(s Settlement, at, out float64) (BusStop, bool) {
		x, y, dx, dy := line.at(at)
		st := BusStop{S: s, Stop: tileAt(at)}
		// The pole on the right of the bus arriving (going out of town,
		// that's the left), on dry land outside the town.
		ins := inside(s)
		if ins == nil {
			ins = func(Point) bool { return false }
		}
		for _, side := range []float64{out, -out} {
			px := w.OX + int64(math.Round(x-dy*side*busPoleSide))
			py := w.OY + int64(math.Round(y+dx*side*busPoleSide))
			p := in.norm(px, py)
			if !in.water(p.X, p.Y) && !ins(p) && !in.avoided(p.X, p.Y, 0) {
				st.Pole = p
				return st, true
			}
		}
		return st, false
	}
	stA, okA := stop(a, sa, -1)
	stB, okB := stop(b, sb, 1)
	if !okA || !okB {
		return nil
	}
	r := &BusRoute{Edge: e, A: stA, B: stB, OX: w.OX, OY: w.OY, Pts: line.cut(sa, sb)}
	if a.ID != e.A.ID { // the line goes from e.A to e.B
		r.A, r.B = r.B, r.A
		for i, j := 0, len(r.Pts)-1; i < j; i, j = i+1, j-1 {
			r.Pts[i], r.Pts[j] = r.Pts[j], r.Pts[i]
		}
	}
	return r
}

// BusStopsIn returns the bus stops whose pole is in the rectangle of tiles
// (x, y, w, h).
func (in *Infra) BusStopsIn(x, y, w, h int64) []BusStop {
	r := tierRadius[NumTiers-1][1] + 128
	var out []BusStop
	for _, s := range in.InRect(x-r, y-r, w+2*r, h+2*r, networks[Road].min) {
		for _, e := range in.rng(Road, s) {
			br := in.BusRoute(e)
			if br == nil {
				continue
			}
			st := br.A
			if br.B.S.ID == s.ID {
				st = br.B
			}
			dx, dy := in.delta(x, y, st.Pole.X, st.Pole.Y)
			if dx >= 0 && dy >= 0 && dx < w && dy < h {
				out = append(out, st)
			}
		}
	}
	return out
}
