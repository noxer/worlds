package scene

import (
	"fmt"
	"strings"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// Health between fights. Hit points carry over from one fight to the next;
// the party doesn't heal by walking. It heals:
//
//   - at the inn: a night's rest (it costs coins, said in the innkeeper's
//     dialog, which asks for the action "rest") heals everyone fully and
//     lasts until the next morning;
//   - by camping (the menu's Camp, outdoors and with no enemy about): eight
//     hours, after which everyone has at least half their hit points;
//   - after a won fight, members who went down get up with 1 hit point.
//
// A lost fight isn't the end: the party wakes up at the inn the next
// morning, healed, but a quarter of its coins is gone, and the enemies of
// that fight are back.
//
// What is kept is the hit points each member is missing (zero, and left
// out, at full health), so that a change of the maximum, by a level gained
// or a new rule, needs nothing done to old saves.

const (
	innMorning   = 7 // o'clock, when the night at the inn is over
	campHours    = 8 // how long camping takes
	defeatCoinsQ = 4 // a defeat costs 1/defeatCoinsQ of the coins
	innMap       = "inn"
	innBed       = "bed" // the spawn on innMap where the party wakes
)

// maxHP is a member's maximum hit points (by the combat rules).
func (e *Explore) maxHP(name string) int {
	if e.combat == nil {
		return 0
	}
	return e.member(name).MaxHP
}

// hp returns a member's hit points and maximum.
func (e *Explore) hp(name string) (cur, max int) {
	max = e.maxHP(name)
	return max - min(e.wounds[name], max-1), max
}

// setHP sets a member's hit points (at least 1: nobody stays down).
func (e *Explore) setHP(name string, hp int) {
	m := e.maxHP(name)
	hp = min(max(hp, 1), m)
	if hp == m {
		delete(e.wounds, name)
		return
	}
	if e.wounds == nil {
		e.wounds = map[string]int{}
	}
	e.wounds[name] = m - hp
}

// healAll restores everyone's hit points.
func (e *Explore) healAll() { e.wounds = nil }

// hurt reports whether anyone in the party is missing hit points.
func (e *Explore) hurt() bool { return len(e.wounds) > 0 }

// ticksTo returns the ticks until the next time the clock shows hour
// o'clock (a full day if it shows it now).
func ticksTo(c sim.Clock, hour int) int64 {
	now := c.Minutes() % sim.MinutesPerDay
	wait := (int64(hour*60) - now + sim.MinutesPerDay) % sim.MinutesPerDay
	if wait == 0 {
		wait = sim.MinutesPerDay
	}
	return wait*sim.TicksPerMinute - c.Ticks()%sim.TicksPerMinute
}

// sleepAtInn passes the night at the inn: the party wakes in its bed in the
// morning, healed. note is shown when it wakes.
func (e *Explore) sleepAtInn(note string) {
	e.healAll()
	e.clock.AdvanceTicks(ticksTo(e.clock, innMorning))
	to, facing := e.home, world.Down
	if s, ok := e.w.Spawn(innMap, innBed); ok {
		to = s.Pos
		if s.HasFacing {
			facing = s.Facing
		}
	}
	e.warp = &world.Warp{To: to, Facing: facing, HasFacing: true}
	e.fade = fadeTicks
	e.pendingNotice = note
}

// defeated wakes the party at the inn after a lost fight, a quarter of its
// coins poorer.
func (e *Explore) defeated() {
	lost := e.story.Count("coin") / defeatCoinsQ
	e.story.Take("coin", lost)
	note := "You wake up at the Sleeping Bee. Someone carried you here."
	if lost > 0 {
		note += fmt.Sprintf(" Your purse is %d coins lighter.", lost)
	}
	e.sleepAtInn(note)
}

// canCamp reports whether the party can camp here, and if not, why.
func (e *Explore) canCamp() (bool, string) {
	l := e.party.Leader()
	switch {
	case !l.Pos.Map.IsMain():
		return false, "You can only camp outdoors."
	case e.ride != nil:
		return false, "Not while travelling."
	case e.foeNear(l.Pos):
		return false, "Not with enemies about."
	case !e.hurt():
		return false, "Everyone is fit; no need to rest."
	}
	return true, ""
}

// foeNear reports whether an enemy that would fight is close to p.
func (e *Explore) foeNear(p world.Pos) bool {
	for _, n := range e.npcs {
		if n.enemy == "" || !n.active || n.Pos.Map != p.Map {
			continue
		}
		if dx, dy, ok := e.w.Delta(p, n.Pos); ok && max(geom.Abs(dx), geom.Abs(dy)) <= 2*battleRadius {
			return true
		}
	}
	return false
}

// camp rests for campHours: everyone gets back to at least half their hit
// points.
func (e *Explore) camp() {
	e.clock.Advance(campHours * 60)
	for _, m := range e.party.Members {
		cur, mx := e.hp(m.Name)
		e.setHP(m.Name, max(cur, (mx+1)/2))
	}
	e.say(fmt.Sprintf("You make camp and rest for %d hours.", campHours))
}

// dialogActions carries out the actions a story dialog asked for (see
// converse): "rest" (a night at the inn), "shop <id>" (opens a shop).
func (e *Explore) dialogActions(ctx *Context, run *dialog.Runner) {
	for _, a := range run.Actions() {
		switch {
		case a == "rest":
			e.sleepAtInn("Morning. Marta's knock wakes you; the room smells of bread.")
		case strings.HasPrefix(a, "shop "):
			if sh, ok := e.shops[strings.TrimPrefix(a, "shop ")]; ok {
				ctx.Push(NewShop(e, sh))
			}
		}
	}
}

// useItem uses an item on a member of the party outside a fight; it
// returns what happened, and whether the item was used up.
func (e *Explore) useItem(id, member string) (string, bool) {
	it, ok := e.items[id]
	switch {
	case !ok || it.Use == nil || e.story.Count(id) == 0:
		return "That can't be used.", false
	case it.Use.Revive:
		return "Only in a fight.", false
	}
	cur, mx := e.hp(member)
	if cur >= mx {
		return member + " is in full health already.", false
	}
	e.setHP(member, cur+it.Use.Heal)
	e.story.Take(id, 1)
	now, _ := e.hp(member)
	return fmt.Sprintf("%s: %s, %d HP back.", member, it.Name, now-cur), true
}
