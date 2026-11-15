package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// TestGeneratedDoorIsLocked: walking into the door of a generated house
// shows the locked notice; confirm opens the dialog.
func TestGeneratedDoorIsLocked(t *testing.T) {
	h := newHarness(t)
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	town, _ := h.w.PatchOrigin("town")
	var door world.Pos
	found := false
	for _, site := range c.Infra.SitesIn(town.X-6000, town.Y-6000, 12000, 12000) {
		for _, b := range site.Buildings {
			for _, o := range h.w.Prefabs()[b.Prefab].Objects {
				if o.Type == "door" && !found {
					door, _ = h.w.Normalize(world.Pos{Map: world.Main, X: b.X + o.Pos.X, Y: b.Y + o.Pos.Y})
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("no generated house near Brookhaven")
	}
	step, _ := h.w.Neighbor(door, world.Down)
	h.e.party.PlaceAll(step, world.Up)
	h.hold(input.Up, 5)
	if h.e.notice == nil || !strings.Contains(h.e.notice.text, "locked") {
		t.Fatalf("expected the locked notice at %v, got %+v", door, h.e.notice)
	}
	if h.leader().Pos != step {
		t.Fatal("walked into the door")
	}
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); !ok {
		t.Fatal("confirm at the door should open the dialog")
	}
}
