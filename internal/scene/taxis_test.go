package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// taxiRank returns a bus stop near the start with a taxi rank.
func taxiRank(t *testing.T, in *infra.Infra) infra.BusStop {
	t.Helper()
	for _, s := range in.Near(0, 193770, 60000, infra.Town) {
		for _, e := range in.Edges(infra.Road, s) {
			if r := in.BusRoute(e); r != nil {
				for _, st := range []infra.BusStop{r.A, r.B} {
					if st.HasRank {
						return st
					}
				}
			}
		}
	}
	t.Fatal("no taxi rank")
	return infra.BusStop{}
}

func TestTaxiRide(t *testing.T) {
	h, in := newCrowdHarness(t)
	st := taxiRank(t, in)
	front, back := rankCells(st, h.w)
	for _, p := range []world.Pos{front, back} {
		if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
			t.Fatalf("the taxi at %v stands on a solid or wet tile", p)
		}
	}
	// Next to the taxi, facing it.
	placed := false
	for _, car := range []world.Pos{front, back} {
		for _, d := range []world.Dir{world.Left, world.Right, world.Up, world.Down} {
			q, _ := h.w.Neighbor(car, d)
			if q == front || q == back || h.w.Solid(q) || h.w.Terrain(q) != world.Land {
				continue
			}
			if _, bus := h.e.trainAt(q); bus {
				continue
			}
			to, _ := h.w.DirBetween(q, car)
			h.e.party.PlaceAll(q, to)
			placed = true
			break
		}
		if placed {
			break
		}
	}
	if !placed {
		t.Fatal("nowhere to stand by the taxi")
	}
	h.e.ranks = nil
	h.tick(input.Frame{})
	if _, ok := h.e.ranks[front]; !ok {
		t.Fatalf("no taxi at the rank %v", front)
	}
	if !h.e.blocked(front) {
		t.Error("the taxi doesn't block the way")
	}
	h.press(input.Confirm)
	box, ok := h.top().(*DialogBox)
	if !ok || len(box.r.Choices()) < 2 {
		t.Fatalf("no destinations (top: %T)", h.top())
	}
	dests := h.e.taxiDestinations(front, st.S)
	h.choose("taxi 0")
	h.runUntil("getting in", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == taxiMap })
	r := h.e.ride
	if r == nil || r.line.Kind != transit.Taxi || r.to.ID != dests[0].ID {
		t.Fatalf("not in a taxi to %v: %+v", dests[0].ID, r)
	}

	// Off it goes; the doors stay shut on the way.
	h.runUntil("leaving", 400, func() bool { return r.line.At(float64(h.e.clock.Ticks())).Moving })
	h.e.leaveTrain(false)
	if h.e.fade != 0 || h.leader().Pos.Map.Template != taxiMap {
		t.Fatal("got out of a moving taxi")
	}
	if _, _, ok := h.e.viewTrack(0); !ok {
		t.Fatal("no view out of the windows")
	}

	// Doze on the back seat until there.
	var seat world.Pos
	for p, id := range h.e.signs {
		if id == "train_seat" && p.Map == h.leader().Pos.Map {
			seat = p
			break
		}
	}
	below, _ := h.w.Neighbor(seat, world.Down)
	h.e.party.PlaceAll(below, world.Up)
	h.press(input.Confirm)
	h.choose("rest")
	h.tick(input.Frame{})
	h.tick(input.Frame{})
	if !h.e.ride.arrived || h.e.notice == nil || !strings.Contains(h.e.notice.text, in.Name(r.to)) {
		t.Fatalf("not arrived at %s: %+v, notice %+v", in.Name(r.to), h.e.ride, h.e.notice)
	}
	h.e.leaveTrain(false)
	h.runUntil("getting out", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	p := h.leader().Pos
	want := h.e.arrivalAt(r.to)
	if dx, dy, _ := h.w.Delta(p, want); dx*dx+dy*dy > 9 {
		t.Errorf("out at %v, not by %v in %s", p, want, in.Name(r.to))
	}
	if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
		t.Errorf("out at %v: not walkable land", p)
	}
	// The taxi waits there, then drives back.
	h.tick(input.Frame{})
	if len(h.e.trainCells) == 0 {
		t.Error("the taxi isn't to be seen at the drop-off")
	}
}
