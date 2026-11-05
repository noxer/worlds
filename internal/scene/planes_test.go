package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

func TestFlight(t *testing.T) {
	h, in := newCrowdHarness(t)
	a, ok := h.e.airportNear(h.e.home)
	if !ok {
		t.Fatal("no airport")
	}
	// A road leads out of the way in, towards the city.
	ent := a.Entrance()
	road := false
	for _, w := range in.WaysIn(ent.X, ent.Y+5, 1, 1) {
		road = road || w.Access
	}
	if !road {
		t.Error("no access road at the airport's way in")
	}
	for k := int64(0); k <= 6; k++ {
		p := world.Pos{Map: world.Main, X: ent.X, Y: ent.Y + k}
		if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
			t.Errorf("the way in is blocked at %v", p)
		}
	}
	b := a.Board()
	board := world.Pos{Map: world.Main, X: b.X, Y: b.Y}
	if id, ok := h.w.Bump(board); !ok || id != "airport" || !h.w.Solid(board) {
		t.Fatalf("no departures board at %v", board)
	}
	below, _ := h.w.Neighbor(board, world.Down)
	h.e.party.PlaceAll(below, world.Up)
	h.e.nearLines = nil
	h.tick(input.Frame{})
	// A plane stands at its gate while it boards: nose, body and tail.
	line := h.e.line(a.Flight[0])
	atA := a.Flight[0].A.ID == a.S.ID
	arrive, _ := line.NextStop(h.e.clock.Ticks(), atA)
	h.e.clock.AdvanceTicks(arrive + 60 - h.e.clock.Ticks())
	h.e.nearLines = nil
	h.tick(input.Frame{})
	g := a.Gate(0)
	for dx := int64(0); dx < 3; dx++ {
		if c, ok := h.e.trainAt(world.Pos{Map: world.Main, X: g.X + dx, Y: g.Y}); !ok || c.line != line {
			t.Errorf("gate 0: no plane on %d,%d", g.X+dx, g.Y)
		}
	}
	// Seen from the apron below the gate, the plane is drawn whole.
	h.e.party.PlaceAll(world.Pos{Map: world.Main, X: g.X + 1, Y: g.Y + 3}, world.Up)
	h.tick(input.Frame{})
	if d := h.e.trainDrawables(world.Main, 0); len(d) < 9 {
		t.Errorf("%d tiles drawn, want the plane's 9 at least", len(d))
	}
	h.e.party.PlaceAll(below, world.Up)
	h.tick(input.Frame{})
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); !ok {
		t.Fatalf("no departures (top: %T)", h.top())
	}
	h.choose("wait")
	h.runUntil("boarding", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == planeMap })
	r := h.e.ride
	if r == nil || r.line.Kind != transit.Plane || r.from.ID != a.S.ID {
		t.Fatalf("not on a plane from %v: %+v", a.S.ID, r)
	}
	for !r.line.At(float64(h.e.clock.Ticks())).Moving {
		h.tick(input.Frame{})
	}
	h.e.leaveTrain(false)
	if h.e.fade != 0 || h.leader().Pos.Map.Template != planeMap {
		t.Fatal("got off a moving plane")
	}
	var seat world.Pos
	for p, id := range h.e.signs {
		if id == "plane_seat" && p.Map == h.leader().Pos.Map && p.Y == 2 {
			seat = p
			break
		}
	}
	under, _ := h.w.Neighbor(seat, world.Down)
	h.e.party.PlaceAll(under, world.Up)
	h.press(input.Confirm)
	h.choose("rest")
	h.tick(input.Frame{})
	h.tick(input.Frame{})
	if !h.e.ride.arrived || h.e.notice == nil || !strings.Contains(h.e.notice.text, in.Name(r.to)) {
		t.Fatalf("not arrived at %s: %+v, notice %+v", in.Name(r.to), h.e.ride, h.e.notice)
	}
	h.e.leaveTrain(false)
	h.runUntil("getting off", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.IsMain() })
	p := h.leader().Pos
	if h.w.Solid(p) || h.w.Terrain(p) != world.Land {
		t.Errorf("off at %v: not walkable land", p)
	}
	other, _ := in.Airport(r.to)
	if dx, dy := p.X-other.X, p.Y-other.Y; dx < 0 || dy < 0 || dx >= 60 || dy >= 22 {
		t.Errorf("off at %v, not at %s's airport (%d,%d)", p, in.Name(r.to), other.X, other.Y)
	}
	h.tick(input.Frame{})
	if len(h.e.trainCells) == 0 {
		t.Error("no plane to be seen at the gate")
	}
}
