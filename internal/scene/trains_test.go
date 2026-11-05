package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// stationDoor returns a city with rail lines, a timetable board of its
// station and the tile beside it (east) to stand on.
func stationDoor(t *testing.T, in *infra.Infra) (infra.Settlement, world.Pos, world.Pos) {
	t.Helper()
	for _, s := range in.Near(0, 193770, 140000, infra.City) {
		if len(in.Edges(infra.Rail, s)) == 0 {
			continue
		}
		r, ok := in.StationRect(s)
		if !ok {
			continue
		}
		// The first timetable board: column 1 of the concourse (row 7).
		board := world.Pos{Map: world.Main, X: r.X + 1, Y: r.Y + 7}
		return s, board, world.Pos{Map: world.Main, X: r.X + 2, Y: r.Y + 7}
	}
	t.Fatal("no station")
	return infra.Settlement{}, world.Pos{}, world.Pos{}
}

// choose picks the dialog choice whose action starts with prefix.
func (h *harness) choose(prefix string) {
	h.t.Helper()
	box, ok := h.top().(*DialogBox)
	if !ok {
		h.t.Fatalf("no dialog open (top: %T)", h.top())
	}
	for i, c := range box.r.Choices() {
		if strings.HasPrefix(c.Do, prefix) {
			box.cursor = i
			for k := 0; k < 20 && h.top() != Scene(h.e); k++ {
				h.press(input.Confirm) // reveal the text, then choose
			}
			if h.top() != Scene(h.e) {
				h.t.Fatal("the dialog didn't close")
			}
			return
		}
	}
	h.t.Fatalf("no choice %q in %+v", prefix, box.r.Choices())
}

func TestTrainJourney(t *testing.T) {
	h, in := newCrowdHarness(t)
	city, _, front := stationDoor(t, in)
	h.e.party.PlaceAll(front, world.Left)
	h.tick(input.Frame{})

	// The timetable at the station door: wait for the next train.
	h.press(input.Confirm)
	h.choose("wait")
	before := h.e.clock.Ticks()
	h.runUntil("boarding", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == trainMap })
	r := h.e.ride
	if r == nil || r.from.ID != city.ID {
		t.Fatalf("not riding from %v: %+v", city.ID, r)
	}
	if h.e.clock.Ticks() < before {
		t.Fatal("the clock went back")
	}

	// The doors stay closed while it moves.
	st := r.line.At(float64(h.e.clock.Ticks()))
	for st.Moving == false {
		h.tick(input.Frame{})
		st = r.line.At(float64(h.e.clock.Ticks()))
	}
	h.e.leaveTrain(false)
	if h.e.fade != 0 || h.leader().Pos.Map.Template != trainMap {
		t.Fatal("got off a moving train")
	}

	// Resting on a seat: woken on arrival.
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

	// Off at the destination's station door.
	h.e.leaveTrain(false)
	h.runUntil("getting off", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	if p := h.leader().Pos; !in.InStation(r.to, p.X, p.Y) || h.w.Solid(p) {
		t.Errorf("got off at %v, not on %s's platforms", p, in.Name(r.to))
	}
	if h.e.ride != nil {
		t.Error("still riding")
	}
	// The train waits at the platform behind the station.
	h.tick(input.Frame{})
	if len(h.e.trainCells) == 0 {
		t.Error("no train to be seen at the station")
	}
}

// TestWindowViewScrollsOneWay: on the way there and back, the scenery
// moves the same way past the windows (the carriage always goes right).
func TestWindowViewScrollsOneWay(t *testing.T) {
	h, in := newCrowdHarness(t)
	city, _, _ := stationDoor(t, in)
	line := h.e.line(in.Edges(infra.Rail, city)[0])
	for _, toB := range []bool{true, false} {
		h.e.ride = &ride{line: line, toB: toB}
		// The start of the run in that direction.
		arrive, leave := line.NextStop(0, toB)
		_ = arrive
		h.e.clock.AdvanceTicks(leave - h.e.clock.Ticks())
		prev := -1e18
		for k := 0; k < 3000; k++ {
			s, heading, ok := h.e.viewTrack(0)
			if !ok {
				t.Fatal("no view")
			}
			if u := heading * s; u < prev-1e-9 {
				t.Fatalf("toB=%v, tick %d: the view went back (%.3f after %.3f)", toB, k, u, prev)
			} else {
				prev = u
			}
			h.e.clock.Tick()
		}
	}
}
