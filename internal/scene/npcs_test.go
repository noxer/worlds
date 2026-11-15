package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// spawnPos looks up a spawn point of the shipped content.
func (h *harness) spawnPos(name string) world.Pos {
	h.t.Helper()
	sp, ok := h.w.Spawn(world.MainMap, name)
	if !ok {
		h.t.Fatalf("no spawn %q", name)
	}
	return sp.Pos
}

// setTime sets the world clock.
func (h *harness) setTime(hour, minute int) { h.e.clock = sim.NewClock(1, hour, minute) }

// minutes returns the number of ticks for game minutes.
func minutes(m int) int { return m * sim.TicksPerMinute }

func TestScheduledNPCWalksRoutine(t *testing.T) {
	h := newHarness(t)
	h.setTime(8, 25)
	h.tick(input.Frame{})
	h.e.refreshReach()
	lina := h.npc("lina")
	if !lina.active || lina.Pos != h.spawnPos("lina_home") {
		t.Fatalf("at 08:25 lina should be home: active %v at %v", lina.active, lina.Pos)
	}
	// She leaves at 08:30 and walks to the square in a few game minutes.
	square := h.spawnPos("square")
	h.runUntil("lina at the square", minutes(12), func() bool { return lina.Pos == square && !lina.Moving() })
	if m := h.e.clock.Minutes() % sim.MinutesPerDay; m < 8*60+30 {
		t.Errorf("lina left before 08:30 (arrived at minute %d)", m)
	}
	if !h.e.blocked(square) {
		t.Error("an active NPC must block its tile")
	}
}

// TestScheduledNPCOutOfReach: away from the party, an NPC isn't simulated
// tile by tile; when the party comes back, it is where its schedule says.
func TestScheduledNPCOutOfReach(t *testing.T) {
	h := newHarness(t)
	h.setTime(8, 0)
	h.e.refreshReach()
	lina := h.npc("lina")
	if !lina.active {
		t.Fatal("lina should be active near the start")
	}

	// Into the smithy: another map, so lina (in town) is out of reach.
	smithy, ok := h.w.Spawn(world.MapID("smithy"), "entrance")
	if !ok {
		t.Fatal("no smithy entrance")
	}
	h.e.party.PlaceAll(smithy.Pos, world.Up)
	h.e.refreshReach()
	if lina.active {
		t.Fatal("lina should be out of reach while the party is indoors")
	}

	// 10:00: she should stand at the square, not at home.
	h.setTime(10, 0)
	h.e.party.PlaceAll(h.spawnPos("smithy_door"), world.Down)
	h.e.refreshReach()
	if !lina.active || lina.Pos != h.spawnPos("square") {
		t.Fatalf("at 10:00 lina should be at the square: active %v at %v", lina.active, lina.Pos)
	}

	// 11:30: she is at the smithy counter, which is in reach once the party
	// is inside too.
	h.setTime(11, 30)
	h.e.party.PlaceAll(smithy.Pos, world.Up)
	h.e.refreshReach()
	counter, _ := h.w.Spawn(world.MapID("smithy"), "counter")
	if !lina.active || lina.Pos != counter.Pos {
		t.Fatalf("at 11:30 lina should be at the smithy counter: active %v at %v", lina.active, lina.Pos)
	}
}

// TestScheduledNPCHandoverOffScreen: walking towards an NPC on the main
// map, it comes into play far outside the screen, at the place its schedule
// gives for the time.
func TestScheduledNPCHandoverOffScreen(t *testing.T) {
	h := newHarness(t)
	h.setTime(9, 0)
	square := h.spawnPos("square")
	far, _ := h.w.Offset(square, -2*npcReach, 0)
	h.e.party.PlaceAll(far, world.Right)
	h.e.refreshReach()
	lina := h.npc("lina")
	if lina.active {
		t.Fatal("lina should be out of reach")
	}
	// Move the party towards the square at riding speed (a tile every 8
	// ticks); the terrain doesn't matter here.
	for i := 0; !lina.active; i++ {
		if i > 2*npcReach*8 {
			t.Fatal("lina never came into reach")
		}
		if i%8 == 0 {
			p, _ := h.w.Offset(h.leader().Pos, 1, 0)
			h.e.party.PlaceAll(p, world.Right)
		}
		h.tick(input.Frame{})
	}
	want := sim.Where(h.e.planner, h.w, lina.schedule, h.e.clock.Minutes())
	if lina.Pos != want {
		t.Errorf("lina appeared at %v, schedule says %v", lina.Pos, want)
	}
	dx, dy, _ := h.w.Delta(h.leader().Pos, lina.Pos)
	halfScreen := int64(320 / world.TileSize / 2)
	if d := max(geom.Abs(dx), geom.Abs(dy)); d <= halfScreen*2 {
		t.Errorf("lina appeared %d tiles from the leader, that's on or near the screen", d)
	}
}

// TestScheduledNPCKeepsOffParty: standing on an NPC's destination makes it
// wait next to it instead of walking into the party.
func TestScheduledNPCKeepsOffParty(t *testing.T) {
	h := newHarness(t)
	h.setTime(8, 29)
	square := h.spawnPos("square")
	h.e.party.PlaceAll(square, world.Down)
	h.e.refreshReach()
	lina := h.npc("lina")
	for i := 0; i < minutes(10); i++ {
		h.tick(input.Frame{})
		for _, m := range h.e.party.Members {
			if m.Pos == lina.Pos {
				t.Fatalf("lina walked onto a party member at %v", m.Pos)
			}
		}
	}
	if _, ok := h.w.DirBetween(lina.Pos, square); !ok {
		t.Errorf("lina should wait next to the square, is at %v", lina.Pos)
	}
}
