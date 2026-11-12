package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/input"
)

func TestWorldMap(t *testing.T) {
	h, _ := newCrowdHarness(t)
	h.tick(input.Frame{})
	h.e.updateTaxis() // the start counts as visited
	h.press(input.Menu)
	h.menuTo("Map")
	m, ok := h.top().(*WorldMap)
	if !ok {
		t.Fatalf("no world map (top: %T)", h.top())
	}
	for k := 0; k < 200 && m.row < m.h; k++ {
		h.tick(input.Frame{})
	}
	if m.row < m.h {
		t.Fatal("the map never finished drawing")
	}
	// The party stands on land: the pixel under it is land.
	p := h.leader().Pos
	px, py := m.pixelOf(p.X, p.Y)
	if px != m.w/2 || py != m.h/2 {
		t.Errorf("the party is at pixel %d,%d, not the middle", px, py)
	}
	i := 4 * (py*m.w + px)
	if got := [3]byte{m.pix[i], m.pix[i+1], m.pix[i+2]}; got != [3]byte{mapLand.R, mapLand.G, mapLand.B} {
		t.Errorf("the party's pixel is %v, not land", got)
	}
	// Some sea in the widest view (the start is on a coast).
	h.press(input.Cancel)
	h.press(input.Cancel)
	if m.zoom != 0 {
		t.Fatalf("zoom %d after zooming out twice", m.zoom)
	}
	for k := 0; k < 400 && m.row < m.h; k++ {
		h.tick(input.Frame{})
	}
	sea := 0
	for i := 0; i < len(m.pix); i += 4 {
		if m.pix[i] == mapDeep.R && m.pix[i+1] == mapDeep.G {
			sea++
		}
	}
	if sea == 0 {
		t.Error("no sea on the whole world's map")
	}
	if len(m.marks) == 0 {
		t.Error("no metropolises on the whole world's map")
	}
	// Closer again: the start, a village, is marked as visited at every
	// zoom from the second.
	for z := 1; z < len(mapZooms); z++ {
		h.press(input.Confirm)
		h.tick(input.Frame{})
		visited := false
		for _, mk := range m.marks {
			visited = visited || mk.visited
		}
		if !visited {
			t.Errorf("zoom %d: no visited place among %d marks", z, len(m.marks))
		}
	}
	// Panning moves the view.
	cx := m.cx
	h.press(input.Right)
	if m.cx == cx {
		t.Error("panning right didn't move the view")
	}
	// Menu closes it; the world is still where it was.
	h.press(input.Menu)
	if h.top() != Scene(h.e) {
		t.Fatalf("the map stayed open (top: %T)", h.top())
	}
	if h.leader().Pos != p {
		t.Error("the party moved while the map was open")
	}
}
