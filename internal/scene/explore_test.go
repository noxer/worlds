package scene

import (
	"image"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
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
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	w := c.World
	party := entity.NewParty(&entity.Actor{Name: "leader"})
	for i := 0; i < 3; i++ {
		if err := party.Add(&entity.Actor{}); err != nil {
			t.Fatal(err)
		}
	}
	e, err := NewExplore(ExploreConfig{World: w, Sheets: gfx.NewSheets(assets.FS), Dialogs: c.Dialogs, Party: party, Start: c.Start})
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
	return !h.e.party.Busy() && h.e.fade == 0 && !h.e.hasGoal
}

// checkQueue verifies the party forms a connected queue. Followers lag a
// few ticks behind, so it compares where everyone is heading.
func (h *harness) checkQueue() {
	h.t.Helper()
	ms := h.e.party.Members
	for i := 1; i < len(ms); i++ {
		a, b := ms[i].Dest(), ms[i-1].Dest()
		if a == b {
			continue
		}
		if _, ok := h.w.DirBetween(a, b); !ok {
			h.t.Errorf("member %d heading to %v is not next to member %d heading to %v", i, a, i-1, b)
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
	if want, _ := h.w.Offset(start, 3, 0); h.leader().Pos != want {
		t.Fatalf("leader at %v, want 3 tiles right of %v", h.leader().Pos, start)
	}
	h.checkQueue()
}

// TestCameraDoesNotBob checks that the view moves smoothly with the leader:
// walking sideways must never move the camera vertically (the walk hop only
// moves the sprite).
func TestCameraDoesNotBob(t *testing.T) {
	h := newHarness(t)
	h.press(input.Right) // turn first
	h.runUntil("idle", 100, h.idle)
	y := h.e.camY
	lastX := h.e.camX
	var f input.Frame
	f.Held[input.Right] = true
	for i := 0; i < 3*entity.StepTicks; i++ {
		h.tick(f)
		if h.e.camY != y {
			t.Fatalf("tick %d: camera moved vertically from %d to %d", i, y, h.e.camY)
		}
		if d := h.e.camX - lastX; d < 0 || d > 1 {
			t.Fatalf("tick %d: camera moved %d px horizontally, want 0 or 1", i, d)
		}
		lastX = h.e.camX
	}
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
	target, _ := h.w.Offset(h.leader().Pos, -2, 1)
	x, y := h.e.screen(target.Map, target.X*tile, target.Y*tile)
	p := image.Pt(x+8, y+8)
	h.tick(input.Frame{Tap: p, Tapped: true})
	h.runUntil("arrival", 500, func() bool { return h.idle() && h.leader().Pos == target })
}

// TestTravelThroughWarpAndAcrossSeam sends the party from inside the smithy
// to the meadow: out through the door (warp), along the street, across the
// east-west wrap seam of the world (the town straddles it) into the meadow.
func TestTravelThroughWarpAndAcrossSeam(t *testing.T) {
	h := newHarness(t)
	in, _ := h.w.Spawn("smithy", "entrance")
	h.e.party.PlaceAll(in.Pos, world.Up)
	meadow, _ := h.w.PatchOrigin("meadow")
	goal, _ := h.w.Offset(meadow, 20, 17)
	h.e.goTo(goal)

	var maps []world.MapRef
	crossed := false
	last := h.leader().Pos
	h.runUntil("arrival in the meadow", 5000, func() bool {
		l := h.leader().Pos
		if len(maps) == 0 || maps[len(maps)-1] != l.Map {
			maps = append(maps, l.Map)
		}
		if l.Map.IsMain() && last.Map.IsMain() && last.X == h.w.W-1 && l.X == 0 {
			crossed = true
		}
		last = l
		h.checkQueue()
		return h.idle() && l == goal
	})
	if len(maps) != 2 || maps[0] != world.Detail("smithy") || maps[1] != world.Main {
		t.Fatalf("visited maps %v, want [smithy main]", maps)
	}
	if !crossed {
		t.Fatal("the route should cross the wrap seam")
	}
	h.checkQueue()
	for i, m := range h.e.party.Members[1:] {
		if m.Pos == goal {
			t.Errorf("follower %d still stacked on the leader", i+1)
		}
	}
}

func TestEnterBuildingThroughDoor(t *testing.T) {
	h := newHarness(t)
	goal := world.Pos{Map: world.Detail("smithy"), X: 6, Y: 3}
	h.e.goTo(goal)
	h.runUntil("arrival in the smithy", 3000, func() bool { return h.idle() && h.leader().Pos == goal })
}

// TestBumpShowsPassiveNotice walks into the locked door: a message appears
// without opening a dialog, input keeps working, and it goes away when the
// party walks off.
func TestBumpShowsPassiveNotice(t *testing.T) {
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	door, _ := h.w.Offset(town, 22, 5)
	below, _ := h.w.Offset(door, 0, 1)
	h.e.party.PlaceAll(below, world.Up)

	h.hold(input.Up, 5)
	if h.e.notice == nil || h.e.notice.text != "The door is locked." {
		t.Fatalf("expected the locked-door notice, got %+v", h.e.notice)
	}
	if h.top() != Scene(h.e) {
		t.Fatal("bumping must not open a modal dialog")
	}
	if h.leader().Pos != below {
		t.Fatal("walked into the locked door")
	}
	// It stays while the party stands still.
	for i := 0; i < 120; i++ {
		h.tick(input.Frame{})
	}
	if h.e.notice == nil {
		t.Fatal("notice vanished without moving")
	}
	// Walking away clears it straight away, no confirmation needed.
	var f input.Frame
	f.Held[input.Down] = true
	h.tick(f)
	for i := 0; i < turnDelay+2 && h.e.notice != nil; i++ {
		h.tick(f)
	}
	if h.e.notice != nil {
		t.Fatal("notice should disappear when walking away")
	}

	// Facing the door and pressing confirm still opens the full dialog.
	h.e.party.PlaceAll(below, world.Up)
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); !ok {
		t.Fatal("confirm at the door should open the dialog")
	}
}
