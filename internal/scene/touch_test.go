package scene

import (
	"image"
	"testing"

	"github.com/noxer/worlds/internal/input"
)

func (h *harness) tapAt(p image.Point) {
	h.t.Helper()
	h.tick(input.Frame{Tap: p, Tapped: true})
	h.tick(input.Frame{})
}

func centre(r image.Rectangle) image.Point { return r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2)) }

// TestTouchOnly: with taps alone, the menu opens from its button, the map
// opens from the menu, zooms out with its button and closes, and a tap
// beside the menu closes it.
func TestTouchOnly(t *testing.T) {
	h, _ := newCrowdHarness(t)
	h.tick(input.Frame{})
	p := h.leader().Pos
	h.tapAt(centre(cornerButton(320, 0)))
	m, ok := h.top().(*Menu)
	if !ok {
		t.Fatalf("the menu button didn't open the menu (top: %T)", h.top())
	}
	if h.leader().Pos != p || h.e.path != nil {
		t.Error("the tap on the button also walked the party")
	}
	_, rows := m.rects()
	h.tapAt(centre(rows[1])) // Map
	wm, ok := h.top().(*WorldMap)
	if !ok {
		t.Fatalf("no map (top: %T)", h.top())
	}
	z := wm.zoom
	h.tapAt(centre(cornerButton(320, 1)))
	if wm.zoom != z-1 {
		t.Errorf("zoom %d after the minus button, want %d", wm.zoom, z-1)
	}
	h.tapAt(centre(cornerButton(320, 2)))
	if wm.zoom != z {
		t.Errorf("zoom %d after the plus button, want %d", wm.zoom, z)
	}
	h.tapAt(centre(cornerButton(320, 0)))
	if h.top() != Scene(h.e) {
		t.Fatalf("the close button didn't close the map (top: %T)", h.top())
	}
	h.tapAt(centre(cornerButton(320, 0)))
	if _, ok := h.top().(*Menu); !ok {
		t.Fatal("the menu didn't open again")
	}
	h.tapAt(image.Pt(300, 170)) // beside the menu
	if h.top() != Scene(h.e) {
		t.Fatalf("a tap beside the menu didn't close it (top: %T)", h.top())
	}
}
