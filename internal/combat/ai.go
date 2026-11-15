package combat

import "github.com/noxer/worlds/internal/world"

// Action is one thing the AI wants the active combatant to do next.
type Action struct {
	Kind    ActionKind
	To      world.Pos // Move: where to; Use: the target's tile
	Ability AbilityID // Use
}

// ActionKind is what an Action is.
type ActionKind uint8

const (
	ActEnd ActionKind = iota
	ActMove
	ActUse
)

// Next returns what the active combatant should do next, played by the
// computer: use the ability that does the most (a kill first) from where
// it stands; otherwise move to where it can, or at least nearer the
// nearest opponent; otherwise end the turn. Call it again after carrying
// the action out, until it says ActEnd.
func (f *Fight) Next() Action {
	if f.Outcome() != Ongoing {
		return Action{}
	}
	c := f.Active()
	// From here.
	if a, ok := f.bestUse(c, c.Pos); ok {
		return a
	}
	// From somewhere reachable: the nearest such tile.
	if c.MP > 0 {
		reach, _ := f.flood(c)
		for _, r := range reach[1:] {
			if _, ok := f.bestUse(c, r.Pos); ok {
				return Action{Kind: ActMove, To: r.Pos}
			}
		}
		// Nothing to do from anywhere: close in on the nearest opponent.
		// (Only while it can still pay for something, or it would walk
		// back and forth.)
		if f.canAfford(c) {
			best, bestD := c.Pos, f.nearest(c, c.Pos)
			for _, r := range reach[1:] {
				if d := f.nearest(c, r.Pos); d < bestD {
					best, bestD = r.Pos, d
				}
			}
			if best != c.Pos {
				return Action{Kind: ActMove, To: best}
			}
		}
	}
	return Action{}
}

func (f *Fight) canAfford(c *Combatant) bool {
	for _, id := range c.Abilities {
		if f.rules.Abilities[id].AP <= c.APLeft {
			return true
		}
	}
	return false
}

// bestUse picks the ability and target that score highest from a tile.
func (f *Fight) bestUse(c *Combatant, from world.Pos) (Action, bool) {
	best, bestScore := Action{}, 0
	for _, id := range c.Abilities {
		ab := f.rules.Abilities[id]
		for _, t := range f.cs {
			if t.Down || t.Team == c.Team || f.canUseFrom(c, from, id, t.Pos) != nil {
				continue
			}
			// Twice the average damage, so halves count; a kill is worth
			// a lot; a shove that only moves is worth a little.
			score := ab.Damage[0] + ab.Damage[1]
			if ab.Push > 0 {
				score = 1
			} else if ab.Damage[0] >= t.HP {
				score += 100
			} else if (ab.Damage[0]+ab.Damage[1]+1)/2 >= t.HP {
				score += 20
			}
			if score > bestScore {
				best, bestScore = Action{Kind: ActUse, To: t.Pos, Ability: id}, score
			}
		}
	}
	return best, bestScore > 0
}

// nearest is the distance from p to the nearest standing opponent of c.
func (f *Fight) nearest(c *Combatant, p world.Pos) int {
	best := 1 << 30
	for _, t := range f.cs {
		if !t.Down && t.Team != c.Team {
			best = min(best, f.Distance(p, t.Pos))
		}
	}
	return best
}
