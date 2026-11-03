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

// Cars is the number of cars of a train, the locomotive included.
const Cars = 4

// Timing, in ticks (60 per game minute).
const (
	Dwell = 10 * sim.TicksPerMinute // waiting at a station before leaving

	vMax   = 2000.0 / sim.TicksPerMinute // tiles per tick: about 120 km/h
	vSlow  = 0.5                         // tiles per tick when leaving and arriving
	aSlow  = 0.002                       // tiles per tick², up to vSlow
	aFast  = 0.2                         // tiles per tick², from vSlow to vMax
	cycleK = 0x747261696e                // "train"
)

// Line is a rail line with its train.
type Line struct {
	Edge   infra.Edge
	ox, oy int64      // the track's first tile (A's buffer stop), normalised
	pts    [][2]int64 // track corners in tiles relative to (ox, oy): axis-aligned legs
	cum    []float64  // track distance to each corner
	Length float64    // in tiles, from buffer stop to buffer stop
	Travel int64      // ticks from leaving one station to arriving at the other
	Cycle  int64      // ticks of a round trip
	offset int64      // where the cycle starts
	w, h   int64      // world size, for wrapping

	prof profile
}

// NewLine makes the line of a rail edge, or returns nil if it has no
// route.
func NewLine(in *infra.Infra, e infra.Edge, worldW, worldH int64, seed uint64) *Line {
	way := in.WayOf(e)
	if way == nil || way.Net != infra.Rail || len(way.Pts) < 2 {
		return nil
	}
	l := &Line{Edge: e, w: worldW, h: worldH}
	for _, p := range way.Pts {
		q := [2]int64{geom.FloorDiv(p[0], 16), geom.FloorDiv(p[1], 16)}
		if n := len(l.pts); n > 0 && l.pts[n-1] == q {
			continue
		}
		l.pts = append(l.pts, q)
	}
	l.ox, l.oy = geom.Mod(way.OX+l.pts[0][0], worldW), geom.Mod(way.OY+l.pts[0][1], worldH)
	o := l.pts[0]
	for k := range l.pts {
		l.pts[k] = [2]int64{l.pts[k][0] - o[0], l.pts[k][1] - o[1]}
	}
	l.cum = make([]float64, len(l.pts))
	for k := 1; k < len(l.pts); k++ {
		a, b := l.pts[k-1], l.pts[k]
		l.cum[k] = l.cum[k-1] + float64(geom.Abs(b[0]-a[0])+geom.Abs(b[1]-a[1]))
	}
	l.Length = l.cum[len(l.cum)-1]
	l.prof = newProfile(l.Length - (Cars - 1))
	l.Travel = int64(math.Ceil(l.prof.total))
	l.Cycle = 2 * (Dwell + l.Travel)
	h := uint64(seed) ^ cycleK
	for _, v := range []int64{e.A.ID.CX, e.A.ID.CY, e.B.ID.CX, e.B.ID.CY} {
		h = (h ^ uint64(v)) * 0x9e3779b97f4a7c15
	}
	l.offset = int64(h>>1) % l.Cycle
	return l
}

// profile is the speed curve of a run: gently up to vSlow, fast up to the
// cruising speed, cruise, and down the same way.
type profile struct {
	d                  float64 // distance to run
	t1, d1, t2, d2, vc float64 // the two speeding-up phases and the cruising speed
	cruise, total      float64
}

func newProfile(d float64) profile {
	p := profile{d: d}
	p.t1, p.d1 = vSlow/aSlow, vSlow*vSlow/(2*aSlow)
	vc := vMax
	// On a short line, the train doesn't reach full speed.
	if 2*(p.d1+(vc*vc-vSlow*vSlow)/(2*aFast)) > d {
		vc = math.Sqrt(max(vSlow*vSlow, aFast*(d-2*p.d1)+vSlow*vSlow))
	}
	p.vc = vc
	p.t2, p.d2 = (vc-vSlow)/aFast, (vc*vc-vSlow*vSlow)/(2*aFast)
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
		return aSlow * t * t / 2
	case t < p.t1+p.t2:
		u := t - p.t1
		return p.d1 + vSlow*u + aFast*u*u/2
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
	switch {
	case c < Dwell:
		return State{AtA: true, ToB: true, Front: Cars - 1, Since: start, Until: start + Dwell}
	case c < Dwell+l.Travel:
		run := float64(c-Dwell) + frac
		return State{Moving: true, ToB: true, Front: Cars - 1 + l.prof.at(min(run, tr)),
			Since: start + Dwell, Until: start + Dwell + l.Travel}
	case c < 2*Dwell+l.Travel:
		s := start + Dwell + l.Travel
		return State{Front: l.Length - (Cars - 1), Since: s, Until: s + Dwell}
	}
	run := float64(c-2*Dwell-l.Travel) + frac
	s := start + 2*Dwell + l.Travel
	return State{Moving: true, Front: l.Length - (Cars - 1) - l.prof.at(min(run, tr)), Since: s, Until: s + l.Travel}
}

// NextStop returns when the train next stands at A's (atA) or B's
// station, at or after clock time t, and when it leaves again.
func (l *Line) NextStop(t int64, atA bool) (arrive, leave int64) {
	c := geom.Mod(t-l.offset, l.Cycle)
	start := t - c
	at := int64(0) // standing at A from the cycle's start
	if !atA {
		at = Dwell + l.Travel
	}
	if c >= at+Dwell {
		start += l.Cycle
	}
	return start + at, start + at + Dwell
}

// Pos returns the tile position (fractional, normalised) of track
// distance s from A's buffer stop, and the direction of increasing s there.
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
	f := s - l.cum[k-1]
	dx, dy := sign(b[0]-a[0]), sign(b[1]-a[1])
	x = float64(l.ox+a[0]) + float64(dx)*f
	y = float64(l.oy+a[1]) + float64(dy)*f
	switch {
	case dx > 0:
		d = world.Right
	case dx < 0:
		d = world.Left
	case dy > 0:
		d = world.Down
	default:
		d = world.Up
	}
	return math.Mod(x+float64(l.w), float64(l.w)), math.Mod(y+float64(l.h), float64(l.h)), d
}

// Car is one car of a train.
type Car struct {
	X, Y   float64   // tile position (fractional, normalised)
	Facing world.Dir // the direction of travel
	Loco   bool
}

// Cars returns where the cars of a train in state st are, the locomotive
// first.
func (l *Line) Cars(st State) []Car {
	out := make([]Car, Cars)
	for k := range Cars {
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

func sign(v int64) int64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
