package scene

import (
	"fmt"
	"sort"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// Taxis wait at the ranks beside the bus stops (infra.BusStop.Rank), one
// at every stop, always there. The driver takes the party to a place it
// has been to, or the nearest village, town or city, within taxiReach. The
// trip is a one-off line (transit.NewTaxi) ridden like the others, in the
// "taxi" detail map with the way passing the windows; at the end the taxi
// stands at the drop-off until it drives back.

const (
	taxiMap        world.MapID = "taxi"
	taxiReach                  = 30000 // tiles: the farthest a taxi goes
	taxiChoices                = 6
	visitedReach               = 1200 // settlements this close are checked for a visit
	visitedMargin              = 16   // tiles beyond a settlement's radius that count as being there
	taxiDestPrefix             = "taxi "
)

// updateTaxis finds the taxi ranks around the party and notes the places
// it has been to.
func (e *Explore) updateTaxis() {
	l := e.party.Leader()
	if !l.Pos.Map.IsMain() || (e.clock.Ticks()%60 != 0 && e.ranks != nil) {
		return
	}
	if e.ranks == nil {
		e.ranks = map[world.Pos]infra.BusStop{}
	}
	clear(e.ranks)
	for _, st := range e.infra.BusStopsIn(l.Pos.X-trainReach, l.Pos.Y-trainReach, 2*trainReach, 2*trainReach) {
		if !st.HasRank {
			continue
		}
		front, back := rankCells(st, e.w)
		e.ranks[front], e.ranks[back] = st, st
	}
	for _, s := range e.infra.Near(l.Pos.X, l.Pos.Y, visitedReach, infra.Village) {
		dx, dy, _ := e.w.Delta(l.Pos, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
		if r := s.Radius + visitedMargin; dx*dx+dy*dy <= r*r {
			e.visited[s.ID] = s
		}
	}
}

// rankCells returns the tiles of the taxi waiting at a rank.
func rankCells(st infra.BusStop, w *world.World) (front, back world.Pos) {
	front = world.Pos{Map: world.Main, X: st.Rank.X, Y: st.Rank.Y}
	back = world.Pos{Map: world.Main, X: geom.Mod(st.Rank.X-st.RankDir[0], w.W), Y: geom.Mod(st.Rank.Y-st.RankDir[1], w.H)}
	return front, back
}

// rankTaxi is the parked taxi of a rank, as cars to draw.
func rankTaxi(st infra.BusStop, w *world.World) []transit.Car {
	front, back := rankCells(st, w)
	d := dirOf(st.RankDir)
	return []transit.Car{
		{X: float64(front.X), Y: float64(front.Y), Facing: d, Loco: true},
		{X: float64(back.X), Y: float64(back.Y), Facing: d},
	}
}

func dirOf(v [2]int64) world.Dir {
	switch {
	case v[0] > 0:
		return world.Right
	case v[0] < 0:
		return world.Left
	case v[1] > 0:
		return world.Down
	}
	return world.Up
}

// taxiDestinations returns where a taxi from here can go: the places the
// party has been to and the nearest village, town and city, within reach,
// nearest first.
func (e *Explore) taxiDestinations(from world.Pos, here infra.Settlement) []infra.Settlement {
	dist := func(s infra.Settlement) int64 {
		dx, dy, _ := e.w.Delta(from, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
		return dx*dx + dy*dy
	}
	seen := map[infra.ID]bool{here.ID: true}
	var out []infra.Settlement
	add := func(s infra.Settlement) {
		if !seen[s.ID] && dist(s) <= taxiReach*taxiReach {
			seen[s.ID] = true
			out = append(out, s)
		}
	}
	for _, s := range e.visited {
		add(s)
	}
	for _, t := range []infra.Tier{infra.Village, infra.Town, infra.City} {
		best := int64(-1)
		var bs infra.Settlement
		for _, s := range e.infra.Near(from.X, from.Y, taxiReach, t) {
			if s.ID.Tier != t || seen[s.ID] {
				continue
			}
			if d := dist(s); best < 0 || d < best {
				best, bs = d, s
			}
		}
		if best >= 0 {
			add(bs)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if di, dj := dist(out[i]), dist(out[j]); di != dj {
			return di < dj
		}
		return fmt.Sprint(out[i].ID) < fmt.Sprint(out[j].ID)
	})
	if len(out) > taxiChoices {
		out = out[:taxiChoices]
	}
	return out
}

// taxiOffer asks the driver at a rank where to.
func (e *Explore) taxiOffer(ctx *Context, st infra.BusStop) {
	front, _ := rankCells(st, e.w)
	dests := e.taxiDestinations(front, st.S)
	var choices []dialog.Choice
	for k, s := range dests {
		dx, dy, _ := e.w.Delta(front, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
		km := float64(isqrt(dx*dx+dy*dy)) / 1000
		choices = append(choices, dialog.Choice{Text: fmt.Sprintf("%s (%.1f km)", e.infra.Name(s), km),
			Do: fmt.Sprintf("%s%d", taxiDestPrefix, k)})
	}
	text := "Where to?"
	if len(dests) == 0 {
		text = "Nowhere I can take you from here, I'm afraid."
	}
	choices = append(choices, dialog.Choice{Text: "Not now."})
	d := &dialog.Dialog{ID: "taxi", Start: "main", Nodes: map[string]*dialog.Node{
		"main": {Speaker: "Taxi driver", Lines: []string{text}, Choices: choices},
	}}
	r := dialog.NewRunner(d)
	ctx.Push(NewDialogBox(r, func() {
		for _, a := range r.Actions() {
			var k int
			if _, err := fmt.Sscanf(a, taxiDestPrefix+"%d", &k); err != nil || k >= len(dests) {
				continue
			}
			e.takeTaxi(st, dests[k])
		}
	}))
}

// takeTaxi boards the taxi at a rank for s.
func (e *Explore) takeTaxi(st infra.BusStop, s infra.Settlement) {
	to := e.arrivalAt(s)
	_, back := rankCells(st, e.w)
	edge := infra.Edge{Net: infra.Road, A: st.S, B: s}
	line := transit.NewTaxi(edge, infra.Point{X: back.X, Y: back.Y}, st.RankDir,
		infra.Point{X: to.X, Y: to.Y}, e.clock.Ticks(), e.w.W, e.w.H)
	if line == nil {
		e.say("You could walk there from here.")
		return
	}
	e.board(line, true)
}

// arrivalAt is where to put the party arriving at s by taxi (or debug
// travel): a city's station door, just below a village's well, a town's
// plaza; on free land.
func (e *Explore) arrivalAt(s infra.Settlement) world.Pos {
	dest := world.Pos{Map: world.Main, X: s.X - 1, Y: s.Y + 10}
	switch {
	case s.ID.Tier == infra.Village:
		dest.Y = s.Y + 4
	case s.ID.Tier >= infra.City:
		if f, ok := e.infra.StationFront(s); ok {
			dest = world.Pos{Map: world.Main, X: f.X, Y: f.Y}
		}
	}
	if s.Fixed != "" {
		dest = world.Pos{Map: world.Main, X: s.X, Y: s.Y}
		if e.home.Map.IsMain() {
			if dx, dy, _ := e.w.Delta(dest, e.home); dx*dx+dy*dy <= s.Radius*s.Radius {
				dest = e.home // where the game starts
			}
		}
	}
	return e.freeNear(dest)
}

// freeNear returns the first walkable land tile at or to the east of p.
func (e *Explore) freeNear(p world.Pos) world.Pos {
	for r := int64(0); r < 8 && (e.w.Solid(p) || e.w.Terrain(p) != world.Land); r++ {
		p, _ = e.w.Offset(p, 1, 0)
	}
	return p
}

func isqrt(v int64) int64 {
	if v <= 0 {
		return 0
	}
	r := int64(1)
	for r*r <= v {
		r *= 2
	}
	lo, hi := r/2, r
	for lo < hi-1 {
		m := (lo + hi) / 2
		if m*m <= v {
			lo = m
		} else {
			hi = m
		}
	}
	return lo
}
