package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/world"
)

// cell is a tile of the cellar.
func cell(x, y int64) world.Pos { return world.Pos{Map: world.Detail("cellar"), X: x, Y: y} }

// walk presses a direction until the leader has stepped (or bumped) once.
func (h *harness) walk(d input.Action) {
	h.t.Helper()
	from := h.leader().Pos
	var f input.Frame
	f.Held[d] = true
	facing := h.leader().Facing
	for k := 0; k < 40; k++ {
		h.tick(f)
		// A step began (the position changes at once), or a bump: facing
		// that way and still there after the turn delay.
		if h.leader().Pos != from || (k > 12 && h.leader().Facing != facing) || k > 20 {
			break
		}
	}
	h.tick(input.Frame{})
	for k := 0; k < 40 && h.leader().Moving(); k++ {
		h.tick(input.Frame{})
	}
}

// walkTo walks the leader along a route of single steps.
func (h *harness) walkTo(steps ...input.Action) {
	h.t.Helper()
	for _, d := range steps {
		h.walk(d)
	}
}

// TestCellarPuzzle: the lever opens the first door; the crate pushed onto
// the plate opens the second; a save keeps both.
func TestCellarPuzzle(t *testing.T) {
	h := newBattleHarness(t)
	store := save.Memory{}
	h.e.saves = store
	door1, door2 := h.e.puzzle.byKey["cellar/door_1"], h.e.puzzle.byKey["cellar/door_2"]
	lever, crate := h.e.puzzle.byKey["cellar/lever_1"], h.e.puzzle.byKey["cellar/crate_1"]
	if door1 == nil || door2 == nil || lever == nil || crate == nil {
		t.Fatal("the cellar's pieces aren't all there")
	}
	// Down the hatch in the meadow.
	sp, ok := h.w.Spawn(world.MainMap, "cellar_door")
	if !ok {
		t.Fatal("no spawn by the cellar hatch")
	}
	hatch, _ := h.w.Neighbor(sp.Pos, world.Up)
	h.e.party.PlaceAll(sp.Pos, world.Up)
	h.walk(input.Up)
	h.runUntil("going down", 200, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == "cellar" })
	if h.leader().Pos != cell(7, 10) {
		t.Fatalf("down the hatch at %v, want the cellar's entrance (hatch %v)", h.leader().Pos, hatch)
	}
	// The first door is shut.
	if !h.w.Solid(door1.pos) || door1.on {
		t.Fatal("the first door is open from the start")
	}
	// To the lever (3,8): left 3, up 2 to (4,8), face left, pull.
	h.walkTo(input.Left, input.Left, input.Left, input.Up, input.Up)
	if h.leader().Pos != cell(4, 8) {
		t.Fatalf("at %v, want 4,8", h.leader().Pos)
	}
	h.walk(input.Left) // bumps the lever: it is solid
	if h.leader().Pos != cell(4, 8) || h.leader().Facing != world.Left {
		t.Fatalf("walked onto the lever: %v", h.leader().Pos)
	}
	h.press(input.Confirm)
	if !lever.on || !door1.on || h.w.Solid(door1.pos) || !h.e.story.Flag("lever.cellar/lever_1") {
		t.Fatalf("after pulling: lever %v, door %v", lever.on, door1.on)
	}
	// Through the door (8,7) into the store room: right 4 to (8,8), up.
	h.walkTo(input.Right, input.Right, input.Right, input.Right, input.Up)
	if h.leader().Pos != cell(8, 7) {
		t.Fatalf("at %v, want in the doorway 8,7", h.leader().Pos)
	}
	// The lever pulled back with someone in the doorway: it stays open.
	lever.on = false
	h.e.updatePuzzles()
	if !door1.on {
		t.Fatal("the door shut on the party")
	}
	lever.on = true
	h.e.updatePuzzles()
	// The crate (10,3) to the plate (12,6): from 9,3 push right twice,
	// from 12,2 push down three times.
	h.walkTo(input.Up, input.Up, input.Up, input.Up, input.Right)
	if h.leader().Pos != cell(9, 3) {
		t.Fatalf("at %v, want 9,3", h.leader().Pos)
	}
	h.walkTo(input.Right, input.Right)
	if crate.pos != cell(12, 3) || h.leader().Pos != cell(11, 3) {
		t.Fatalf("after pushing right twice: crate %v, leader %v", crate.pos, h.leader().Pos)
	}
	h.walkTo(input.Up, input.Right)
	if h.leader().Pos != cell(12, 2) {
		t.Fatalf("at %v, want 12,2", h.leader().Pos)
	}
	if door2.on {
		t.Fatal("the second door is open before the crate is on the plate")
	}
	h.walkTo(input.Down, input.Down)
	if crate.pos != cell(12, 5) || door2.on {
		t.Fatalf("crate at %v (door %v), want just short of the plate", crate.pos, door2.on)
	}
	h.walkTo(input.Down)
	if crate.pos != cell(12, 6) {
		t.Fatalf("crate at %v, want on the plate 12,6", crate.pos)
	}
	if !door2.on || h.w.Solid(door2.pos) {
		t.Fatal("the crate on the plate didn't open the second door")
	}
	// Against the wall it won't go.
	h.walkTo(input.Down)
	if crate.pos != cell(12, 6) || h.leader().Pos != cell(12, 5) {
		t.Fatalf("crate moved to %v, leader at %v", crate.pos, h.leader().Pos)
	}
	// Save, scramble, load.
	h.press(input.Menu)
	h.menuTo("Save")
	h.menuTo("1")
	h.e.moveBlock(crate, crate.home)
	h.e.story.SetFlag("lever.cellar/lever_1", false)
	h.e.restorePuzzles(nil)
	if door1.on || door2.on {
		t.Fatal("scrambling didn't shut the doors")
	}
	h.press(input.Menu)
	h.menuTo("Load")
	h.menuTo("1")
	h.runUntil("loading", 200, func() bool { return h.e.fade == 0 })
	if !lever.on || crate.pos != cell(12, 6) || !door1.on || !door2.on {
		t.Fatalf("after loading: lever %v, crate %v, doors %v %v", lever.on, crate.pos, door1.on, door2.on)
	}
	if !h.w.Solid(crate.pos) || h.w.Solid(crate.home) {
		t.Error("the crate's blocking didn't move with it")
	}
}

// TestPlateUnderTheParty: standing on a plate presses it; stepping off
// lets it up.
func TestPlateUnderTheParty(t *testing.T) {
	h := newBattleHarness(t)
	plate, door2 := h.e.puzzle.byKey["cellar/plate_1"], h.e.puzzle.byKey["cellar/door_2"]
	h.e.party.PlaceAll(plate.pos, world.Up)
	h.tick(input.Frame{})
	if !plate.on || !door2.on {
		t.Fatal("the party on the plate doesn't press it")
	}
	h.e.party.PlaceAll(cell(10, 6), world.Up)
	h.tick(input.Frame{})
	if plate.on || door2.on {
		t.Fatal("the plate stays down with nobody on it")
	}
}

// TestStuckCrateResets: a crate pushed into a corner goes back when the
// party leaves; one on its plate stays.
func TestStuckCrateResets(t *testing.T) {
	h := newBattleHarness(t)
	crate := h.e.puzzle.byKey["cellar/crate_1"]
	h.e.party.PlaceAll(cell(7, 10), world.Up)
	h.tick(input.Frame{})
	h.e.moveBlock(crate, cell(14, 1)) // in the corner
	h.e.warp = &world.Warp{To: h.e.home}
	h.e.fade = fadeTicks
	h.runUntil("leaving", 200, func() bool { return h.e.fade == 0 })
	if crate.pos != crate.home || !h.w.Solid(crate.home) || h.w.Solid(cell(14, 1)) {
		t.Fatalf("after leaving: crate at %v", crate.pos)
	}
	// On its plate, it stays.
	h.e.party.PlaceAll(cell(7, 10), world.Up)
	h.e.moveBlock(crate, cell(12, 6))
	h.e.warp = &world.Warp{To: h.e.home}
	h.e.fade = fadeTicks
	h.runUntil("leaving", 200, func() bool { return h.e.fade == 0 })
	if crate.pos != cell(12, 6) {
		t.Fatalf("the crate left its plate: %v", crate.pos)
	}
}
