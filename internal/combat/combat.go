// Package combat holds the rules of a fight: turns in initiative order,
// movement points and action points, abilities as data, line of sight, and
// a simple AI for the enemies. It knows nothing about drawing or input,
// and a fight with the same seed and the same actions always goes the
// same way, so fights can be tested and replayed.
//
// A fight happens on one map. Distances are counted in steps (4
// directions), like walking.
package combat

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/world"
)

// Team is a side of a fight.
type Team uint8

const (
	Party Team = iota
	Enemy
)

// Board is the ground a fight stands on.
type Board interface {
	// Offset returns the tile (dx, dy) from p; false off the map.
	Offset(p world.Pos, dx, dy int64) (world.Pos, bool)
	// Delta returns the offset from a to b (the shortest way round).
	Delta(a, b world.Pos) (dx, dy int64)
	// Walkable reports whether a combatant can stand on p (leaving the
	// combatants themselves aside).
	Walkable(p world.Pos) bool
	// Opaque reports whether p blocks the line of sight.
	Opaque(p world.Pos) bool
}

// Combatant is someone in a fight.
type Combatant struct {
	ID        int // index in the fight
	Name      string
	Kind      string // the row of Rules.Combatants it was made from
	Team      Team
	Pos       world.Pos
	HP, MaxHP int
	Move, AP  int // movement and action points per turn
	Init      int // initiative: higher goes first
	Abilities []AbilityID
	Power     int // added to the damage of every hit
	Guard     int // taken off the damage of every hit on it (which still does 1)

	MP, APLeft int  // left this turn
	Down       bool // out of the fight
}

// Outcome is how a fight stands.
type Outcome uint8

const (
	Ongoing Outcome = iota
	Victory         // every enemy is down
	Defeat          // the whole party is down
)

// Result is what an action did, for whoever shows it.
type Result struct {
	Actor, Target int // combatant IDs; Target is -1 when nobody was hit
	Ability       AbilityID
	Damage        int
	Pushed        bool      // the target was moved
	From, To      world.Pos // of the push
	Downed        bool      // the target went down
	Item          string    // the name of the item used, for a treatment
	Healed        int       // hit points the target got back
	Revived       bool      // the target got up again
}

// Fight is a fight in progress.
type Fight struct {
	rules *Rules
	board Board
	rng   *rand.Rand
	cs    []*Combatant
	order []int // combatant IDs by initiative
	turn  int   // index into order
	Round int   // starts at 1
}

// Errors of actions that can't be done.
var (
	ErrNotReachable = errors.New("can't get there this turn")
	ErrNoAP         = errors.New("not enough action points")
	ErrRange        = errors.New("out of range")
	ErrSight        = errors.New("no line of sight")
	ErrNoTarget     = errors.New("nobody to hit there")
	ErrUnknown      = errors.New("doesn't know that ability")
	ErrOver         = errors.New("the fight is over")
	ErrFit          = errors.New("already in full health")
	ErrStanding     = errors.New("still on their feet")
	ErrNotDown      = errors.New("nobody down there")
)

// New starts a fight. Combatants get their IDs (their index), full
// movement and action points come with their turn. seed drives the dice.
func New(rules *Rules, board Board, seed uint64, cs []Combatant) *Fight {
	f := &Fight{rules: rules, board: board, rng: rand.New(rand.NewPCG(seed, 0x636f6d626174)), Round: 1} // "combat"
	for i := range cs {
		c := cs[i]
		c.ID = i
		f.cs = append(f.cs, &c)
		f.order = append(f.order, i)
	}
	// Highest initiative first; ties: the party, then as listed.
	sort.SliceStable(f.order, func(a, b int) bool {
		ca, cb := f.cs[f.order[a]], f.cs[f.order[b]]
		if ca.Init != cb.Init {
			return ca.Init > cb.Init
		}
		return ca.Team < cb.Team
	})
	f.turn = -1
	f.next()
	return f
}

// Combatants returns everyone in the fight, by ID.
func (f *Fight) Combatants() []*Combatant { return f.cs }

// Active returns whose turn it is.
func (f *Fight) Active() *Combatant { return f.cs[f.order[f.turn]] }

// Order returns the combatant IDs in turn order.
func (f *Fight) Order() []int { return f.order }

// At returns the standing combatant on p, if any.
func (f *Fight) At(p world.Pos) *Combatant {
	for _, c := range f.cs {
		if !c.Down && c.Pos == p {
			return c
		}
	}
	return nil
}

// Outcome reports how the fight stands.
func (f *Fight) Outcome() Outcome {
	party, enemy := false, false
	for _, c := range f.cs {
		if c.Down {
			continue
		}
		if c.Team == Party {
			party = true
		} else {
			enemy = true
		}
	}
	switch {
	case !enemy:
		return Victory
	case !party:
		return Defeat
	}
	return Ongoing
}

// next gives the turn to the next combatant standing.
func (f *Fight) next() {
	for range f.order {
		f.turn++
		if f.turn >= len(f.order) {
			f.turn = 0
			f.Round++
		}
		if c := f.Active(); !c.Down {
			c.MP, c.APLeft = c.Move, c.AP
			return
		}
	}
}

// EndTurn passes the turn on.
func (f *Fight) EndTurn() {
	if f.Outcome() == Ongoing {
		f.next()
	}
}

// Spent reports whether the active combatant can do nothing more this
// turn: no movement left and no ability it can pay for.
func (f *Fight) Spent() bool {
	c := f.Active()
	if c.MP > 0 {
		return false
	}
	for _, id := range c.Abilities {
		if f.rules.Abilities[id].AP <= c.APLeft {
			return false
		}
	}
	return true
}

var steps = [4][2]int64{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// Reach is a tile the active combatant can move to, with the movement it
// costs.
type Reach struct {
	Pos  world.Pos
	Cost int
}

// Reachable returns the tiles the active combatant can move to this turn,
// nearest first (its own tile first, at no cost). Nobody walks through
// anybody.
func (f *Fight) Reachable() []Reach {
	out, _ := f.flood(f.Active())
	return out
}

func (f *Fight) flood(c *Combatant) ([]Reach, map[world.Pos]world.Pos) {
	out := []Reach{{c.Pos, 0}}
	prev := map[world.Pos]world.Pos{c.Pos: c.Pos}
	for i := 0; i < len(out); i++ {
		r := out[i]
		if r.Cost >= c.MP {
			continue
		}
		for _, d := range steps {
			q, ok := f.board.Offset(r.Pos, d[0], d[1])
			if !ok {
				continue
			}
			if _, seen := prev[q]; seen || !f.board.Walkable(q) || f.At(q) != nil {
				continue
			}
			prev[q] = r.Pos
			out = append(out, Reach{q, r.Cost + 1})
		}
	}
	return out, prev
}

// Move walks the active combatant to a tile it can reach and returns the
// tiles it passes, in order (without the one it starts on).
func (f *Fight) Move(to world.Pos) ([]world.Pos, error) {
	if f.Outcome() != Ongoing {
		return nil, ErrOver
	}
	c := f.Active()
	reach, prev := f.flood(c)
	cost := -1
	for _, r := range reach {
		if r.Pos == to {
			cost = r.Cost
		}
	}
	if cost < 0 {
		return nil, ErrNotReachable
	}
	var path []world.Pos
	for p := to; p != c.Pos; p = prev[p] {
		path = append(path, p)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	c.Pos, c.MP = to, c.MP-cost
	return path, nil
}

// Distance is the number of steps between two tiles, ignoring obstacles.
func (f *Fight) Distance(a, b world.Pos) int {
	dx, dy := f.board.Delta(a, b)
	return int(geom.Abs(dx) + geom.Abs(dy))
}

// Sight reports whether there is a line of sight from a to b: no opaque
// tile strictly between them.
func (f *Fight) Sight(a, b world.Pos) bool { return Sight(f.board, a, b) }

// Sight reports whether nothing opaque lies strictly between a and b on
// board (a straight line on the grid).
func Sight(board Board, a, b world.Pos) bool {
	dx, dy := board.Delta(a, b)
	n := max(geom.Abs(dx), geom.Abs(dy))
	for k := int64(1); k < n; k++ {
		// The tile nearest the line at step k of n (rounded half away
		// from zero, the same from either end).
		p, ok := board.Offset(a, roundDiv(dx*k, n), roundDiv(dy*k, n))
		if !ok || board.Opaque(p) {
			return false
		}
	}
	return true
}

func roundDiv(a, b int64) int64 {
	if a < 0 {
		return -((-a + b/2) / b)
	}
	return (a + b/2) / b
}

// CanUse reports why the active combatant can't use an ability on the
// tile target from where it stands (nil if it can).
func (f *Fight) CanUse(id AbilityID, target world.Pos) error {
	return f.canUseFrom(f.Active(), f.Active().Pos, id, target)
}

func (f *Fight) canUseFrom(c *Combatant, from world.Pos, id AbilityID, target world.Pos) error {
	if f.Outcome() != Ongoing {
		return ErrOver
	}
	ab, ok := f.rules.Abilities[id]
	if !ok || !c.knows(id) {
		return ErrUnknown
	}
	if ab.AP > c.APLeft {
		return ErrNoAP
	}
	t := f.At(target)
	if t == nil || t == c {
		return ErrNoTarget
	}
	if d := f.Distance(from, target); d < ab.Range[0] || d > ab.Range[1] {
		return ErrRange
	}
	if ab.Sight && !f.Sight(from, target) {
		return ErrSight
	}
	return nil
}

func (c *Combatant) knows(id AbilityID) bool {
	for _, a := range c.Abilities {
		if a == id {
			return true
		}
	}
	return false
}

// Targets returns the tiles the active combatant can use an ability on
// from where it stands: standing opponents in range (and in sight).
func (f *Fight) Targets(id AbilityID) []world.Pos {
	c := f.Active()
	var out []world.Pos
	for _, t := range f.cs {
		if !t.Down && t.Team != c.Team && f.CanUse(id, t.Pos) == nil {
			out = append(out, t.Pos)
		}
	}
	return out
}

// Use has the active combatant use an ability on the combatant at target.
func (f *Fight) Use(id AbilityID, target world.Pos) (Result, error) {
	if err := f.CanUse(id, target); err != nil {
		return Result{}, err
	}
	c, t, ab := f.Active(), f.At(target), f.rules.Abilities[id]
	c.APLeft -= ab.AP
	res := Result{Actor: c.ID, Target: t.ID, Ability: id}
	dmg := ab.Damage[0]
	if ab.Damage[1] > ab.Damage[0] {
		dmg += f.rng.IntN(ab.Damage[1] - ab.Damage[0] + 1)
	}
	if dmg > 0 {
		dmg += c.Power
	}
	if ab.Push > 0 {
		// A shove moves the target straight away from the shover, as far
		// as there is room; it only hurts if there is none at all.
		dx, dy := f.board.Delta(c.Pos, t.Pos)
		sx, sy := sign(dx), sign(dy)
		if geom.Abs(dx) >= geom.Abs(dy) {
			sy = 0
		} else {
			sx = 0
		}
		res.From = t.Pos
		for k := 0; k < ab.Push; k++ {
			q, ok := f.board.Offset(t.Pos, sx, sy)
			if !ok || !f.board.Walkable(q) || f.At(q) != nil {
				break
			}
			t.Pos, res.Pushed = q, true
		}
		res.To = t.Pos
		if res.Pushed {
			dmg = 0
		}
	}
	if dmg > 0 {
		dmg = max(dmg-t.Guard, 1)
	}
	res.Damage = min(dmg, t.HP)
	t.HP -= res.Damage
	if t.HP <= 0 {
		t.Down, res.Downed = true, true
	}
	return res, nil
}

// Treatment is what an item does to the friend it is used on: hit points
// back, or getting someone who is down back on their feet (with Heal hit
// points, at least 1). Using an item costs ItemAP and reaches ItemRange
// steps: the user or someone next to them.
type Treatment struct {
	Name   string // the item's name, for the log
	Heal   int
	Revive bool
}

const (
	ItemAP    = 1
	ItemRange = 1
)

// downAt returns a fallen combatant of the team on p, if any (and nobody
// stands on it).
func (f *Fight) downAt(team Team, p world.Pos) *Combatant {
	if f.At(p) != nil {
		return nil
	}
	for _, c := range f.cs {
		if c.Down && c.Team == team && c.Pos == p {
			return c
		}
	}
	return nil
}

// CanTreat reports why the active combatant can't use an item on the tile
// target (nil if it can).
func (f *Fight) CanTreat(t Treatment, target world.Pos) error {
	_, err := f.patient(t, target)
	return err
}

func (f *Fight) patient(t Treatment, target world.Pos) (*Combatant, error) {
	if f.Outcome() != Ongoing {
		return nil, ErrOver
	}
	c := f.Active()
	if c.APLeft < ItemAP {
		return nil, ErrNoAP
	}
	if f.Distance(c.Pos, target) > ItemRange {
		return nil, ErrRange
	}
	if t.Revive {
		p := f.downAt(c.Team, target)
		if p == nil {
			if s := f.At(target); s != nil && s.Team == c.Team {
				return nil, ErrStanding
			}
			return nil, ErrNotDown
		}
		return p, nil
	}
	p := f.At(target)
	switch {
	case p == nil || p.Team != c.Team:
		return nil, ErrNoTarget
	case p.HP >= p.MaxHP:
		return nil, ErrFit
	}
	return p, nil
}

// Patients returns the tiles the active combatant can use an item on.
func (f *Fight) Patients(t Treatment) []world.Pos {
	var out []world.Pos
	for _, c := range f.cs {
		if c.Team == f.Active().Team && f.CanTreat(t, c.Pos) == nil {
			out = append(out, c.Pos)
		}
	}
	return out
}

// Treat has the active combatant use an item on the friend at target.
func (f *Fight) Treat(t Treatment, target world.Pos) (Result, error) {
	p, err := f.patient(t, target)
	if err != nil {
		return Result{}, err
	}
	c := f.Active()
	c.APLeft -= ItemAP
	res := Result{Actor: c.ID, Target: p.ID, Item: t.Name}
	if p.Down {
		p.Down, p.HP, res.Revived = false, 0, true
	}
	before := p.HP
	p.HP = min(p.MaxHP, max(p.HP+t.Heal, 1))
	res.Healed = p.HP - before
	return res, nil
}

func sign(v int64) int64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// String describes a result in words.
func (f *Fight) String(r Result) string {
	a, t, ab := f.cs[r.Actor], f.cs[r.Target], f.rules.Abilities[r.Ability]
	if r.Item != "" {
		s := fmt.Sprintf("%s: %s for %s", a.Name, r.Item, t.Name)
		if a == t {
			s = fmt.Sprintf("%s: %s", a.Name, r.Item)
		}
		if r.Revived {
			s += ", back on their feet"
		}
		if r.Healed > 0 {
			s += fmt.Sprintf(", %d HP back", r.Healed)
		}
		return s + "."
	}
	s := fmt.Sprintf("%s: %s on %s", a.Name, ab.Name, t.Name)
	switch {
	case r.Pushed:
		s += ", pushed back"
	case r.Damage > 0:
		s += fmt.Sprintf(", %d damage", r.Damage)
	}
	if r.Downed {
		s += ". " + t.Name + " is down"
	}
	return s + "."
}
