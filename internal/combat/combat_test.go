package combat_test

import (
	"errors"
	"testing"

	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/world"
)

// grid is a test board: rows of '.', '#' (a wall: no standing, no seeing
// through) and '~' (water: no standing, but see-through).
type grid []string

func at(x, y int64) world.Pos { return world.Pos{Map: world.Main, X: x, Y: y} }

func (g grid) cell(p world.Pos) byte {
	if p.Y < 0 || p.Y >= int64(len(g)) || p.X < 0 || p.X >= int64(len(g[0])) {
		return '#'
	}
	return g[p.Y][p.X]
}
func (g grid) Offset(p world.Pos, dx, dy int64) (world.Pos, bool) {
	q := at(p.X+dx, p.Y+dy)
	return q, q.Y >= 0 && q.Y < int64(len(g)) && q.X >= 0 && q.X < int64(len(g[0]))
}
func (g grid) Delta(a, b world.Pos) (int64, int64) { return b.X - a.X, b.Y - a.Y }
func (g grid) Walkable(p world.Pos) bool           { return g.cell(p) == '.' }
func (g grid) Opaque(p world.Pos) bool             { return g.cell(p) == '#' }

const rulesYAML = `
abilities:
  strike: {name: Strike, ap: 2, range: [1, 1], damage: [3, 5]}
  shot:   {name: Shot, ap: 2, range: [2, 6], sight: true, damage: [2, 4]}
  shove:  {name: Shove, ap: 1, range: [1, 1], push: 1, damage: [2, 2]}
combatants:
  hero:   {hp: 12, move: 4, ap: 3, init: 6, abilities: [strike, shove]}
  archer: {hp: 8, move: 4, ap: 3, init: 7, abilities: [shot, shove]}
  slime:  {hp: 8, move: 3, ap: 2, init: 2, abilities: [strike]}
  wisp:   {hp: 6, move: 4, ap: 2, init: 8, abilities: [shot]}
default:  {hp: 10, move: 4, ap: 3, init: 4, abilities: [strike]}
`

func rules(t *testing.T) *combat.Rules {
	t.Helper()
	r, err := combat.LoadRules([]byte(rulesYAML))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func place(r *combat.Rules, kind string, team combat.Team, x, y int64) combat.Combatant {
	c := r.Combatant(kind, kind, team)
	c.Pos = at(x, y)
	return c
}

func TestRulesAreChecked(t *testing.T) {
	for name, y := range map[string]string{
		"unknown ability": "abilities: {}\ncombatants: {a: {hp: 1, ap: 1, abilities: [nope]}}\ndefault: {hp: 1, ap: 1}",
		"no hp":           "abilities: {}\ndefault: {hp: 0, ap: 1}",
		"bad range":       "abilities: {x: {name: X, ap: 1, range: [2, 1]}}\ndefault: {hp: 1, ap: 1}",
		"free ability":    "abilities: {x: {name: X, ap: 0, range: [1, 1]}}\ndefault: {hp: 1, ap: 1}",
	} {
		if _, err := combat.LoadRules([]byte(y)); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	r := rules(t)
	if c := r.Combatant("stranger", "Stranger", combat.Enemy); c.HP != 10 || c.MaxHP != 10 || len(c.Abilities) != 1 {
		t.Errorf("an unlisted kind gets %+v, not the default", c)
	}
}

func TestTurnOrder(t *testing.T) {
	r := rules(t)
	g := grid{"......", "......"}
	f := combat.New(r, g, 1, []combat.Combatant{
		place(r, "slime", combat.Enemy, 5, 0), place(r, "hero", combat.Party, 0, 0),
		place(r, "wisp", combat.Enemy, 5, 1), place(r, "archer", combat.Party, 0, 1),
	})
	var got []string
	for range 5 {
		c := f.Active()
		if c.MP != c.Move || c.APLeft != c.AP {
			t.Errorf("%s starts its turn with %d MP, %d AP", c.Name, c.MP, c.APLeft)
		}
		got = append(got, c.Name)
		f.EndTurn()
	}
	want := []string{"wisp", "archer", "hero", "slime", "wisp"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("turn order %v, want %v", got, want)
		}
	}
	if f.Round != 2 {
		t.Errorf("round %d after everyone had a turn", f.Round)
	}
}

func TestMovement(t *testing.T) {
	r := rules(t)
	g := grid{
		".....",
		".##..",
		".~...",
	}
	f := combat.New(r, g, 1, []combat.Combatant{place(r, "hero", combat.Party, 0, 0), place(r, "slime", combat.Enemy, 2, 0)})
	cost := map[world.Pos]int{}
	for _, rc := range f.Reachable() {
		cost[rc.Pos] = rc.Cost
	}
	// Not through the slime, the wall or the water; 4 steps at most.
	for p, want := range map[world.Pos]int{at(0, 0): 0, at(1, 0): 1, at(0, 2): 2} {
		if c, ok := cost[p]; !ok || c != want {
			t.Errorf("%v: cost %d (%v), want %d", p, c, ok, want)
		}
	}
	for _, p := range []world.Pos{at(2, 0), at(3, 0), at(1, 1), at(1, 2), at(4, 4)} {
		if _, ok := cost[p]; ok {
			t.Errorf("%v is reachable", p)
		}
	}
	if _, err := f.Move(at(3, 0)); !errors.Is(err, combat.ErrNotReachable) {
		t.Errorf("moving past the slime: %v", err)
	}
	path, err := f.Move(at(0, 2))
	if err != nil || len(path) != 2 || path[1] != at(0, 2) || f.Active().Pos != at(0, 2) || f.Active().MP != 2 {
		t.Errorf("move: path %v, err %v, at %v with %d MP", path, err, f.Active().Pos, f.Active().MP)
	}
}

func TestSight(t *testing.T) {
	g := grid{
		"......",
		"..#...",
		"..~...",
	}
	for _, c := range []struct {
		a, b world.Pos
		want bool
	}{
		{at(0, 1), at(5, 1), false}, // the wall between
		{at(0, 2), at(5, 2), true},  // across the water
		{at(0, 0), at(5, 0), true},
		{at(1, 1), at(2, 1), true}, // nothing strictly between
		{at(0, 0), at(4, 2), false},
	} {
		if got := combat.Sight(g, c.a, c.b); got != c.want {
			t.Errorf("sight %v to %v: %v", c.a, c.b, got)
		}
		if got := combat.Sight(g, c.b, c.a); got != c.want {
			t.Errorf("sight %v to %v (the other way): %v", c.b, c.a, got)
		}
	}
}

func TestAbilities(t *testing.T) {
	r := rules(t)
	g := grid{
		".......",
		"...#...",
	}
	f := combat.New(r, g, 7, []combat.Combatant{
		place(r, "archer", combat.Party, 0, 0), place(r, "hero", combat.Party, 4, 0),
		place(r, "slime", combat.Enemy, 5, 0), place(r, "slime", combat.Enemy, 6, 1),
	})
	// The archer (first): a shot needs range 2..6 and sight.
	if got := f.Targets("shot"); len(got) != 1 || got[0] != at(5, 0) {
		t.Errorf("shot targets %v, want the slime at 5,0 (the other is 7 away)", got)
	}
	if err := f.CanUse("strike", at(5, 0)); !errors.Is(err, combat.ErrUnknown) {
		t.Errorf("the archer striking: %v", err)
	}
	res, err := f.Use("shot", at(5, 0))
	if err != nil || res.Damage < 2 || res.Damage > 4 || f.Active().APLeft != 1 {
		t.Fatalf("shot: %+v, %v, %d AP left", res, err, f.Active().APLeft)
	}
	if err := f.CanUse("shot", at(5, 0)); !errors.Is(err, combat.ErrNoAP) {
		t.Errorf("a second shot: %v", err)
	}
	f.EndTurn()
	// The hero: strike the slime next to it, then shove it away.
	if err := f.CanUse("strike", at(6, 1)); !errors.Is(err, combat.ErrRange) {
		t.Errorf("striking two steps away: %v", err)
	}
	hp := f.At(at(5, 0)).HP
	res, err = f.Use("strike", at(5, 0))
	if err != nil || res.Damage < 3 || res.Damage > 5 || f.At(at(5, 0)).HP != hp-res.Damage {
		t.Fatalf("strike: %+v, %v", res, err)
	}
	res, err = f.Use("shove", at(5, 0))
	if err != nil || !res.Pushed || res.To != at(6, 0) || res.Damage != 0 || f.At(at(6, 0)) == nil {
		t.Fatalf("shove: %+v, %v", res, err)
	}
	// No action points left, but it can still move: not spent until it has.
	if f.Spent() {
		t.Errorf("spent with %d MP left", f.Active().MP)
	}
	for _, to := range []world.Pos{at(1, 0), at(2, 0)} { // 3 steps, then 1
		if _, err := f.Move(to); err != nil {
			t.Fatal(err)
		}
	}
	if !f.Spent() {
		t.Errorf("not spent with %d MP, %d AP", f.Active().MP, f.Active().APLeft)
	}
}

func TestShoveAgainstAWallHurts(t *testing.T) {
	r := rules(t)
	g := grid{"..#"}
	f := combat.New(r, g, 1, []combat.Combatant{place(r, "hero", combat.Party, 0, 0), place(r, "slime", combat.Enemy, 1, 0)})
	res, err := f.Use("shove", at(1, 0))
	if err != nil || res.Pushed || res.Damage != 2 {
		t.Fatalf("shove against a wall: %+v, %v", res, err)
	}
}

func TestOutcomeAndDeterminism(t *testing.T) {
	r := rules(t)
	g := grid{"........", "........", "........"}
	play := func(seed uint64) (combat.Outcome, int, []int) {
		f := combat.New(r, g, seed, []combat.Combatant{
			place(r, "hero", combat.Party, 0, 1), place(r, "archer", combat.Party, 0, 0),
			place(r, "slime", combat.Enemy, 7, 1), place(r, "wisp", combat.Enemy, 7, 2),
		})
		// Everyone is played by the computer.
		for n := 0; n < 2000 && f.Outcome() == combat.Ongoing; n++ {
			switch a := f.Next(); a.Kind {
			case combat.ActMove:
				if _, err := f.Move(a.To); err != nil {
					t.Fatalf("the AI's move to %v: %v", a.To, err)
				}
			case combat.ActUse:
				if _, err := f.Use(a.Ability, a.To); err != nil {
					t.Fatalf("the AI's %s on %v: %v", a.Ability, a.To, err)
				}
			default:
				f.EndTurn()
			}
		}
		var hp []int
		for _, c := range f.Combatants() {
			hp = append(hp, c.HP)
		}
		return f.Outcome(), f.Round, hp
	}
	o1, r1, hp1 := play(42)
	o2, r2, hp2 := play(42)
	if o1 == combat.Ongoing {
		t.Fatalf("the fight never ended (round %d, hp %v)", r1, hp1)
	}
	if o1 != o2 || r1 != r2 || len(hp1) != len(hp2) {
		t.Fatalf("the same seed gave %v round %d and %v round %d", o1, r1, o2, r2)
	}
	for i := range hp1 {
		if hp1[i] != hp2[i] {
			t.Fatalf("the same seed gave hp %v and %v", hp1, hp2)
		}
	}
}

func TestAIClosesInAndStrikes(t *testing.T) {
	r := rules(t)
	g := grid{"......"}
	f := combat.New(r, g, 1, []combat.Combatant{place(r, "slime", combat.Enemy, 5, 0), place(r, "hero", combat.Party, 0, 0)})
	f.EndTurn() // the hero stands still; the slime's turn
	if f.Active().Name != "slime" {
		t.Fatalf("it is %s's turn", f.Active().Name)
	}
	a := f.Next()
	if a.Kind != combat.ActMove || a.To != at(2, 0) {
		t.Fatalf("the slime (3 moves, 5 away) does %+v, want a move to 2,0", a)
	}
	f.Move(a.To)
	if a = f.Next(); a.Kind != combat.ActEnd {
		t.Fatalf("out of reach it does %+v, want to end its turn", a)
	}
	f.EndTurn()
	f.EndTurn()
	if a = f.Next(); a.Kind != combat.ActMove || a.To != at(1, 0) {
		t.Fatalf("next turn it does %+v, want a move next to the hero", a)
	}
	f.Move(a.To)
	if a = f.Next(); a.Kind != combat.ActUse || a.Ability != "strike" || a.To != at(0, 0) {
		t.Fatalf("next to the hero it does %+v, want a strike", a)
	}
}

func TestItems(t *testing.T) {
	r := rules(t)
	g := grid{"......"}
	cs := []combat.Combatant{
		place(r, "archer", combat.Party, 0, 0), place(r, "hero", combat.Party, 1, 0),
		place(r, "hero", combat.Party, 3, 0), place(r, "slime", combat.Enemy, 5, 0),
	}
	cs[1].HP = 4
	cs[2].HP, cs[2].Down = 0, true
	f := combat.New(r, g, 3, cs)
	if f.Active().Kind != "archer" {
		t.Fatalf("%s first", f.Active().Name)
	}
	cake := combat.Treatment{Name: "Cake", Heal: 5}
	salts := combat.Treatment{Name: "Salts", Heal: 3, Revive: true}
	if got := f.Patients(cake); len(got) != 1 || got[0] != at(1, 0) {
		t.Errorf("cake patients %v, want the hurt hero next to the archer", got)
	}
	if err := f.CanTreat(cake, at(0, 0)); !errors.Is(err, combat.ErrFit) {
		t.Errorf("cake for the archer at full health: %v", err)
	}
	if err := f.CanTreat(salts, at(1, 0)); !errors.Is(err, combat.ErrStanding) {
		t.Errorf("salts for someone standing: %v", err)
	}
	if err := f.CanTreat(cake, at(5, 0)); !errors.Is(err, combat.ErrRange) {
		t.Errorf("cake for the slime: %v", err)
	}
	res, err := f.Treat(cake, at(1, 0))
	if err != nil || res.Healed != 5 || f.At(at(1, 0)).HP != 9 || f.Active().APLeft != 2 {
		t.Fatalf("cake: %+v, %v", res, err)
	}
	if s := f.String(res); s != "archer: Cake for hero, 5 HP back." {
		t.Errorf("log %q", s)
	}
	// The fallen hero is two steps away: walk up, then revive.
	if err := f.CanTreat(salts, at(3, 0)); !errors.Is(err, combat.ErrRange) {
		t.Errorf("salts from afar: %v", err)
	}
	f.EndTurn() // the healed hero: walks up and revives the other
	if _, err := f.Move(at(2, 0)); err != nil {
		t.Fatal(err)
	}
	res, err = f.Treat(salts, at(3, 0))
	if err != nil || !res.Revived || res.Healed != 3 || f.At(at(3, 0)) == nil || f.At(at(3, 0)).HP != 3 {
		t.Fatalf("salts: %+v, %v", res, err)
	}
	if s := f.String(res); s != "hero: Salts for hero, back on their feet, 3 HP back." {
		t.Errorf("log %q", s)
	}
}

func TestLevels(t *testing.T) {
	for _, c := range []struct{ xp, level int }{{0, 1}, {9, 1}, {10, 2}, {29, 2}, {30, 3}, {60, 4}, {100, 5}} {
		if got := combat.LevelAt(c.xp); got != c.level {
			t.Errorf("%d XP: level %d, want %d", c.xp, got, c.level)
		}
	}
	r, err := combat.LoadRules([]byte(rulesYAML + "\n" + `
`))
	if err != nil {
		t.Fatal(err)
	}
	r.Combatants["hero"] = func() combat.Stats { s := r.Combatants["hero"]; s.Growth.HP = 3; return s }()
	c := r.AtLevel(r.Combatant("hero", "hero", combat.Party), 4)
	if c.MaxHP != 12+9 || c.HP != c.MaxHP || c.Power != 2 {
		t.Errorf("hero at level 4: %d/%d HP, power %d", c.HP, c.MaxHP, c.Power)
	}
	// Power adds to every hit.
	g := grid{".."}
	c.Pos = at(0, 0)
	f := combat.New(r, g, 1, []combat.Combatant{c, place(r, "slime", combat.Enemy, 1, 0)})
	res, err := f.Use("strike", at(1, 0))
	if err != nil || res.Damage < 5 || res.Damage > 7 {
		t.Errorf("a strike with power 2: %+v, %v", res, err)
	}
}

func TestGuard(t *testing.T) {
	r := rules(t)
	g := grid{".."}
	hero, slime := place(r, "hero", combat.Party, 0, 0), place(r, "slime", combat.Enemy, 1, 0)
	slime.Guard = 10
	f := combat.New(r, g, 1, []combat.Combatant{hero, slime})
	res, err := f.Use("strike", at(1, 0))
	if err != nil || res.Damage != 1 {
		t.Errorf("a strike on guard 10: %+v, %v (want 1 damage)", res, err)
	}
}
