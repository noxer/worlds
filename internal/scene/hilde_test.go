package scene

import (
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/world"
)

// playBattle plays a fight to its end through the battle's controls, the
// party's moves chosen by the rules' AI, and goes back to exploring.
func (h *harness) playBattle(b *Battle) combat.Outcome {
	h.t.Helper()
	pick := func(i int) {
		for k := 0; k < 10 && b.action != i; k++ {
			h.press(input.Menu)
		}
	}
	for turns := 0; turns < 400 && b.f.Outcome() == combat.Ongoing; turns++ {
		h.settle(b)
		if b.f.Outcome() != combat.Ongoing {
			break
		}
		switch a := b.f.Next(); a.Kind {
		case combat.ActMove:
			pick(0)
			b.cursor = a.To
			h.press(input.Confirm)
		case combat.ActUse:
			for i, act := range b.actions() {
				if act.ability == a.Ability {
					pick(i)
				}
			}
			b.cursor = a.To
			h.press(input.Confirm)
		default:
			pick(len(b.actions()) - 1)
			h.press(input.Confirm)
		}
	}
	h.settle(b)
	o := b.f.Outcome()
	for k := 0; k < 100 && !b.done; k++ {
		h.tick(input.Frame{})
	}
	for k := 0; k < 100 && h.top() != Scene(h.e); k++ {
		h.press(input.Confirm)
	}
	h.runUntil("back to exploring", 300, func() bool { return h.top() == Scene(h.e) && h.e.fade == 0 })
	return o
}

// TestHildesDelivery plays the quest: Hilde asks, the bandits in the
// cellar are beaten, the crate is taken from their chest, Hilde pays.
func TestHildesDelivery(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	h := newBattleHarness(t)
	h.e.quests = c.Quests
	store := save.Memory{}
	h.e.saves = store
	stage := func() string {
		for _, s := range h.e.quests.Journal(h.e.story) {
			if s.Quest.ID == "hilde_delivery" {
				if s.Done {
					return "done"
				}
				return s.Stage.ID
			}
		}
		return ""
	}

	// Hilde in her smithy.
	h.speakWith("smith", "We'll get it back")
	if stage() != "bandits" {
		t.Fatalf("after talking to Hilde: stage %q, flags %v", stage(), h.e.story.Flags())
	}
	if got := h.toastTitles(); len(got) == 0 || got[len(got)-1] != "New quest: Hilde's delivery" {
		t.Fatalf("banners %v", got)
	}
	h.speakWith("smith") // no crate yet: she waits
	if h.e.story.Count("coin") != 0 {
		t.Fatal("paid already")
	}

	// The cellar, puzzles solved: the lever pulled, the crate on its plate.
	lever, crate := h.e.puzzle.byKey["cellar/lever_1"], h.e.puzzle.byKey["cellar/crate_1"]
	h.e.pullLever(lever)
	h.e.moveBlock(crate, cell(12, 6))
	h.e.updatePuzzles()
	if !h.e.puzzle.byKey["cellar/door_2b"].on {
		t.Fatal("the door to the back room is shut")
	}
	// Into the doorway of the back room: the bandits see the party.
	h.e.party.PlaceAll(cell(8, 4), world.Left)
	for k := 0; k < 60 && h.top() == Scene(h.e); k++ {
		h.tick(input.Frame{})
	}
	b, ok := h.top().(*Battle)
	if !ok {
		t.Fatalf("no fight in the cellar (top: %T)", h.top())
	}
	if n := len(b.f.Combatants()); n != 7 {
		t.Fatalf("%d in the fight, want 3 bandits and 4 of the party", n)
	}
	// The party gathers round its leader, not strung out in a line.
	var leader world.Pos
	for _, c := range b.f.Combatants() {
		if c.Name == h.leader().Name {
			leader = c.Pos
		}
	}
	for _, c := range b.f.Combatants() {
		if c.Team == combat.Party && b.f.Distance(c.Pos, leader) > 2 {
			t.Errorf("%s starts %d steps from the leader", c.Name, b.f.Distance(c.Pos, leader))
		}
	}
	if o := h.playBattle(b); o != combat.Victory {
		t.Fatalf("the fight with the bandits: %v", o)
	}
	if stage() != "crate" {
		t.Fatalf("after the fight: stage %q", stage())
	}

	// The chest in the back room (2,2): from below, facing up.
	chest := cell(2, 2)
	if h.e.pickupAt(chest) == nil || !h.w.Solid(chest) {
		t.Fatal("no chest in the back room")
	}
	h.e.party.PlaceAll(cell(2, 3), world.Up)
	h.press(input.Confirm)
	if h.e.story.Count("crate") != 1 || h.e.pickupAt(chest) != nil || h.w.Solid(chest) {
		t.Fatalf("after opening the chest: %d crates", h.e.story.Count("crate"))
	}
	if stage() != "back" {
		t.Fatalf("with the crate: stage %q", stage())
	}
	h.press(input.Confirm) // nothing more there
	if h.e.story.Count("crate") != 1 {
		t.Fatal("the chest gave a second crate")
	}
	// Save here (crate in hand, chest empty), then hand it over.
	h.press(input.Menu)
	h.menuTo("Save")
	h.menuTo("1")

	h.speakWith("smith")
	if stage() != "done" || h.e.story.Count("crate") != 0 || h.e.story.Count("coin") != 20 {
		t.Fatalf("after handing over: stage %q, %d crates, %d coins", stage(), h.e.story.Count("crate"), h.e.story.Count("coin"))
	}
	h.speakWith("smith")
	if h.e.story.Count("coin") != 20 {
		t.Fatal("Hilde paid twice")
	}
	// The save from before: crate in hand, the chest still empty.
	h.press(input.Menu)
	h.menuTo("Load")
	h.menuTo("1")
	h.runUntil("loading", 200, func() bool { return h.e.fade == 0 })
	if h.e.story.Count("crate") != 1 || h.e.pickupAt(chest) != nil || stage() != "back" {
		t.Fatalf("after loading: %d crates, chest %v, stage %q", h.e.story.Count("crate"), h.e.pickupAt(chest), stage())
	}
	for _, n := range h.e.npcs {
		if n.enemy != "" && !n.Pos.Map.IsMain() && n.active {
			t.Errorf("%s is back after loading", n.Name)
		}
	}
}
