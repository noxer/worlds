package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/world"
)

// talkTo stands the party next to the NPC called name, facing it, and
// talks until the dialog is over.
func (h *harness) talkTo(name string) {
	h.t.Helper()
	for _, n := range h.e.npcs {
		if n.Name != name {
			continue
		}
		for _, d := range world.Dirs {
			p, ok := h.w.Neighbor(n.Pos, d)
			if !ok || h.w.Solid(p) || h.w.Terrain(p) != world.Land || h.e.npcAt(p) != nil {
				continue
			}
			h.e.party.PlaceAll(p, d.Opposite())
			h.press(input.Confirm)
			if _, ok := h.top().(*DialogBox); !ok {
				h.t.Fatalf("no dialog with %s (top: %T)", name, h.top())
			}
			for k := 0; k < 400 && h.top() != Scene(h.e); k++ {
				h.press(input.Confirm)
			}
			if h.top() != Scene(h.e) {
				h.t.Fatalf("the dialog with %s never ended", name)
			}
			return
		}
		h.t.Fatalf("nowhere to stand next to %s", name)
	}
	h.t.Fatalf("no NPC called %s", name)
}

// TestStoryInDialogs: the wanderer hands over a honey cake, once; the
// party is told; the menu lists it; a save keeps it and what was said.
func TestStoryInDialogs(t *testing.T) {
	h := newBattleHarness(t)
	h.e.combat = nil // no fights here: the slimes are close by
	store := save.Memory{}
	h.e.saves = store

	h.talkTo("wanderer")
	if n := h.e.story.Count("honey_cake"); n != 1 || !h.e.story.Flag("wanderer_cake") {
		t.Fatalf("after talking: %d cakes, flag %v", n, h.e.story.Flag("wanderer_cake"))
	}
	if h.e.notice == nil || h.e.notice.text != "Received: Honey cake." {
		t.Errorf("notice %+v, want the cake announced", h.e.notice)
	}
	h.talkTo("wanderer")
	if n := h.e.story.Count("honey_cake"); n != 1 {
		t.Fatalf("a second talk gave another cake: %d", n)
	}
	if h.e.notice != nil && strings.Contains(h.e.notice.text, "Received") {
		t.Errorf("a second talk announced %q", h.e.notice.text)
	}

	// The menu's Items page.
	h.press(input.Menu)
	h.menuTo("Items")
	m := h.top().(*Menu)
	if its := m.items(); len(its) != 2 || its[0].text != "Honey cake" {
		t.Fatalf("the items page lists %v", its)
	}
	if its := m.items(); !strings.Contains(its[m.cursor].about, "Brookhaven") {
		t.Errorf("about the cake: %q", its[m.cursor].about)
	}
	h.press(input.Cancel)
	// Save, forget everything, load.
	h.menuTo("Save")
	h.menuTo("1")
	h.e.story.Restore(nil, nil)
	h.press(input.Menu)
	h.menuTo("Load")
	h.menuTo("1")
	h.runUntil("loading", 200, func() bool { return h.e.fade == 0 })
	if n := h.e.story.Count("honey_cake"); n != 1 || !h.e.story.Flag("wanderer_cake") {
		t.Fatalf("after loading: %d cakes, flag %v", n, h.e.story.Flag("wanderer_cake"))
	}
}

// TestDefeatedEnemiesStayGone: enemies beaten are saved; loading a game
// from before brings them back.
func TestDefeatedEnemiesStayGone(t *testing.T) {
	h := newBattleHarness(t)
	h.e.combat = nil
	store := save.Memory{}
	h.e.saves = store
	save1 := func(slot string) {
		h.press(input.Menu)
		h.menuTo("Save")
		h.menuTo(slot)
	}
	load := func(slot string) {
		h.press(input.Menu)
		h.menuTo("Load")
		h.menuTo(slot)
		h.runUntil("loading", 200, func() bool { return h.e.fade == 0 })
	}
	foes := h.enemies()
	save1("1") // everyone still there
	// As after a won fight.
	for _, n := range foes[:2] {
		n.active, n.defeated = false, true
		h.e.story.SetFlag(defeatedFlag(n), true)
	}
	save1("2")
	load("1")
	for _, n := range foes {
		if !n.active || n.defeated {
			t.Errorf("after loading the earlier game, %s is gone", n.Name)
		}
	}
	load("2")
	for i, n := range foes {
		if gone := i < 2; n.active == gone || n.defeated != gone {
			t.Errorf("after loading the later game, %s: active %v, defeated %v", n.Name, n.active, n.defeated)
		}
	}
}
