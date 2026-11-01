package entity

import (
	"errors"

	"github.com/noxer/worlds/internal/world"
)

// MaxFollowers is the number of companions that can travel with the leader.
const MaxFollowers = 5

// FollowDelay is how many ticks each follower starts its step after the
// member in front of it when walking, so the queue moves in a slight ripple
// instead of in lockstep. Faster or slower steps scale it proportionally. It
// must stay below StepTicks/MaxFollowers so the last follower still starts
// its step before the leader takes the next one.
const FollowDelay = 3

// followDelay scales FollowDelay to a step of the given duration.
func followDelay(ticks int) int { return max(1, FollowDelay*ticks/StepTicks) }

// ErrPartyFull is returned when adding more than MaxFollowers companions.
var ErrPartyFull = errors.New("party is full")

// Party is the leader plus up to MaxFollowers companions queued up behind
// them. Each follower walks to the tile the member in front of it just left,
// so the party moves as a snake: across map edges, around corners, and
// through warps (after a warp everyone stands on the arrival tile and the
// queue unfolds again as the leader walks on).
type Party struct {
	Members []*Actor // Members[0] is the leader
}

// NewParty creates a party led by leader.
func NewParty(leader *Actor) *Party { return &Party{Members: []*Actor{leader}} }

// Leader returns the party leader.
func (p *Party) Leader() *Actor { return p.Members[0] }

// Add appends a follower at the end of the queue, standing on the last
// member's tile.
func (p *Party) Add(a *Actor) error {
	if len(p.Members) > MaxFollowers {
		return ErrPartyFull
	}
	last := p.Members[len(p.Members)-1]
	a.Place(last.Pos, last.Facing)
	p.Members = append(p.Members, a)
	return nil
}

// Step starts a leader step in direction d onto tile `to`. Each follower
// queues a step into the tile the member in front of it is leaving, starting
// a little after that member. ticks gives the duration of a step onto a tile
// (it depends on terrain and riding); nil means StepTicks.
func (p *Party) Step(w *world.World, d world.Dir, to world.Pos, ticks func(world.Pos) int) {
	if ticks == nil {
		ticks = func(world.Pos) int { return StepTicks }
	}
	prev := make([]world.Pos, len(p.Members))
	for i, m := range p.Members {
		prev[i] = m.Dest()
	}
	lt := ticks(to)
	p.Leader().BeginStep(d, to, lt)
	for i := 1; i < len(p.Members); i++ {
		m, target := p.Members[i], prev[i-1]
		if prev[i] == target {
			continue // still stacked after a warp
		}
		if fd, ok := w.DirBetween(prev[i], target); ok {
			m.QueueStep(fd, target, ticks(target), i*followDelay(lt))
		} else {
			// Should not happen, but never let a follower get lost.
			m.Place(target, m.Facing)
		}
	}
}

// PlaceAll puts the whole party on one tile, e.g. after a warp.
func (p *Party) PlaceAll(pos world.Pos, facing world.Dir) {
	for _, m := range p.Members {
		m.Place(pos, facing)
	}
}

// Update advances all members. It returns true when the leader arrives on a
// tile.
func (p *Party) Update() (leaderArrived bool) {
	for i, m := range p.Members {
		if m.Update() && i == 0 {
			leaderArrived = true
		}
	}
	return leaderArrived
}

// Busy reports whether any member is still moving or has queued steps.
func (p *Party) Busy() bool {
	for _, m := range p.Members {
		if m.Busy() {
			return true
		}
	}
	return false
}

// Occupies reports whether any member stands on pos.
func (p *Party) Occupies(pos world.Pos) bool {
	for _, m := range p.Members {
		if m.Pos == pos {
			return true
		}
	}
	return false
}
