package scene

import (
	"image"
	"testing"

	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// TestTapOnDoorEntersOnce: a tap on a building's door walks the party in,
// and it stays in (it used to walk back out to the door, and in again).
func TestTapOnDoorEntersOnce(t *testing.T) {
	h := newHarness(t)
	sp, ok := h.w.Spawn(world.MainMap, "smithy_door")
	if !ok {
		t.Fatal("no spawn at the smithy's door")
	}
	door, _ := h.w.Neighbor(sp.Pos, world.Up) // the warp into the smithy
	if h.w.WarpAt(door) == nil {
		t.Fatalf("no warp at %v", door)
	}
	start, _ := h.w.Offset(door, 0, 3)
	h.e.party.PlaceAll(start, world.Up)
	h.tick(input.Frame{})
	x, y := h.e.screen(door.Map, door.X*tile, door.Y*tile)
	h.tick(input.Frame{Tap: image.Pt(x+8, y+8), Tapped: true})
	h.runUntil("inside", 600, func() bool { return h.e.fade == 0 && h.leader().Pos.Map.Template == "smithy" })
	inside := h.leader().Pos
	for k := 0; k < 600; k++ {
		h.tick(input.Frame{})
		if !h.leader().Pos.Map.IsMain() {
			continue
		}
		t.Fatalf("the party walked back out after %d ticks (to %v)", k, h.leader().Pos)
	}
	if h.leader().Pos != inside || h.e.hasGoal {
		t.Errorf("inside, the party went on to %v (goal %v)", h.leader().Pos, h.e.hasGoal)
	}
}
