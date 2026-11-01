// Package entity contains the logic of things that move on the tile grid:
// actors and the party that follows the leader. Rendering lives elsewhere.
package entity

import "github.com/noxer/worlds/internal/world"

// StepTicks is the duration of one tile step in ticks (60 ticks per second).
// At 16 ticks per 16px tile, actors move exactly one pixel per tick.
const StepTicks = 16

// Sprite references a frame on a tile sheet by tileset name and index.
type Sprite struct {
	Sheet string
	Index int
}

// Actor is anything that occupies a tile and can walk: party members, NPCs,
// later enemies.
type Actor struct {
	Name   string
	Sprite Sprite
	// Pos is the tile the actor occupies. While stepping, it is already the
	// destination tile, which reserves it against other actors.
	Pos    world.Pos
	Facing world.Dir

	stepping bool
	stepDir  world.Dir
	tick     int

	queue []queuedStep // steps to start later (followers lag behind)
}

type queuedStep struct {
	dir  world.Dir
	to   world.Pos
	wait int // ticks until the step may start
}

// Moving reports whether the actor is between two tiles.
func (a *Actor) Moving() bool { return a.stepping }

// Busy reports whether the actor is moving or has queued steps.
func (a *Actor) Busy() bool { return a.stepping || len(a.queue) > 0 }

// Dest returns where the actor will stand once its queued steps are done.
func (a *Actor) Dest() world.Pos {
	if n := len(a.queue); n > 0 {
		return a.queue[n-1].to
	}
	return a.Pos
}

// QueueStep schedules a step that starts after `wait` ticks, or as soon as
// the actor has finished the steps before it.
func (a *Actor) QueueStep(d world.Dir, to world.Pos, wait int) {
	a.queue = append(a.queue, queuedStep{dir: d, to: to, wait: wait})
}

// BeginStep starts walking one tile in direction d to the tile `to`.
func (a *Actor) BeginStep(d world.Dir, to world.Pos) {
	a.Facing = d
	a.Pos = to
	a.stepDir = d
	a.stepping = true
	a.tick = 0
}

// Place teleports the actor and cancels any step.
func (a *Actor) Place(p world.Pos, facing world.Dir) {
	a.Pos = p
	a.Facing = facing
	a.stepping = false
	a.tick = 0
	a.queue = a.queue[:0]
}

// Update advances the step animation. It returns true on the tick the actor
// arrives on its destination tile.
func (a *Actor) Update() (arrived bool) {
	for i := range a.queue {
		a.queue[i].wait--
	}
	if a.stepping {
		a.tick++
		if a.tick >= StepTicks {
			a.stepping = false
			a.tick = 0
			arrived = true
		}
	}
	// Start the next queued step right away so continuous walking has no
	// gaps between steps.
	if !a.stepping && len(a.queue) > 0 && a.queue[0].wait <= 0 {
		q := a.queue[0]
		a.queue = a.queue[1:]
		a.BeginStep(q.dir, q.to)
	}
	return arrived
}

// Offset returns the pixel offset of the actor's drawn position relative to
// its tile Pos. It is non-zero while stepping.
func (a *Actor) Offset() (dx, dy int) {
	if !a.stepping {
		return 0, 0
	}
	remaining := (StepTicks - a.tick) * world.TileSize / StepTicks
	ddx, ddy := a.stepDir.Delta()
	return -int(ddx) * remaining, -int(ddy) * remaining
}

// Hop returns a small vertical bounce used as walk animation, since the
// Kenney characters have a single frame.
func (a *Actor) Hop() int {
	if a.stepping && a.tick >= StepTicks/4 && a.tick < StepTicks*3/4 {
		return -1
	}
	return 0
}
