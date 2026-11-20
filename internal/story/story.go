// Package story holds the state of the story: named flags (things that
// have happened) and the party's items. Dialogs, quests and puzzles read
// and change it through conditions and effects, which are short strings in
// the content files:
//
//	conditions            effects
//	flag met_elder        set met_elder
//	!flag met_elder       clear met_elder
//	item crate            give crate
//	item coin 3           give coin 3
//	!item crate           take coin 3
//
// A list of conditions holds if every one of them does.
package story

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// State is the story so far.
type State struct {
	flags map[string]bool
	items map[string]int
}

// New returns the state at the beginning: nothing has happened, nothing is
// carried.
func New() *State { return &State{flags: map[string]bool{}, items: map[string]int{}} }

// Flag reports whether a flag is set.
func (s *State) Flag(name string) bool { return s.flags[name] }

// SetFlag sets or clears a flag.
func (s *State) SetFlag(name string, on bool) {
	if on {
		s.flags[name] = true
	} else {
		delete(s.flags, name)
	}
}

// Count returns how many of an item the party carries.
func (s *State) Count(item string) int { return s.items[item] }

// Give adds n of an item.
func (s *State) Give(item string, n int) {
	if n > 0 {
		s.items[item] += n
	}
}

// Take removes n of an item; false (and nothing removed) if the party
// carries fewer.
func (s *State) Take(item string, n int) bool {
	if n < 0 || s.items[item] < n {
		return false
	}
	if s.items[item] -= n; s.items[item] == 0 {
		delete(s.items, item)
	}
	return true
}

// Flags returns the flags that are set, sorted.
func (s *State) Flags() []string {
	out := make([]string, 0, len(s.flags))
	for f := range s.flags {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Items returns the items carried and how many of each (a copy).
func (s *State) Items() map[string]int {
	out := make(map[string]int, len(s.items))
	for k, v := range s.items {
		out[k] = v
	}
	return out
}

// Restore replaces the state (a loaded game).
func (s *State) Restore(flags []string, items map[string]int) {
	clear(s.flags)
	clear(s.items)
	for _, f := range flags {
		s.flags[f] = true
	}
	for k, v := range items {
		if v > 0 {
			s.items[k] = v
		}
	}
}

// expr is a parsed condition or effect.
type expr struct {
	not  bool
	verb string // flag, item; set, clear, give, take
	name string
	n    int
}

func parse(s string, verbs ...string) (expr, error) {
	var e expr
	f := strings.Fields(s)
	if len(f) > 0 && strings.HasPrefix(f[0], "!") {
		e.not = true
		if f[0] = f[0][1:]; f[0] == "" {
			f = f[1:]
		}
	}
	if len(f) < 2 || len(f) > 3 {
		return e, fmt.Errorf("%q: want \"<what> <name> [count]\"", s)
	}
	e.verb, e.name, e.n = f[0], f[1], 1
	ok := false
	for _, v := range verbs {
		ok = ok || v == e.verb
	}
	if !ok {
		return e, fmt.Errorf("%q: unknown word %q (want one of %s)", s, e.verb, strings.Join(verbs, ", "))
	}
	counted := e.verb == "item" || e.verb == "give" || e.verb == "take"
	if len(f) == 3 {
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 1 || !counted {
			return e, fmt.Errorf("%q: bad count %q", s, f[2])
		}
		e.n = n
	}
	return e, nil
}

// CheckCondition reports what is wrong with a condition's wording (nil if
// nothing), and the item it mentions, if any.
func CheckCondition(c string) (item string, err error) {
	e, err := parse(c, "flag", "item")
	if e.verb == "item" {
		item = e.name
	}
	return item, err
}

// CheckEffect reports what is wrong with an effect's wording (nil if
// nothing), and the item it mentions, if any.
func CheckEffect(c string) (item string, err error) {
	e, err := parse(c, "set", "clear", "give", "take")
	if e.not {
		err = fmt.Errorf("%q: an effect can't start with !", c)
	}
	if e.verb == "give" || e.verb == "take" {
		item = e.name
	}
	return item, err
}

// Check reports whether a condition holds. A badly worded condition never
// holds (content is checked when it is loaded).
func (s *State) Check(c string) bool {
	e, err := parse(c, "flag", "item")
	if err != nil {
		return false
	}
	ok := s.flags[e.name]
	if e.verb == "item" {
		ok = s.items[e.name] >= e.n
	}
	return ok != e.not
}

// All reports whether every condition holds (true for none).
func (s *State) All(conds []string) bool {
	for _, c := range conds {
		if !s.Check(c) {
			return false
		}
	}
	return true
}

// Change is what an effect did to the items, for telling the player.
type Change struct {
	Item string
	N    int // > 0 received, < 0 handed over
}

// Apply carries out an effect and reports what it did to the items (the
// zero Change if nothing). Taking more than is carried takes nothing.
func (s *State) Apply(effect string) Change {
	e, err := parse(effect, "set", "clear", "give", "take")
	if err != nil || e.not {
		return Change{}
	}
	switch e.verb {
	case "set":
		s.SetFlag(e.name, true)
	case "clear":
		s.SetFlag(e.name, false)
	case "give":
		s.Give(e.name, e.n)
		return Change{e.name, e.n}
	case "take":
		if s.Take(e.name, e.n) {
			return Change{e.name, -e.n}
		}
	}
	return Change{}
}

// Item describes a kind of item.
type Item struct {
	Name string `yaml:"name"`
	Text string `yaml:"text"` // a line of description
	Use  *Use   `yaml:"use"`  // what using it does; nil if it can't be used
	// Equip makes it something a party member can wear or wield; nil if
	// it isn't.
	Equip *Equip `yaml:"equip"`
	// Price is what a shop asks for it; it buys it back for half. 0: it
	// can't be bought or sold.
	Price int `yaml:"price"`
}

// Equipment slots: a member wears one item in each.
const (
	SlotWeapon = "weapon"
	SlotArmour = "armour"
)

// Slots are the equipment slots, in order.
var Slots = []string{SlotWeapon, SlotArmour}

// Equip is what an item does worn: power (damage on every hit), guard
// (damage taken off every hit, which still does at least 1), more hit
// points. Who lists the members who can use it (anyone if empty).
type Equip struct {
	Slot  string   `yaml:"slot"`
	Power int      `yaml:"power"`
	Guard int      `yaml:"guard"`
	HP    int      `yaml:"hp"`
	Who   []string `yaml:"who"`
}

// Fits reports whether a member can use it.
func (q *Equip) Fits(member string) bool {
	if len(q.Who) == 0 {
		return true
	}
	for _, w := range q.Who {
		if w == member {
			return true
		}
	}
	return false
}

// SellPrice is what a shop pays for an item: half its price, at least 1;
// 0 if it can't be sold.
func (it Item) SellPrice() int {
	if it.Price <= 0 {
		return 0
	}
	return max(it.Price/2, 1)
}

// Shop is a shop's stock: the items it sells, at their prices, in order.
// It buys whatever has a price.
type Shop struct {
	Name  string   `yaml:"name"`
	Sells []string `yaml:"sells"` // item ids
}

// Use is what an item does to the party member it is used on (and it is
// used up): hit points back, or, in a fight, getting someone who is down
// back on their feet (with Heal hit points).
type Use struct {
	Heal   int  `yaml:"heal"`
	Revive bool `yaml:"revive"`
}
