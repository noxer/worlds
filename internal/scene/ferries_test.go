package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// ferryHarbour returns a harbour with a ferry, its pier and the lane.
func ferryHarbour(t *testing.T, in *infra.Infra) (infra.Settlement, infra.Pier, infra.Edge) {
	t.Helper()
	for _, s := range in.Near(0, 193770, 400000, infra.Town) {
		p, ok := in.Harbour(s)
		if !ok {
			continue
		}
		for _, e := range in.Edges(infra.SeaLane, s) {
			if len(in.FerryRoute(e)) >= 2 {
				return s, p, e
			}
		}
	}
	t.Fatal("no ferry")
	return infra.Settlement{}, infra.Pier{}, infra.Edge{}
}

func TestFerryJourney(t *testing.T) {
	h, in := newCrowdHarness(t)
	town, pier, edge := ferryHarbour(t, in)
	end := pier.Cells[len(pier.Cells)-1]
	facing := map[[2]int64]world.Dir{{0, -1}: world.Up, {1, 0}: world.Right, {0, 1}: world.Down, {-1, 0}: world.Left}[[2]int64{pier.DX, pier.DY}]
	h.e.party.PlaceAll(world.Pos{Map: world.Main, X: end.X, Y: end.Y}, facing)
	h.e.nearLines = nil
	h.tick(input.Frame{})
	line := h.e.line(edge)
	if line == nil || line.Kind != transit.Ferry {
		t.Fatalf("no ferry line: %+v", line)
	}

	// At the end of the pier: the harbour's timetable (or, with the ferry
	// alongside, straight aboard).
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); ok {
		h.choose("wait")
	}
	h.runUntil("boarding", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == ferryMap })
	r := h.e.ride
	if r == nil || r.from.ID != town.ID || r.line.Kind != transit.Ferry {
		t.Fatalf("not on the ferry from %v: %+v", town.ID, r)
	}

	// Before it casts off, back down the gangway onto the pier, and aboard
	// again by walking into the moored ferry.
	if r.line.At(float64(h.e.clock.Ticks())).Moving {
		t.Fatal("cast off at once")
	}
	var gangway world.Pos
	for p, id := range h.e.bumps {
		if id == "ferry_exit" && p.Map == h.leader().Pos.Map {
			gangway = p
			break
		}
	}
	above, _ := h.w.Neighbor(gangway, world.Up)
	h.e.party.PlaceAll(above, world.Down)
	h.press(input.Down)
	h.runUntil("going ashore", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	p := h.leader().Pos
	if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
		t.Fatalf("ashore at %v: not walkable land", p)
	}
	h.tick(input.Frame{})
	var boat world.Pos
	var toBoat world.Dir
	for _, d := range []world.Dir{world.Up, world.Right, world.Down, world.Left} {
		if q, _ := h.w.Neighbor(p, d); func() bool { _, ok := h.e.trainAt(q); return ok }() {
			boat, toBoat = q, d
		}
	}
	if boat == (world.Pos{}) {
		t.Fatalf("the ferry isn't alongside %v", p)
	}
	h.e.party.PlaceAll(p, toBoat)
	h.press(map[world.Dir]input.Action{world.Up: input.Up, world.Right: input.Right, world.Down: input.Down, world.Left: input.Left}[toBoat])
	h.runUntil("boarding again", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == ferryMap })
	r = h.e.ride

	// The gangway stays up at sea.
	for !r.line.At(float64(h.e.clock.Ticks())).Moving {
		h.tick(input.Frame{})
	}
	h.e.leaveTrain(false)
	if h.e.fade != 0 || h.leader().Pos.Map.Template != ferryMap {
		t.Fatal("got off at sea")
	}
	// The sea view draws without trouble.
	if _, _, ok := h.e.viewTrack(0); !ok {
		t.Fatal("no view")
	}

	// A rest on a bench until the other harbour.
	var bench world.Pos
	for p, id := range h.e.signs {
		if id == "ferry_bench" && p.Map == h.leader().Pos.Map {
			bench = p
			break
		}
	}
	above, _ = h.w.Neighbor(bench, world.Up)
	h.e.party.PlaceAll(above, world.Down)
	h.press(input.Confirm)
	h.choose("rest")
	h.tick(input.Frame{})
	h.tick(input.Frame{})
	if !h.e.ride.arrived || h.e.notice == nil || !strings.Contains(h.e.notice.text, in.Name(r.to)) {
		t.Fatalf("not arrived at %s: %+v, notice %+v", in.Name(r.to), h.e.ride, h.e.notice)
	}

	// Ashore on the other pier.
	h.e.leaveTrain(false)
	h.runUntil("going ashore", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	other, _ := in.Harbour(r.to)
	p = h.leader().Pos
	if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
		t.Errorf("ashore at %v: not walkable land", p)
	}
	if dx, dy := p.X-other.Tip.X, p.Y-other.Tip.Y; dx*dx+dy*dy > 9 {
		t.Errorf("ashore at %v, far from %s's pier (tip %v)", p, in.Name(r.to), other.Tip)
	}
	// The ferry lies alongside.
	h.e.nearLines = nil
	h.tick(input.Frame{})
	if len(h.e.trainCells) == 0 {
		t.Error("no ferry to be seen at the pier")
	}
}
