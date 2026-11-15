package scene

import (
	"fmt"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// Stops: where vehicles with a timetable are caught. A station's boards
// (bump "station"), a bus stop's post ("bus_stop") and an airport's
// departures board ("airport") are painted by the generator; a harbour's
// stop is the water off the end of its pier. Bumping one says what it is,
// confirming opens its timetable.

// stop is a place to catch vehicles of one kind.
type stop struct {
	kind  transit.Kind
	s     infra.Settlement
	edges []infra.Edge // the lines that call here
	name  string       // "Elgate Station": the timetable's speaker
	hint  string       // the notice when it is bumped
}

// stopAt returns the stop at tile p, whose bump id is id ("" if none).
func (e *Explore) stopAt(p world.Pos, id string) (stop, bool) {
	switch id {
	case "station":
		if s, ok := e.stationAt(p); ok {
			n := e.infra.Name(s) + " Station"
			return stop{transit.Train, s, e.infra.Edges(infra.Rail, s), n, n + ": departures. Press to read the timetable."}, true
		}
	case "airport":
		if a, ok := e.airportAt(p); ok {
			n := e.infra.Name(a.S) + " Airport"
			return stop{transit.Plane, a.S, a.Flight, n, n + ": departures. Press to read the board."}, true
		}
	case "bus_stop":
		if s, edges, ok := e.busStopAt(p); ok {
			n := e.infra.Name(s) + " bus stop"
			return stop{transit.Bus, s, edges, n, n + ". Press to read the timetable."}, true
		}
	}
	if p.Map.IsMain() {
		if s, ok := e.harbourAt(p); ok {
			n := e.infra.Name(s) + " Harbour"
			return stop{transit.Ferry, s, e.infra.Edges(infra.SeaLane, s), n, n + ". Press to read the timetable."}, true
		}
	}
	return stop{}, false
}

// vehicleBump handles walking into a vehicle, a stop, a taxi or a
// vehicle's doors from inside; it reports whether it did.
func (e *Explore) vehicleBump(p world.Pos) bool {
	if e.infra == nil {
		return false
	}
	if c, ok := e.vehicleAt(p); ok {
		if !c.st.Moving {
			e.board(c.line, c.st.ToB)
		}
		return true
	}
	if _, ok := e.ranks[p]; ok && p.Map.IsMain() {
		e.say("A taxi. Press to talk to the driver.")
		return true
	}
	id, ok := e.bumps[p]
	if !ok {
		id, _ = e.w.Bump(p)
	}
	if isVehicleExit(id) {
		e.leaveVehicle(false)
		return true
	}
	if st, ok := e.stopAt(p, id); ok {
		e.say(st.hint)
		return true
	}
	return false
}

// vehicleTalk handles confirm at a waiting vehicle, a taxi, a stop or a
// seat inside; it reports whether it did.
func (e *Explore) vehicleTalk(ctx *Context, p world.Pos) bool {
	if e.infra == nil {
		return false
	}
	if c, ok := e.vehicleAt(p); ok {
		if !c.st.Moving {
			e.board(c.line, c.st.ToB)
		}
		return true
	}
	if rank, ok := e.ranks[p]; ok && p.Map.IsMain() {
		e.taxiOffer(ctx, rank)
		return true
	}
	id, _ := e.w.Bump(p)
	if st, ok := e.stopAt(p, id); ok {
		e.timetable(ctx, st)
		return true
	}
	if r := e.ride; r != nil && !r.arrived && e.signs[p] != "" && e.signs[p] == vehicles[r.line.Kind].seat {
		e.restOffer(ctx)
		return true
	}
	return false
}

// stationAt returns the city whose station tile p (a timetable board) is.
func (e *Explore) stationAt(p world.Pos) (infra.Settlement, bool) {
	for _, s := range e.infra.Near(p.X, p.Y, 200, infra.City) {
		if e.infra.InStation(s, p.X, p.Y) {
			return s, true
		}
	}
	return infra.Settlement{}, false
}

// harbourAt returns the settlement whose pier tip (the water off the end
// of the pier, where the ferry moors) tile p is.
func (e *Explore) harbourAt(p world.Pos) (infra.Settlement, bool) {
	for _, s := range e.infra.Near(p.X, p.Y, ferryReach, infra.Town) {
		if pier, ok := e.infra.Harbour(s); ok && pier.Tip.X == p.X && pier.Tip.Y == p.Y {
			return s, true
		}
	}
	return infra.Settlement{}, false
}

// airportAt returns the airport whose departures board is tile p.
func (e *Explore) airportAt(p world.Pos) (infra.Airport, bool) {
	for _, a := range e.infra.AirportsIn(p.X, p.Y, 1, 1) {
		if b := a.Board(); b.X == p.X && b.Y == p.Y {
			return a, true
		}
	}
	return infra.Airport{}, false
}

// busStopAt returns the town whose bus stop post is tile p, and the roads
// whose buses stop there.
func (e *Explore) busStopAt(p world.Pos) (infra.Settlement, []infra.Edge, bool) {
	for _, st := range e.infra.BusStopsIn(p.X, p.Y, 1, 1) {
		var edges []infra.Edge
		for _, edge := range e.infra.Edges(infra.Road, st.S) {
			if r := e.infra.BusRoute(edge); r != nil && (r.A.Pole == st.Pole || r.B.Pole == st.Pole) {
				edges = append(edges, edge)
			}
		}
		return st.S, edges, true
	}
	return infra.Settlement{}, nil, false
}

// timetable shows the vehicles leaving a stop, to board or wait for.
func (e *Explore) timetable(ctx *Context, at stop) {
	s := at.s
	now := e.clock.Ticks()
	var choices []dialog.Choice
	var lines []*transit.Line
	for _, edge := range at.edges {
		line := e.line(edge)
		if line == nil {
			continue
		}
		atA := edge.A.ID == s.ID
		dest := edge.B
		if !atA {
			dest = edge.A
		}
		k := len(lines)
		lines = append(lines, line)
		st := line.At(float64(now))
		if !st.Moving && st.AtA == atA {
			choices = append(choices, dialog.Choice{Text: "To " + e.infra.Name(dest) + ": boarding now", Do: fmt.Sprintf("board %d", k)})
		} else {
			arrive, _ := line.NextStop(now, atA)
			choices = append(choices, dialog.Choice{Text: "To " + e.infra.Name(dest) + ": wait for the " + clockTime(arrive),
				Do: fmt.Sprintf("wait %d", k)})
		}
	}
	text := "Where would you like to go?"
	if len(lines) == 0 {
		text = vehicles[at.kind].none
	}
	choices = append(choices, dialog.Choice{Text: "Not now."})
	d := &dialog.Dialog{ID: "timetable", Start: "main", Nodes: map[string]*dialog.Node{
		"main": {Speaker: at.name, Lines: []string{text}, Choices: choices},
	}}
	r := dialog.NewRunner(d)
	ctx.Push(NewDialogBox(r, func() {
		for _, a := range r.Actions() {
			var k int
			var verb string
			if _, err := fmt.Sscanf(a, "%s %d", &verb, &k); err != nil || k >= len(lines) {
				continue
			}
			line := lines[k]
			atA := line.Edge.A.ID == s.ID
			if verb == "wait" {
				// Until shortly before it leaves (it stands there a
				// while; no need to sit through all of that).
				arrive, leave := line.NextStop(e.clock.Ticks(), atA)
				e.clock.AdvanceTicks(max(arrive+1, leave-boardingTicks) - e.clock.Ticks())
				e.refreshReach()
			}
			e.board(line, atA)
		}
	}))
}

// boardingTicks is how long before a vehicle leaves the party gets on
// when it waited for it (5 real seconds).
const boardingTicks = 300

// clockTime formats clock ticks as the time of day.
func clockTime(ticks int64) string {
	m := geom.Mod(ticks/sim.TicksPerMinute, sim.MinutesPerDay)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}
