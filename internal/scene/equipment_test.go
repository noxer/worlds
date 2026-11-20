package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/input"
)

// TestGear: the menu's Gear puts a sword and a mail shirt on Ari; they
// raise her numbers, leave the bag, and come back to it when taken off.
func TestGear(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.Give("iron_sword", 1)
	h.e.story.Give("mail_shirt", 1)
	base := h.e.member("Ari")
	h.e.setHP("Ari", base.MaxHP-3)

	h.press(input.Menu)
	h.menuTo("Gear")
	h.menuTo("Ari")
	h.menuTo("Weapon")
	h.menuTo("Iron sword")
	h.menuTo("Armour")
	h.menuTo("Mail shirt")
	c := h.e.member("Ari")
	if c.Power != base.Power+1 || c.Guard != base.Guard+1 || c.MaxHP != base.MaxHP+4 {
		t.Fatalf("Ari with sword and mail: %+v, without: %+v", c, base)
	}
	if cur, mx := h.e.hp("Ari"); mx-cur != 3 {
		t.Errorf("Ari has %d/%d HP: the 3 missing should stay missing", cur, mx)
	}
	if h.e.story.Count("iron_sword") != 0 || h.e.story.Count("mail_shirt") != 0 {
		t.Error("worn items still in the bag")
	}
	if got := h.e.wardrobe("Bryn", "armour"); len(got) != 0 {
		t.Errorf("Bryn's wardrobe: %v", got)
	}
	h.e.story.Give("mail_shirt", 1)
	if got := h.e.wardrobe("Bryn", "armour"); len(got) != 0 {
		t.Errorf("a mail shirt fits Bryn: %v", got)
	}
	if got := h.e.wardrobe("Dagny", "armour"); len(got) != 1 {
		t.Errorf("Dagny's wardrobe: %v", got)
	}

	// Saved and loaded, still worn.
	st, err := h.e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	h.e.gear = nil
	if err := h.e.Restore(st); err != nil {
		t.Fatal(err)
	}
	if h.e.worn("Ari", "weapon") != "iron_sword" {
		t.Fatalf("after loading Ari wears %q", h.e.worn("Ari", "weapon"))
	}

	// Off again: back in the bag.
	h.menuTo("Weapon")
	h.menuTo("(nothing)")
	if h.e.worn("Ari", "weapon") != "" || h.e.story.Count("iron_sword") != 1 {
		t.Fatalf("after taking the sword off: worn %q, %d in the bag", h.e.worn("Ari", "weapon"), h.e.story.Count("iron_sword"))
	}

	// In a fight, what is worn counts.
	h.press(input.Cancel)
	h.press(input.Cancel)
	h.press(input.Cancel)
	if h.top() != Scene(h.e) {
		t.Fatalf("menu still open: %T", h.top())
	}
	h.runUntil("the load's fade", 300, func() bool { return h.e.fade == 0 && h.e.warp == nil })
	b := h.meadowBattle()
	if ari := h.combatant(b, "Ari"); ari.Guard != 1 || ari.MaxHP != base.MaxHP+4 {
		t.Errorf("Ari in the fight: %+v", ari)
	}
}

// TestForge: Hilde opens her forge once her quest is done.
func TestForge(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.SetFlag("quest.hilde_delivery.done", true)
	h.speakWith("smith", "Show us")
	s, ok := h.top().(*Shop)
	if !ok || s.shop.Name != "Hilde's Forge" {
		t.Fatalf("no forge (top: %T)", h.top())
	}
}
