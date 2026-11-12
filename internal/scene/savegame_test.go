package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/world"
)

// menuTo presses Down until the menu's cursor is on the item with text, and
// confirms it.
func (h *harness) menuTo(text string) {
	h.t.Helper()
	m, ok := h.top().(*Menu)
	if !ok {
		h.t.Fatalf("no menu open (top: %T)", h.top())
	}
	for range 10 {
		if its := m.items(); m.cursor < len(its) && len(its[m.cursor].text) >= len(text) && its[m.cursor].text[:len(text)] == text {
			h.press(input.Confirm)
			return
		}
		h.press(input.Down)
	}
	h.t.Fatalf("no menu item %q in %v", text, m.items())
}

// TestSaveAndLoad: the menu saves the game into a slot; after the party
// has moved on, the clock run and the order changed, loading puts it all
// back.
func TestSaveAndLoad(t *testing.T) {
	h, _ := newCrowdHarness(t)
	store := save.Memory{}
	h.e.saves = store
	for _, n := range []string{"Bryn", "Cato"} {
		if err := h.e.party.Add(&entity.Actor{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	// Somewhere in a village, riding, after a while.
	st, ok := h.e.busStopNear(h.e.home)
	if !ok {
		t.Fatal("no bus stop")
	}
	at := world.Pos{Map: world.Main, X: st.Stop.X, Y: st.Stop.Y}
	h.e.party.PlaceAll(at, world.Left)
	h.e.clock.Advance(500)
	h.e.setRiding(true)
	if !h.e.riding {
		t.Fatal("can't ride at the bus stop")
	}
	h.e.updateTaxis() // notes the town as visited
	h.tick(input.Frame{})
	visited := len(h.e.visited)
	ticks := h.e.clock.Ticks()

	h.press(input.Menu)
	h.menuTo("Save")
	h.menuTo("2")
	if h.top() != Scene(h.e) {
		t.Fatalf("the menu stayed open after saving (top: %T)", h.top())
	}
	if _, err := store.Load(1); err != nil {
		t.Fatalf("nothing in slot 2: %v", err)
	}
	if l := save.List(store); l[1] == nil || l[1].Place == "" || l[1].Ticks != ticks {
		t.Fatalf("slot 2 lists %+v", l[1])
	}

	// Elsewhere, later, Cato leading, on foot, forgetting where we've been.
	h.e.setRiding(false)
	h.e.party.PlaceAll(h.e.home, world.Up)
	h.e.party.Members[0], h.e.party.Members[2] = h.e.party.Members[2], h.e.party.Members[0]
	h.e.clock.Advance(300)
	clear(h.e.visited)

	h.press(input.Cancel) // Cancel opens the menu too
	h.menuTo("Load")
	m := h.top().(*Menu)
	if m.cursor != 1 {
		t.Errorf("load starts on slot %d, not the newest save (2)", m.cursor+1)
	}
	h.menuTo("2")
	h.runUntil("loading", 200, func() bool { return h.e.fade == 0 && h.leader().Pos == at })
	if h.e.clock.Ticks() != ticks {
		t.Errorf("clock %d, saved %d", h.e.clock.Ticks(), ticks)
	}
	if h.leader().Name != "leader" || h.e.party.Members[1].Name != "Bryn" || h.e.party.Members[2].Name != "Cato" {
		t.Errorf("party order %s, %s, %s", h.leader().Name, h.e.party.Members[1].Name, h.e.party.Members[2].Name)
	}
	if h.leader().Facing != world.Left || !h.e.riding {
		t.Errorf("facing %v, riding %v; saved left, riding", h.leader().Facing, h.e.riding)
	}
	if len(h.e.visited) != visited || visited == 0 {
		t.Errorf("%d places visited, saved %d", len(h.e.visited), visited)
	}
}

// TestNoSavingOnTheWay: the game isn't saved during a journey.
func TestNoSavingOnTheWay(t *testing.T) {
	h, _ := newCrowdHarness(t)
	h.e.saves = save.Memory{}
	h.e.ride = &ride{}
	if _, err := h.e.Snapshot(); err == nil {
		t.Fatal("saved during a ride")
	}
	h.press(input.Menu)
	h.menuTo("Save")
	h.menuTo("1")
	if m, ok := h.top().(*Menu); !ok || m.msg == "" {
		t.Errorf("no message why not (top: %T)", h.top())
	}
}
