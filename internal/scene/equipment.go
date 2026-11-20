package scene

import (
	"fmt"
	"strings"

	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/story"
)

// Equipment: each member wears an item in each slot (a weapon, an
// armour), or nothing. Items say what they do worn ("equip" in
// items.yaml): power, guard, hit points; some only fit some members.
// An item worn is out of the party's bag (it can't be sold or worn twice)
// and goes back into it when taken off.

// worn returns the item a member wears in a slot ("" for nothing).
func (e *Explore) worn(member, slot string) string { return e.gear[member][slot] }

// wear puts an item from the bag on a member (taking off what was in its
// slot); "" takes the slot's item off. It reports whether it could.
func (e *Explore) wear(member, slot, id string) bool {
	if id != "" {
		it, ok := e.items[id]
		if !ok || it.Equip == nil || it.Equip.Slot != slot || !it.Equip.Fits(member) || e.story.Count(id) == 0 {
			return false
		}
	}
	// Hit points missing stay missing; a change of the maximum moves the
	// current hit points with it, but never to 0.
	cur, _ := e.hp(member)
	missing := e.member(member).MaxHP - cur
	if old := e.worn(member, slot); old != "" {
		e.story.Give(old, 1)
		delete(e.gear[member], slot)
	}
	if id != "" {
		e.story.Take(id, 1)
		if e.gear == nil {
			e.gear = map[string]map[string]string{}
		}
		if e.gear[member] == nil {
			e.gear[member] = map[string]string{}
		}
		e.gear[member][slot] = id
	}
	e.setHP(member, e.member(member).MaxHP-missing)
	return true
}

// wardrobe returns the items in the bag a member could wear in a slot.
func (e *Explore) wardrobe(member, slot string) []itemEntry {
	var out []itemEntry
	for _, it := range e.itemList() {
		if q := e.items[it.id].Equip; q != nil && q.Slot == slot && q.Fits(member) {
			out = append(out, it)
		}
	}
	return out
}

// applyGear adds what a member wears to its numbers.
func (e *Explore) applyGear(name string, c combat.Combatant) combat.Combatant {
	for _, slot := range story.Slots {
		if q := e.items[e.worn(name, slot)].Equip; q != nil {
			c.Power += q.Power
			c.Guard += q.Guard
			c.MaxHP += q.HP
		}
	}
	c.HP = c.MaxHP
	return c
}

// equipText describes what an item does worn ("power +1, guard +1").
func equipText(q *story.Equip) string {
	var parts []string
	for _, p := range []struct {
		name string
		n    int
	}{{"power", q.Power}, {"guard", q.Guard}, {"max HP", q.HP}} {
		if p.n != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d", p.name, p.n))
		}
	}
	return strings.Join(parts, ", ")
}
