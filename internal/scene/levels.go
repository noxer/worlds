package scene

import (
	"fmt"
	"strings"

	"github.com/noxer/worlds/internal/combat"
)

// Experience and levels. Beating enemies is worth experience and coins
// (their "xp" and "coins" in combat.yaml). Every member of the party gets
// the whole of the experience of a fight won, those who went down too: the
// party grows together, and nobody is left behind for having had a bad
// fight. Levels follow from experience (combat.LevelAt) and raise the
// stats by themselves: hit points by the member's growth, power (damage on
// every hit) every second level. Levels are never stored: only the
// experience is, so a change to the curve or the growth applies to old
// saves.

// level returns a member's level.
func (e *Explore) level(name string) int { return combat.LevelAt(e.xp[name]) }

// member returns a party member as a combatant at full health, with its
// level (and, see equipment.go, what it wears) applied.
func (e *Explore) member(name string) combat.Combatant {
	c := e.combat.Combatant(name, name, combat.Party)
	return e.applyGear(name, e.combat.AtLevel(c, e.level(name)))
}

// reward gives the party what beating enemies of the given kinds is worth;
// it returns a line saying so, and queues a banner for each level gained.
func (e *Explore) reward(kinds []string) string {
	if e.combat == nil {
		return ""
	}
	xp, coins := 0, 0
	for _, k := range kinds {
		s := e.combat.Combatants[k]
		xp, coins = xp+s.XP, coins+s.Coins
	}
	if coins > 0 {
		e.story.Give("coin", coins)
	}
	for _, m := range e.party.Members {
		before := e.member(m.Name)
		was := e.level(m.Name)
		if xp > 0 {
			if e.xp == nil {
				e.xp = map[string]int{}
			}
			e.xp[m.Name] += xp
		}
		if now := e.level(m.Name); now > was {
			after := e.member(m.Name)
			var gains []string
			if d := after.MaxHP - before.MaxHP; d > 0 {
				gains = append(gains, fmt.Sprintf("max HP +%d", d))
			}
			if d := after.Power - before.Power; d > 0 {
				gains = append(gains, fmt.Sprintf("power +%d", d))
			}
			text := ""
			if len(gains) > 0 {
				text = capitalize(strings.Join(gains, ", ")) + "."
			}
			e.toasts = append(e.toasts, toast{fmt.Sprintf("%s reaches level %d", m.Name, now), text})
		}
	}
	var got []string
	if xp > 0 {
		got = append(got, fmt.Sprintf("%d XP", xp))
	}
	if coins > 0 {
		got = append(got, fmt.Sprintf("%d coins", coins))
	}
	if len(got) == 0 {
		return ""
	}
	return strings.Join(got, ", ")
}
