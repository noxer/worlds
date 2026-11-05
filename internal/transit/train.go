// Package transit runs public transport: where each vehicle is at any
// time, from its timetable. Nothing is simulated in between, so a train is
// where its timetable says whether or not anyone watches (like NPC
// schedules, see package sim).
//
// Trains: one train shuttles on each rail line between two city stations.
// It waits at the platform, leaves gently, speeds up to its cruising speed
// and slows down into the next station the same way.
package transit

import (
	"math"
	"sort"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// Kind is a kind of vehicle.
type Kind uint8

const (
	Train Kind = iota // on a rail line between two stations
	Ferry             // on a sea lane between two piers
	Bus               // on a road between two towns' bus stops
	Taxi              // one trip, from a taxi rank to where the party asked
)

// spec is how a kind of vehicle runs.
type spec struct {
	cars                      int     // cars (a train's locomotive included; a ferry's bow and stern)
	vMax, vSlow, aSlow, aFast float64 // tiles per tick and tiles per tick²
	dwell                     int64   // ticks at each end
}

var specs = [...]spec{
	// About 120 km/h; leaving gently up to half a tile per tick.
	Train: {cars: 4, vMax: 2000.0 / sim.TicksPerMinute, vSlow: 0.5, aSlow: 0.002, aFast: 0.2, dwell: Dwell},
	// About 36 km/h; casting off slowly.
	Ferry: {cars: 2, vMax: 600.0 / sim.TicksPerMinute, vSlow: 0.3, aSlow: 0.001, aFast: 0.02, dwell: Dwell},
	// About 50 km/h; five minutes at a stop.
	Bus: {cars: 2, vMax: 833.0 / sim.TicksPerMinute, vSlow: 0.25, aSlow: 0.004, aFast: 0.05, dwell: 5 * sim.TicksPerMinute},
	// About 60 km/h; it leaves as soon as the doors are shut.
	Taxi: {cars: 2, vMax: 1000.0 / sim.TicksPerMinute, vSlow: 0.3, aSlow: 0.006, aFast: 0.08, dwell: 2 * sim.TicksPerMinute},
}

// Timing, in ticks (60 per game minute).
const (
	Dwell  = 10 * sim.TicksPerMinute // a train's or ferry's wait at a station or pier before leaving
	cycleK = 0x747261696e            // "train"
)

// Line is a route with its vehicle shuttling between the two ends.
type Line struct {
	Kind   Kind
	Edge   infra.Edge
	Cars   int          // cars of its vehicle
	Dwell  int64        // ticks it waits at each end
	ox, oy int64        // the route's first tile (A's end), normalised
	pts    [][2]float64 // route corners in tiles relative to (ox, oy)
	cum    []float64    // route distance to each corner
	Length float64      // in tiles, from end to end
	Travel int64        // ticks from leaving one end to arriving at the other
	Cycle  int64        // ticks of a round trip
	offset int64        // where the cycle starts
	w, h   int64        // world size, for wrapping

	prof profile
}

// NewLine makes the line of a rail edge, or returns nil if it has no
// route.
func NewLine(in *infra.Infra, e infra.Edge, worldW, worldH int64, seed uint64) *Line {
	way := in.WayOf(e)
	if way == nil || way.Net != infra.Rail || len(way.Pts) < 2 {
		return nil
	}
	var pts [][2]int64
	for _, p := range way.Pts {
		q := [2]int64{way.OX + geom.FloorDiv(p[0], 16), way.OY + geom.FloorDiv(p[1], 16)}
		if n := len(pts); n > 0 && pts[n-1] == q {
			continue
		}
		pts = append(pts, q)
	}
	return newLine(Train, e, pts, worldW, worldH, seed)
}

// NewBus makes the bus line of a road edge, between its two towns' bus
// stops, or returns nil if it has none.
func NewBus(in *infra.Infra, e infra.Edge, worldW, worldH int64, seed uint64) *Line {
	r := in.BusRoute(e)
	if r == nil {
		return nil
	}
	return newLineF(Bus, e, r.OX, r.OY, r.Pts, worldW, worldH, seed)
}

// NewFerry makes the line of a sea lane, from pier tip to pier tip, or
// returns nil if it has no ferry route (infra.FerryRoute, which it computes
// if it isn't ready).
func NewFerry(in *infra.Infra, e infra.Edge, worldW, worldH int64, seed uint64) *Line {
	if e.Net != infra.SeaLane {
		return nil
	}
	route, ok := in.FerryRouteReady(e)
	if !ok {
		route = in.FerryRoute(e)
	}
	if len(route) < 2 {
		return nil
	}
	pts := make([][2]int64, 0, len(route))
	for _, p := range route { // already unwrapped
		q := [2]int64{p.X, p.Y}
		if n := len(pts); n == 0 || pts[n-1] != q {
			pts = append(pts, q)
		}
	}
	return newLine(Ferry, e, pts, worldW, worldH, seed)
}

// NewTaxi makes the trip of a taxi waiting at from (its back; facing dir)
// to to, for e (A: where it waits, B: where it goes), boarded at clock
// time now: it leaves at once, along a road-like route with one corner,
// first straight ahead. Nil if the trip is too short.
func NewTaxi(e infra.Edge, from infra.Point, dir [2]int64, to infra.Point, now, worldW, worldH int64) *Line {
	dx, dy := geom.WrapDelta(from.X, to.X, worldW), geom.WrapDelta(from.Y, to.Y, worldH)
	pts := [][2]int64{{from.X, from.Y}}
	// Ahead first (along dir's axis), then across.
	if dir[0] != 0 {
		pts = append(pts, [2]int64{from.X + dx, from.Y})
	} else {
		pts = append(pts, [2]int64{from.X, from.Y + dy})
	}
	pts = append(pts, [2]int64{from.X + dx, from.Y + dy})
	var clean [][2]int64
	for _, p := range pts {
		if n := len(clean); n == 0 || clean[n-1] != p {
			clean = append(clean, p)
		}
	}
	l := newLine(Taxi, e, clean, worldW, worldH, 0)
	if l == nil {
		return nil
	}
	l.offset = geom.Mod(now, l.Cycle) // the trip starts now, waiting at A
	return l
}

func newLine(k Kind, e infra.Edge, pts [][2]int64, worldW, worldH int64, seed uint64) *Line {
	if len(pts) < 2 {
		return nil
	}
	rel := make([][2]float64, len(pts))
	for i, p := range pts {
		rel[i] = [2]float64{float64(p[0] - pts[0][0]), float64(p[1] - pts[0][1])}
	}
	return newLineF(k, e, pts[0][0], pts[0][1], rel, worldW, worldH, seed)
}

// newLineF makes a line through points in tiles relative to (ox, oy).
func newLineF(k Kind, e infra.Edge, ox, oy int64, pts [][2]float64, worldW, worldH int64, seed uint64) *Line {
	if len(pts) < 2 {
		return nil
	}
	sp := specs[k]
	l := &Line{Kind: k, Edge: e, Cars: sp.cars, Dwell: sp.dwell, w: worldW, h: worldH}
	l.ox, l.oy = geom.Mod(ox, worldW), geom.Mod(oy, worldH)
	l.pts = pts
	l.cum = make([]float64, len(l.pts))
	for i := 1; i < len(l.pts); i++ {
		a, b := l.pts[i-1], l.pts[i]
		l.cum[i] = l.cum[i-1] + math.Hypot(b[0]-a[0], b[1]-a[1])
	}
	l.Length = l.cum[len(l.cum)-1]
	if l.Length <= float64(l.Cars) {
		return nil
	}
	l.prof = newProfile(l.Length-float64(l.Cars-1), sp)
	l.Travel = int64(math.Ceil(l.prof.total))
	l.Cycle = 2 * (l.Dwell + l.Travel)
	h := uint64(seed) ^ cycleK ^ uint64(k)
	for _, v := range []int64{e.A.ID.CX, e.A.ID.CY, e.B.ID.CX, e.B.ID.CY} {
		h = (h ^ uint64(v)) * 0x9e3779b97f4a7c15
	}
	l.offset = int64(h>>1) % l.Cycle
	return l
}

// profile is the speed curve of a run: gently up to vSlow, fast up to the
// cruising speed, cruise, and down the same way.
type profile struct {
	sp                 spec
	d                  float64 // distance to run
	t1, d1, t2, d2, vc float64 // the two speeding-up phases and the cruising speed
	cruise, total      float64
}

func newProfile(d float64, sp spec) profile {
	p := profile{sp: sp, d: d}
	p.t1, p.d1 = sp.vSlow/sp.aSlow, sp.vSlow*sp.vSlow/(2*sp.aSlow)
	vc := sp.vMax
	// On a short line, the vehicle doesn't reach full speed.
	if 2*(p.d1+(vc*vc-sp.vSlow*sp.vSlow)/(2*sp.aFast)) > d {
		vc = math.Sqrt(max(sp.vSlow*sp.vSlow, sp.aFast*(d-2*p.d1)+sp.vSlow*sp.vSlow))
	}
	p.vc = vc
	p.t2, p.d2 = (vc-sp.vSlow)/sp.aFast, (vc*vc-sp.vSlow*sp.vSlow)/(2*sp.aFast)
	p.cruise = max(0, d-2*(p.d1+p.d2)) / vc
	p.total = 2*(p.t1+p.t2) + p.cruise
	return p
}

// at returns the distance covered t ticks after leaving.
func (p profile) at(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= p.total:
		return p.d
	case t > p.total/2:
		return p.d - p.at(p.total-t) // arriving mirrors leaving
	case t < p.t1:
		return p.sp.aSlow * t * t / 2
	case t < p.t1+p.t2:
		u := t - p.t1
		return p.d1 + p.sp.vSlow*u + p.sp.aFast*u*u/2
	}
	return min(p.d1+p.d2+(t-p.t1-p.t2)*p.vc, p.d/2)
}

// TimeTo returns the ticks after leaving a station at which the train has
// covered dist tiles.
func (l *Line) TimeTo(dist float64) float64 {
	lo, hi := 0.0, l.prof.total
	for range 50 {
		mid := (lo + hi) / 2
		if l.prof.at(mid) < dist {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

// State is where a line's train is at some time.
type State struct {
	Moving bool
	AtA    bool    // standing at A's station (when not moving)
	ToB    bool    // heading for B (moving, or leaving next)
	Front  float64 // track distance of the locomotive from A's buffer stop
	// Since is when the current wait or run started, Until when it ends
	// (ticks of the clock).
	Since, Until int64
}

// At returns the state at clock time t (ticks, with a fraction between
// ticks for smooth drawing).
func (l *Line) At(t float64) State {
	tick := int64(math.Floor(t))
	frac := t - float64(tick)
	c := geom.Mod(tick-l.offset, l.Cycle)
	start := tick - c
	tr := float64(l.Travel)
	back := float64(l.Cars - 1) // the locomotive (or bow) is this far from the end
	switch {
	case c < l.Dwell:
		return State{AtA: true, ToB: true, Front: back, Since: start, Until: start + l.Dwell}
	case c < l.Dwell+l.Travel:
		run := float64(c-l.Dwell) + frac
		return State{Moving: true, ToB: true, Front: back + l.prof.at(min(run, tr)),
			Since: start + l.Dwell, Until: start + l.Dwell + l.Travel}
	case c < 2*l.Dwell+l.Travel:
		s := start + l.Dwell + l.Travel
		return State{Front: l.Length - back, Since: s, Until: s + l.Dwell}
	}
	run := float64(c-2*l.Dwell-l.Travel) + frac
	s := start + 2*l.Dwell + l.Travel
	return State{Moving: true, Front: l.Length - back - l.prof.at(min(run, tr)), Since: s, Until: s + l.Travel}
}

// NextStop returns when the train next stands at A's (atA) or B's
// station, at or after clock time t, and when it leaves again.
func (l *Line) NextStop(t int64, atA bool) (arrive, leave int64) {
	c := geom.Mod(t-l.offset, l.Cycle)
	start := t - c
	at := int64(0) // standing at A from the cycle's start
	if !atA {
		at = l.Dwell + l.Travel
	}
	if c >= at+l.Dwell {
		start += l.Cycle
	}
	return start + at, start + at + l.Dwell
}

// Pos returns the tile position (fractional, normalised) of route
// distance s from A's end, and the direction of increasing s there (the
// main one, on a diagonal leg).
func (l *Line) Pos(s float64) (x, y float64, d world.Dir) {
	s = min(max(s, 0), l.Length)
	k := sort.SearchFloat64s(l.cum, s)
	if k == 0 {
		k = 1
	}
	if k >= len(l.pts) {
		k = len(l.pts) - 1
	}
	a, b := l.pts[k-1], l.pts[k]
	seg := l.cum[k] - l.cum[k-1]
	f := 0.0
	if seg > 0 {
		f = (s - l.cum[k-1]) / seg
	}
	dx, dy := b[0]-a[0], b[1]-a[1]
	x = float64(l.ox) + a[0] + dx*f
	y = float64(l.oy) + a[1] + dy*f
	switch {
	case math.Abs(dx) >= math.Abs(dy) && dx > 0:
		d = world.Right
	case math.Abs(dx) >= math.Abs(dy):
		d = world.Left
	case dy > 0:
		d = world.Down
	default:
		d = world.Up
	}
	w, h := float64(l.w), float64(l.h)
	return math.Mod(math.Mod(x, w)+w, w), math.Mod(math.Mod(y, h)+h, h), d
}

// Car is one car of a train.
type Car struct {
	X, Y   float64   // tile position (fractional, normalised)
	Facing world.Dir // the direction of travel
	Loco   bool
}

// CarsAt returns where the cars of the vehicle in state st are, the
// locomotive (or bow) first.
func (l *Line) CarsAt(st State) []Car {
	out := make([]Car, l.Cars)
	if l.Kind != Train {
		return l.rigidAt(st, out)
	}
	for k := range l.Cars {
		s := st.Front - float64(k) // heading for B: the cars trail towards A
		if !st.ToB {
			s = st.Front + float64(k)
		}
		x, y, d := l.Pos(s)
		if !st.ToB {
			d = opposite(d)
		}
		out[k] = Car{X: x, Y: y, Facing: d, Loco: k == 0}
	}
	return out
}

// rigidAt places a ferry or a bus: its front where the route puts it, its
// back right behind, in line along the main direction of the route there
// (it doesn't bend round corners like a train).
func (l *Line) rigidAt(st State, out []Car) []Car {
	s, back := st.Front, st.Front-1
	if !st.ToB {
		back = st.Front + 1
	}
	bx, by, _ := l.Pos(s)
	sx, sy, _ := l.Pos(back)
	dx, dy := wrapF(bx-sx, float64(l.w)), wrapF(by-sy, float64(l.h))
	var d world.Dir
	var ux, uy float64
	switch {
	case math.Abs(dx) >= math.Abs(dy) && dx >= 0:
		d, ux = world.Right, 1
	case math.Abs(dx) >= math.Abs(dy):
		d, ux = world.Left, -1
	case dy > 0:
		d, uy = world.Down, 1
	default:
		d, uy = world.Up, -1
	}
	w, h := float64(l.w), float64(l.h)
	out[0] = Car{X: bx, Y: by, Facing: d, Loco: true}
	out[1] = Car{X: math.Mod(bx-ux+w, w), Y: math.Mod(by-uy+h, h), Facing: d}
	return out
}

// wrapF returns d the shortest way round a world of size n.
func wrapF(d, n float64) float64 {
	switch {
	case d > n/2:
		return d - n
	case d < -n/2:
		return d + n
	}
	return d
}

func opposite(d world.Dir) world.Dir {
	switch d {
	case world.Up:
		return world.Down
	case world.Down:
		return world.Up
	case world.Left:
		return world.Right
	}
	return world.Left
}
