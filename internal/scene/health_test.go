package scene

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// meadowBattle walks the party up to the meadow's first slime and returns
// the fight that starts.
func (h *harness) meadowBattle() *Battle {
	h.t.Helper()
	foes := h.enemies()
	at, ok := h.w.Offset(foes[0].Pos, -4, 0)
	if !ok {
		h.t.Fatal("no room west of the slime")
	}
	h.e.party.PlaceAll(at, world.Right)
	for k := 0; k < 30 && h.top() == Scene(h.e); k++ {
		h.tick(input.Frame{})
	}
	b, ok := h.top().(*Battle)
	if !ok {
		h.t.Fatalf("no fight (top: %T)", h.top())
	}
	return b
}

func (h *harness) combatant(b *Battle, name string) *combat.Combatant {
	for _, c := range b.f.Combatants() {
		if c.Name == name {
			return c
		}
	}
	h.t.Fatalf("%s is not in the fight", name)
	return nil
}

// TestWoundsCarryOver: a fight starts with the hit points the party has;
// after a win they stay as they were, and the fallen get up with 1.
func TestWoundsCarryOver(t *testing.T) {
	h := newBattleHarness(t)
	h.e.setHP("Ari", 5)
	b := h.meadowBattle()
	if c := h.combatant(b, "Ari"); c.HP != 5 || c.MaxHP != 14 {
		t.Fatalf("Ari starts the fight with %d/%d hit points, want 5/14", c.HP, c.MaxHP)
	}
	bryn := h.combatant(b, "Bryn")
	bryn.HP, bryn.Down = 0, true
	if o := h.playBattle(b); o != combat.Victory {
		t.Fatalf("the meadow fight: %v", o)
	}
	if cur, _ := h.e.hp("Bryn"); cur != 1 {
		t.Errorf("Bryn went down and has %d hit points after the win, want 1", cur)
	}
	for _, m := range h.e.party.Members {
		if cur, mx := h.e.hp(m.Name); cur < 1 || cur > mx {
			t.Errorf("%s has %d/%d hit points", m.Name, cur, mx)
		}
	}
	if !h.e.hurt() {
		t.Fatal("nobody is hurt after the fight")
	}

	// Saved and loaded, the wounds are still there.
	st, err := h.e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := h.e.hp("Bryn")
	h.e.healAll()
	if err := h.e.Restore(st); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.e.hp("Bryn"); got != want {
		t.Errorf("Bryn has %d hit points after loading, want %d", got, want)
	}
}

// TestDefeatWakesAtInn: a lost fight ends at the inn the next morning,
// healed, a quarter of the coins gone, the enemies back.
func TestDefeatWakesAtInn(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.Give("coin", 10)
	h.setTime(15, 0)
	b := h.meadowBattle()
	for _, c := range b.f.Combatants() {
		if c.Team == combat.Party {
			c.HP, c.Down = 0, true
		}
	}
	if o := h.playBattle(b); o != combat.Defeat {
		t.Fatalf("outcome %v", o)
	}
	if n := h.e.story.Count("coin"); n != 8 {
		t.Errorf("%d coins after the defeat, want 8", n)
	}
	bed, ok := h.w.Spawn(innMap, innBed)
	if !ok {
		t.Fatal("the inn has no bed spawn")
	}
	if p := h.leader().Pos; p != bed.Pos {
		t.Errorf("the party wakes at %v, want the inn's bed at %v", p, bed.Pos)
	}
	if hr, _ := h.e.clock.TimeOfDay(); hr != innMorning || h.e.clock.Day() != 2 {
		t.Errorf("it is %v, want the next morning", h.e.clock)
	}
	if h.e.hurt() {
		t.Errorf("wounds after the night: %v", h.e.wounds)
	}
	for _, n := range h.enemies() {
		if !n.active {
			t.Errorf("%s is gone after the defeat", n.Name)
		}
	}
}

// TestInnRest: Marta lets a bed for five coins, which heals everyone
// overnight; with fewer coins she sends the party away.
func TestInnRest(t *testing.T) {
	h := newBattleHarness(t)
	h.setTime(21, 30)
	h.e.setHP("Cato", 2)
	h.e.story.Give("coin", 3)
	h.speakWith("innkeeper", "We can't afford it")
	if !h.e.hurt() || h.e.story.Count("coin") != 3 {
		t.Fatal("rested without paying")
	}
	h.e.story.Give("coin", 4)
	h.speakWith("innkeeper", "A night's rest")
	h.runUntil("waking up", 300, func() bool { return h.e.fade == 0 && h.e.warp == nil })
	if h.e.story.Count("coin") != 2 {
		t.Errorf("%d coins left, want 2", h.e.story.Count("coin"))
	}
	if h.e.hurt() {
		t.Errorf("still hurt after a night at the inn: %v", h.e.wounds)
	}
	if hr, mi := h.e.clock.TimeOfDay(); hr != innMorning || mi != 0 || h.e.clock.Day() != 2 {
		t.Errorf("it is %v, want the next morning at %d", h.e.clock, innMorning)
	}
	if h.leader().Pos.Map != world.Detail(innMap) {
		t.Errorf("the party wakes on %v", h.leader().Pos.Map)
	}
}

// TestCamp: the menu's Camp rests eight hours outdoors and heals to half;
// it isn't offered indoors or next to enemies.
func TestCamp(t *testing.T) {
	h := newBattleHarness(t)
	h.setTime(20, 0)
	if ok, _ := h.e.canCamp(); ok {
		t.Fatal("camping with nobody hurt")
	}
	h.e.setHP("Ari", 2)
	h.e.setHP("Dagny", 15)
	if ok, why := h.e.canCamp(); !ok {
		t.Fatalf("can't camp at the start: %s", why)
	}
	h.press(input.Menu)
	h.menuTo("Camp")
	if h.top() != Scene(h.e) {
		t.Fatalf("the menu is still open: %T", h.top())
	}
	if cur, _ := h.e.hp("Ari"); cur != 7 {
		t.Errorf("Ari has %d hit points after camping, want 7 (half of 14)", cur)
	}
	if cur, _ := h.e.hp("Dagny"); cur != 15 {
		t.Errorf("Dagny has %d hit points after camping, want 15 (unchanged)", cur)
	}
	if hr, _ := h.e.clock.TimeOfDay(); hr != 4 {
		t.Errorf("it is %v after camping, want 04:00", h.e.clock)
	}

	at, _ := h.w.Offset(h.enemies()[0].Pos, -12, 0)
	h.e.party.PlaceAll(at, world.Left)
	if ok, _ := h.e.canCamp(); ok {
		t.Error("camping next to the slimes")
	}
	bed, _ := h.w.Spawn(innMap, innBed)
	h.e.party.PlaceAll(bed.Pos, world.Down)
	if ok, _ := h.e.canCamp(); ok {
		t.Error("camping indoors")
	}
}

func TestTicksTo(t *testing.T) {
	for _, c := range []struct{ h, m, want int }{{6, 0, 60}, {7, 0, 24 * 60}, {21, 30, 9*60 + 30}, {0, 0, 7 * 60}} {
		clk := sim.NewClock(1, c.h, c.m)
		if got := ticksTo(clk, 7); got != int64(c.want)*sim.TicksPerMinute {
			t.Errorf("from %02d:%02d: %d minutes, want %d", c.h, c.m, got/sim.TicksPerMinute, c.want)
		}
	}
}

// TestItemsInAFight: the bar's Items opens the items; a honey cake heals
// the one whose turn it is, smelling salts get a fallen friend up.
func TestItemsInAFight(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.Give("honey_cake", 1)
	h.e.story.Give("smelling_salts", 1)
	b := h.meadowBattle()
	h.settle(b)
	c := b.f.Active()
	if c.Team != combat.Party {
		t.Fatalf("%s's turn", c.Name)
	}
	var friend *combat.Combatant
	for _, o := range b.f.Combatants() {
		if o.Team == combat.Party && o != c && b.f.Distance(o.Pos, c.Pos) == 1 {
			friend = o
		}
	}
	if friend == nil {
		t.Fatal("nobody next to the one whose turn it is")
	}
	c.HP = 3
	friend.HP, friend.Down = 0, true
	open := func() {
		for k := 0; k < 10 && !b.actions()[b.action].bag; k++ {
			h.press(input.Menu)
		}
		h.press(input.Confirm)
		if !b.items {
			t.Fatal("the items didn't open")
		}
	}
	open()
	if a := b.actions()[b.action]; a.item != "honey_cake" {
		t.Fatalf("first item %+v", a)
	}
	b.cursor = c.Pos
	h.press(input.Confirm)
	if c.HP != 8 || h.e.story.Count("honey_cake") != 0 || b.items {
		t.Fatalf("after the cake: %d HP, %d cakes, items open %v (log %q)", c.HP, h.e.story.Count("honey_cake"), b.items, b.log)
	}
	h.settle(b)
	open()
	b.cursor = friend.Pos
	h.press(input.Confirm)
	if friend.Down || friend.HP != 4 || h.e.story.Count("smelling_salts") != 0 {
		t.Fatalf("after the salts: down %v, %d HP (log %q)", friend.Down, friend.HP, b.log)
	}
	h.settle(b)
	for _, a := range b.actions() {
		if a.bag {
			t.Error("Items still on the bar with nothing left")
		}
	}
}

// TestItemsFromTheMenu: the Items page uses a honey cake on a member.
func TestItemsFromTheMenu(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.Give("honey_cake", 2)
	h.e.setHP("Cato", 4)
	h.press(input.Menu)
	h.menuTo("Items")
	h.menuTo("Honey cake")
	h.menuTo("Ari") // fit: nothing happens
	if h.e.story.Count("honey_cake") != 2 {
		t.Fatal("a cake used on someone in full health")
	}
	h.menuTo("Cato")
	if cur, _ := h.e.hp("Cato"); cur != 9 || h.e.story.Count("honey_cake") != 1 {
		t.Fatalf("Cato has %d HP, %d cakes left", cur, h.e.story.Count("honey_cake"))
	}
	if m := h.top().(*Menu); m.msg != "Cato: Honey cake, 5 HP back." {
		t.Errorf("message %q", m.msg)
	}
}

// TestStore: Oskar's store sells, and buys back for half.
func TestStore(t *testing.T) {
	h := newBattleHarness(t)
	h.e.story.Give("coin", 20)
	h.e.story.Give("crate", 1)
	h.speakWith("shopkeeper", "Let's trade")
	s, ok := h.top().(*Shop)
	if !ok {
		t.Fatalf("no shop (top: %T)", h.top())
	}
	pick := func(prefix string) {
		t.Helper()
		for i, r := range s.rows() {
			if strings.HasPrefix(r.text, prefix) {
				s.cursor = i
				h.press(input.Confirm)
				return
			}
		}
		t.Fatalf("no row %q in %v", prefix, s.rows())
	}
	pick("Herb tonic")
	pick("Herb tonic")
	if h.e.story.Count("herb_tonic") != 2 || h.e.story.Count("coin") != 4 {
		t.Fatalf("after buying two tonics: %d tonics, %d coins", h.e.story.Count("herb_tonic"), h.e.story.Count("coin"))
	}
	pick("Smelling salts")
	if h.e.story.Count("smelling_salts") != 0 || s.msg != "You can't afford that." {
		t.Fatalf("bought salts without the coins (%q)", s.msg)
	}
	pick("Sell")
	for _, r := range s.rows() {
		if strings.HasPrefix(r.text, "Hilde") || strings.HasPrefix(r.text, "Coin") {
			t.Errorf("%q for sale", r.text)
		}
	}
	pick("Herb tonic")
	if h.e.story.Count("herb_tonic") != 1 || h.e.story.Count("coin") != 8 {
		t.Fatalf("after selling a tonic: %d tonics, %d coins", h.e.story.Count("herb_tonic"), h.e.story.Count("coin"))
	}
	pick("Leave")
	if h.top() != Scene(h.e) {
		t.Fatalf("still in the shop: %T", h.top())
	}
}
