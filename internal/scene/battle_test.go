package scene

import (
	"image"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// newBattleHarness is a game with the real world, the combat rules and the
// four party members.
func newBattleHarness(t *testing.T) *harness {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	if c.Combat == nil {
		t.Fatal("no combat rules")
	}
	party := entity.NewParty(&entity.Actor{Name: "Ari"})
	for _, n := range []string{"Bryn", "Cato", "Dagny"} {
		if err := party.Add(&entity.Actor{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	e, err := NewExplore(ExploreConfig{World: c.World, Sheets: gfx.NewSheets(assets.FS), Dialogs: c.Dialogs,
		Party: party, Start: c.Start, Infra: c.Infra, Combat: c.Combat, Items: c.Items})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, w: c.World, in: &input.State{}, e: e}
	h.stack = NewStack(h.in, e)
	h.stack.SetSize(320, 180)
	h.tick(input.Frame{})
	return h
}

func (h *harness) enemies() []*NPC {
	var out []*NPC
	for _, n := range h.e.npcs {
		if n.enemy != "" {
			out = append(out, n)
		}
	}
	return out
}

// settle ticks until the battle shows nothing more (walks, pauses).
func (h *harness) settle(b *Battle) {
	h.t.Helper()
	for k := 0; k < 2000; k++ {
		if !b.busy() && (b.f.Outcome() != combat.Ongoing || b.f.Active().Team == combat.Party) {
			return
		}
		h.tick(input.Frame{})
	}
	h.t.Fatal("the battle never settled")
}

// TestMeadowFight: the enemies in the meadow see the party coming; the
// party fights them through the battle screen's own controls and wins.
func TestMeadowFight(t *testing.T) {
	h := newBattleHarness(t)
	foes := h.enemies()
	if len(foes) != 3 {
		t.Fatalf("%d enemies in the world, want the meadow's 3", len(foes))
	}
	for _, n := range foes {
		if h.w.Solid(n.Pos) || h.w.Terrain(n.Pos) != world.Land {
			t.Fatalf("%s stands on %v, which isn't walkable land", n.Name, n.Pos)
		}
		if _, ok := h.e.combat.Combatants[n.enemy]; !ok {
			t.Errorf("%s is a %q, which the rules don't list", n.Name, n.enemy)
		}
	}
	// Far away nothing happens.
	h.tick(input.Frame{})
	if h.top() != Scene(h.e) {
		t.Fatalf("a fight at the start (top: %T)", h.top())
	}
	// Four steps west of the first slime, in the open: it sees the party.
	at, ok := h.w.Offset(foes[0].Pos, -4, 0)
	if !ok || h.w.Solid(at) {
		t.Fatalf("no room west of %v", foes[0].Pos)
	}
	h.e.party.PlaceAll(at, world.Right)
	for k := 0; k < 30 && h.top() == Scene(h.e); k++ {
		h.tick(input.Frame{})
	}
	b, ok := h.top().(*Battle)
	if !ok {
		t.Fatalf("no fight (top: %T)", h.top())
	}
	if n := len(b.f.Combatants()); n != 7 {
		t.Fatalf("%d combatants, want 3 enemies and 4 party members", n)
	}
	seen := map[world.Pos]bool{}
	for _, c := range b.f.Combatants() {
		if seen[c.Pos] {
			t.Fatalf("two combatants on %v", c.Pos)
		}
		seen[c.Pos] = true
	}
	if h.e.camLock == nil {
		t.Error("the camera isn't held on the fight")
	}

	// The party's turns, decided by the same AI but done with the
	// controls: cursor, Menu to pick the action, Confirm.
	pick := func(i int) {
		for k := 0; k < 10 && b.action != i; k++ {
			h.press(input.Menu)
		}
	}
	turns := 0
	for ; turns < 400 && b.f.Outcome() == combat.Ongoing; turns++ {
		h.settle(b)
		if b.f.Outcome() != combat.Ongoing {
			break
		}
		c := b.f.Active()
		switch a := b.f.Next(); a.Kind {
		case combat.ActMove:
			pick(0)
			b.cursor = a.To
			h.press(input.Confirm)
			if c.Pos != a.To {
				t.Fatalf("%s didn't move to %v (%s)", c.Name, a.To, b.log)
			}
		case combat.ActUse:
			for i, act := range b.actions() {
				if act.ability == a.Ability {
					pick(i)
				}
			}
			ap := c.APLeft
			b.cursor = a.To
			h.press(input.Confirm)
			if c.APLeft == ap {
				t.Fatalf("%s didn't use %s on %v (%s)", c.Name, a.Ability, a.To, b.log)
			}
		default:
			pick(len(b.actions()) - 1)
			h.press(input.Confirm)
		}
	}
	h.settle(b)
	if o := b.f.Outcome(); o != combat.Victory {
		t.Fatalf("outcome %v after %d actions, want a victory", o, turns)
	}
	// The outcome is shown, then Confirm goes back to exploring.
	for k := 0; k < 100 && !b.done; k++ {
		h.tick(input.Frame{})
	}
	for k := 0; k < 100 && h.top() != Scene(h.e); k++ {
		h.press(input.Confirm)
	}
	if h.top() != Scene(h.e) {
		t.Fatalf("still in the fight (top: %T)", h.top())
	}
	for _, n := range foes {
		if n.active || !n.defeated {
			t.Errorf("%s is still there", n.Name)
		}
		if !h.e.story.Flag(defeatedFlag(n)) {
			t.Errorf("%s's defeat isn't in the story (so not in a save)", n.Name)
		}
	}
	if h.e.camLock != nil {
		t.Error("the camera is still held")
	}
	l := h.leader().Pos
	for _, m := range h.e.party.Members {
		if m.Pos != l {
			t.Errorf("%s is at %v, not with the leader at %v", m.Name, m.Pos, l)
		}
	}
	// No second fight: nobody is left.
	for k := 0; k < 60; k++ {
		h.tick(input.Frame{})
	}
	if h.top() != Scene(h.e) {
		t.Fatalf("another fight (top: %T)", h.top())
	}
}

// TestBattleTaps: a tap on a tile moves there, a tap on End passes the
// turn.
func TestBattleTaps(t *testing.T) {
	h := newBattleHarness(t)
	foes := h.enemies()
	at, _ := h.w.Offset(foes[0].Pos, -4, 0)
	h.e.party.PlaceAll(at, world.Right)
	for k := 0; k < 30 && h.top() == Scene(h.e); k++ {
		h.tick(input.Frame{})
	}
	b, ok := h.top().(*Battle)
	if !ok {
		t.Fatalf("no fight (top: %T)", h.top())
	}
	h.settle(b)
	c := b.f.Active()
	// A reachable tile other than its own.
	var to world.Pos
	for _, r := range b.f.Reachable() {
		if r.Cost > 0 && b.onScreen(r.Pos) {
			to = r.Pos
		}
	}
	x, y := h.e.screen(to.Map, to.X*tile, to.Y*tile)
	h.tapAt(image.Pt(x+8, y+8))
	h.settle(b)
	if c.Pos != to {
		t.Fatalf("%s is at %v after a tap on %v (%s)", c.Name, c.Pos, to, b.log)
	}
	_, buttons := b.barRects()
	h.tapAt(centre(buttons[len(buttons)-1]))
	h.settle(b)
	if b.f.Active() == c && b.f.Outcome() == combat.Ongoing {
		t.Errorf("it is still %s's turn after a tap on End", c.Name)
	}
}
