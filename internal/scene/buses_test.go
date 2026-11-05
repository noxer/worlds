package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// busStop returns a bus route near the start and the stop at its A end.
func busStop(t *testing.T, in *infra.Infra) (*infra.BusRoute, infra.BusStop) {
	t.Helper()
	for _, s := range in.Near(0, 193770, 60000, infra.Town) {
		for _, e := range in.Edges(infra.Road, s) {
			if r := in.BusRoute(e); r != nil {
				return r, r.A
			}
		}
	}
	t.Fatal("no bus")
	return nil, infra.BusStop{}
}

func TestBusJourney(t *testing.T) {
	h, in := newCrowdHarness(t)
	route, stop := busStop(t, in)
	pole := world.Pos{Map: world.Main, X: stop.Pole.X, Y: stop.Pole.Y}
	if id, ok := h.w.Bump(pole); !ok || id != "bus_stop" || !h.w.Solid(pole) {
		t.Fatalf("no bus stop post at %v (%q, %v)", pole, id, ok)
	}
	// Beside the post, facing it.
	placed := false
	for _, d := range []world.Dir{world.Left, world.Right, world.Up, world.Down} {
		q, _ := h.w.Neighbor(pole, d)
		if !h.w.Solid(q) && h.w.Terrain(q) == world.Land {
			back, _ := h.w.DirBetween(q, pole)
			h.e.party.PlaceAll(q, back)
			placed = true
			break
		}
	}
	if !placed {
		t.Fatal("nowhere to stand by the post")
	}
	h.e.nearLines = nil
	h.tick(input.Frame{})
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); !ok {
		t.Fatalf("no timetable at the bus stop (top: %T)", h.top())
	}
	h.choose("wait")
	h.runUntil("boarding", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == busMap })
	r := h.e.ride
	if r == nil || r.line.Kind != transit.Bus || r.from.ID != stop.S.ID || r.line.Edge != route.Edge {
		t.Fatalf("not on the bus from %v: %+v", stop.S.ID, r)
	}

	// The doors stay shut on the road.
	for !r.line.At(float64(h.e.clock.Ticks())).Moving {
		h.tick(input.Frame{})
	}
	h.e.leaveTrain(false)
	if h.e.fade != 0 || h.leader().Pos.Map.Template != busMap {
		t.Fatal("got off a moving bus")
	}

	// A seat until the other town.
	var seat world.Pos
	for p, id := range h.e.signs {
		if id == "train_seat" && p.Map == h.leader().Pos.Map && p.Y == 3 {
			seat = p
			break
		}
	}
	above, _ := h.w.Neighbor(seat, world.Up)
	h.e.party.PlaceAll(above, world.Down)
	h.press(input.Confirm)
	h.choose("rest")
	h.tick(input.Frame{})
	h.tick(input.Frame{})
	if !h.e.ride.arrived || h.e.notice == nil || !strings.Contains(h.e.notice.text, in.Name(r.to)) {
		t.Fatalf("not arrived at %s: %+v, notice %+v", in.Name(r.to), h.e.ride, h.e.notice)
	}

	// Off at the other stop.
	h.e.leaveTrain(false)
	h.runUntil("getting off", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	p := h.leader().Pos
	if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
		t.Errorf("off at %v: not walkable land", p)
	}
	other := route.B.Stop
	if dx, dy := p.X-other.X, p.Y-other.Y; dx*dx+dy*dy > 16 {
		t.Errorf("off at %v, far from %s's stop %v", p, in.Name(r.to), other)
	}
	h.tick(input.Frame{})
	if len(h.e.trainCells) == 0 {
		t.Error("no bus to be seen at the stop")
	}
}
