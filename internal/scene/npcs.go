package scene

import (
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// NPCs with a daily schedule live two lives:
//   - In the party's reach (same map, within npcReach tiles) they are
//     active: they walk their routine tile by tile, block the way, talk.
//   - Out of reach they are not simulated at all. When they come back into
//     reach, they appear where the schedule says they would be by now
//     (sim.Where). Reach is far wider than the screen, so this happens out
//     of sight.

const (
	npcReach      = 64 // tiles
	npcLeaveSlack = 8  // extra distance before an active NPC goes out of reach
	npcReachEvery = 30 // ticks between reach checks for inactive NPCs
	npcWaitTicks  = 30 // ticks an NPC waits for a blocked tile before replanning
)

// scheduled reports whether the NPC follows a routine.
func (n *NPC) scheduled() bool { return len(n.schedule) > 0 }

// inReach reports whether p is within reach of the party leader. leaving
// adds some slack for NPCs that are already active, so they don't flicker
// in and out at the border.
func (e *Explore) inReach(p world.Pos, leaving bool) bool {
	l := e.party.Leader().Pos
	if p.Map != l.Map {
		return false
	}
	if !p.Map.IsMain() {
		return true // the same interior
	}
	dx, dy, _ := e.w.Delta(l, p)
	r := int64(npcReach)
	if leaving {
		r += npcLeaveSlack
	}
	return max(geom.Abs(dx), geom.Abs(dy)) <= r
}

// updateNPCs runs every tick the world runs.
func (e *Explore) updateNPCs() {
	e.clock.Tick()
	checkReach := e.clock.Ticks()%npcReachEvery == 0
	for _, n := range e.npcs {
		if !n.scheduled() {
			continue
		}
		if !n.active {
			if checkReach {
				e.maybeActivate(n)
			}
			continue
		}
		if !n.Moving() && !e.inReach(n.Pos, true) {
			n.active, n.path = false, nil
			continue
		}
		e.walkRoutine(n)
	}
}

// refreshReach updates which NPCs are in play right away, after the party
// moved to another map or far away at once.
func (e *Explore) refreshReach() {
	for _, n := range e.npcs {
		if !n.scheduled() {
			continue
		}
		if n.active && !e.inReach(n.Pos, true) {
			n.active, n.path = false, nil
		}
		if !n.active {
			e.maybeActivate(n)
		}
	}
}

// maybeActivate brings an NPC into play if its schedule puts it in reach.
func (e *Explore) maybeActivate(n *NPC) {
	p := sim.Where(e.planner, e.w, n.schedule, e.clock.Minutes())
	if !e.inReach(p, false) {
		return
	}
	if e.party.Reserves(p) || e.npcAt(p) != nil {
		// Someone stands there: step aside.
		for _, d := range world.Dirs {
			q, ok := e.w.Neighbor(p, d)
			if ok && !e.w.Solid(q) && !e.party.Reserves(q) && e.npcAt(q) == nil {
				p = q
				break
			}
		}
	}
	n.Place(p, world.Down)
	n.active, n.path, n.wait = true, nil, 0
}

// npcBlocked reports tiles an NPC must keep off: the party's, and other
// active NPCs'.
func (e *Explore) npcBlocked(self *NPC) nav.Blocked {
	return func(p world.Pos) bool {
		if e.party.Reserves(p) {
			return true
		}
		other := e.npcAt(p)
		return other != nil && other != self
	}
}

// walkRoutine moves an active NPC one tile at a time towards the place its
// schedule asks for, through doors if need be.
func (e *Explore) walkRoutine(n *NPC) {
	if n.Update() {
		if wp := e.w.WarpAt(n.Pos); wp != nil {
			facing := n.Facing
			if wp.HasFacing {
				facing = wp.Facing
			}
			n.Place(wp.To, facing)
			n.path = nil
		}
	}
	if n.Moving() {
		return
	}
	cur, _, _ := n.schedule.Current(e.clock.Minutes())
	if n.Pos == cur.Place {
		n.path = nil
		return
	}
	blocked := e.npcBlocked(n)
	if len(n.path) == 0 {
		if n.wait > 0 { // the last plan failed; don't retry every tick
			n.wait--
			return
		}
		moves, ok := e.planner.Plan(n.Pos, cur.Place, nav.Foot, blocked)
		if !ok || len(moves) == 0 {
			n.wait = npcWaitTicks
			return
		}
		n.path = moves
	}
	mv := n.path[0]
	if next, ok := e.w.Neighbor(n.Pos, mv.Dir); !ok || next != mv.To {
		n.path = nil
		return
	}
	if blocked(mv.To) {
		n.Facing = mv.Dir
		if n.wait++; n.wait >= npcWaitTicks {
			n.wait, n.path = 0, nil
		}
		return
	}
	n.wait = 0
	n.path = n.path[1:]
	ticks, ok := nav.StepTicks(e.w, mv.To, nav.Foot)
	if !ok {
		n.path = nil
		return
	}
	n.BeginStep(mv.Dir, mv.To, ticks)
}
