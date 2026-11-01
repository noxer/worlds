package scene

import (
	"image"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// harness runs the real scene stack on the shipped content, tick by tick,
// with injected input. Nothing is drawn.
type harness struct {
	t     *testing.T
	w     *world.World
	in    *input.State
	stack *Stack
	e     *Explore
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	w, err := world.Load(assets.FS, "maps")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := dialog.Load(assets.FS, "dialog")
	if err != nil {
		t.Fatal(err)
	}
	party := entity.NewParty(&entity.Actor{Name: "leader"})
	for i := 0; i < 3; i++ {
		if err := party.Add(&entity.Actor{}); err != nil {
			t.Fatal(err)
		}
	}
	start, _ := w.Map("town").Spawn("start")
	e, err := NewExplore(ExploreConfig{World: w, Sheets: gfx.NewSheets(assets.FS), Dialogs: lib, Party: party, Start: start})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, w: w, in: &input.State{}, e: e}
	h.stack = NewStack(h.in, e)
	h.stack.SetSize(320, 180)
	h.tick(input.Frame{})
	return h
}

func (h *harness) tick(f input.Frame) {
	h.in.Apply(f)
	h.stack.Update()
}

func (h *harness) press(a input.Action) {
	var f input.Frame
	f.Held[a] = true
	h.tick(f)
	h.tick(input.Frame{})
}

func (h *harness) hold(a input.Action, ticks int) {
	var f input.Frame
	f.Held[a] = true
	for i := 0; i < ticks; i++ {
		h.tick(f)
	}
	h.tick(input.Frame{})
}

func (h *harness) runUntil(what string, max int, cond func() bool) {
	h.t.Helper()
	for i := 0; i < max; i++ {
		if cond() {
			return
		}
		h.tick(input.Frame{})
	}
	h.t.Fatalf("timed out waiting for %s; leader at %v", what, h.leader().Pos)
}

func (h *harness) leader() *entity.Actor { return h.e.party.Leader() }

func (h *harness) top() Scene { return h.stack.scenes[len(h.stack.scenes)-1] }

func (h *harness) npc(name string) *NPC {
	for _, n := range h.e.npcs {
		if n.Name == name {
			return n
		}
	}
	h.t.Fatalf("no npc %q", name)
	return nil
}

func (h *harness) idle() bool {
	for _, m := range h.e.party.Members {
		if m.Moving() {
			return false
		}
	}
	return h.e.fade == 0 && !h.e.hasGoal
}

// checkQueue verifies the party forms a connected queue.
func (h *harness) checkQueue() {
	h.t.Helper()
	ms := h.e.party.Members
	for i := 1; i < len(ms); i++ {
		if ms[i].Pos == ms[i-1].Pos {
			continue
		}
		if _, ok := h.w.DirBetween(ms[i].Pos, ms[i-1].Pos); !ok {
			h.t.Errorf("member %d at %v is not next to member %d at %v", i, ms[i].Pos, i-1, ms[i-1].Pos)
		}
	}
}

func TestKeyboardTurnThenWalk(t *testing.T) {
	h := newHarness(t)
	start := h.leader().Pos
	// A short tap only turns on the spot.
	h.press(input.Right)
	h.runUntil("idle", 100, h.idle)
	if h.leader().Pos != start || h.leader().Facing != world.Right {
		t.Fatalf("tap should only turn: at %v facing %v", h.leader().Pos, h.leader().Facing)
	}
	// Holding walks; three tiles take 3*StepTicks.
	h.hold(input.Right, 3*entity.StepTicks)
	h.runUntil("idle", 100, h.idle)
	if got := h.leader().Pos; got.X != start.X+3 || got.Y != start.Y {
		t.Fatalf("leader at %v, want 3 tiles right of %v", got, start)
	}
	h.checkQueue()
}

func TestNPCBlocksMovement(t *testing.T) {
	h := newHarness(t)
	elder := h.npc("elder")
	above, _ := h.w.Neighbor(elder.Pos, world.Up)
	h.e.party.PlaceAll(above, world.Down)
	h.hold(input.Down, 3*entity.StepTicks)
	if h.leader().Pos != above {
		t.Fatalf("walked into the elder: %v", h.leader().Pos)
	}
}

func TestTalkToNPC(t *testing.T) {
	h := newHarness(t)
	elder := h.npc("elder")
	h.e.goTo(elder.Pos)
	h.runUntil("dialog", 2000, func() bool { _, ok := h.top().(*DialogBox); return ok })

	if d, ok := h.w.DirBetween(h.leader().Pos, elder.Pos); !ok || d != h.leader().Facing {
		t.Fatalf("leader at %v facing %v is not facing the elder at %v", h.leader().Pos, h.leader().Facing, elder.Pos)
	}
	if d, _ := h.w.DirBetween(elder.Pos, h.leader().Pos); elder.Facing != d {
		t.Fatalf("elder should turn to the leader")
	}

	box := h.top().(*DialogBox)
	// The typewriter reveals one character per tick.
	h.runUntil("first line typed", 200, box.typed)
	if box.r.Line() != "Welcome to Brookhaven, travellers." {
		t.Fatalf("unexpected first line %q", box.r.Line())
	}
	h.press(input.Confirm)
	h.runUntil("second line typed", 200, box.typed)
	if len(box.choices()) != 3 {
		t.Fatalf("expected 3 choices, got %d", len(box.choices()))
	}
	// Pick "Farewell." (last choice, reached by wrapping upwards).
	h.press(input.Up)
	h.press(input.Confirm)
	if h.top() != Scene(h.e) {
		t.Fatalf("dialog should be closed")
	}
}

func TestTapToMove(t *testing.T) {
	h := newHarness(t)
	target := h.leader().Pos
	target.X -= 2
	_, rx, ry := h.w.ToRegion(target)
	p := image.Pt(rx*tile-h.e.camX+8, ry*tile-h.e.camY+8)
	h.tick(input.Frame{Tap: p, Tapped: true})
	h.runUntil("arrival", 500, func() bool { return h.idle() && h.leader().Pos == target })
}

// TestTravelThroughWarpAndAcrossEdge sends the party from inside the smithy
// to the far side of the meadow: out through the door (warp), along the
// street and over the edge between the town and meadow maps.
func TestTravelThroughWarpAndAcrossEdge(t *testing.T) {
	h := newHarness(t)
	in, _ := h.w.Map("smithy").Spawn("entrance")
	h.e.party.PlaceAll(in.Pos, world.Up)
	goal := world.Pos{Map: "meadow", X: 20, Y: 17}
	h.e.goTo(goal)

	var maps []world.MapID
	h.runUntil("arrival in the meadow", 5000, func() bool {
		if m := h.leader().Pos.Map; len(maps) == 0 || maps[len(maps)-1] != m {
			maps = append(maps, m)
		}
		h.checkQueue()
		return h.idle() && h.leader().Pos == goal
	})
	want := []world.MapID{"smithy", "town", "meadow"}
	if len(maps) != len(want) {
		t.Fatalf("visited maps %v, want %v", maps, want)
	}
	for i := range want {
		if maps[i] != want[i] {
			t.Fatalf("visited maps %v, want %v", maps, want)
		}
	}
	// Followers queued up behind the leader in the meadow.
	h.checkQueue()
	for i, m := range h.e.party.Members[1:] {
		if m.Pos == goal {
			t.Errorf("follower %d still stacked on the leader", i+1)
		}
	}
}

func TestEnterBuildingThroughDoor(t *testing.T) {
	h := newHarness(t)
	goal := world.Pos{Map: "smithy", X: 6, Y: 3}
	h.e.goTo(goal)
	h.runUntil("arrival in the smithy", 3000, func() bool { return h.idle() && h.leader().Pos == goal })
}
