package nav

import "github.com/noxer/worlds/internal/world"

// Mode is how a mover travels. The terrain of a tile and the mode decide
// whether it can be entered and how long the step takes.
type Mode uint8

const (
	// Foot walks on land and swims through shallow water.
	Foot Mode = iota
	// Ride is a bicycle or scooter: fast, land only, main map only.
	Ride
)

func (m Mode) String() string { return [...]string{"foot", "ride"}[m] }

// Step durations in ticks (60 per second).
const (
	WalkTicks = 16
	SwimTicks = 24
	RideTicks = 8
)

// minTicks is the cheapest step of a mode, used by the A* heuristic.
func (m Mode) minTicks() int {
	if m == Ride {
		return RideTicks
	}
	return WalkTicks
}

// StepTicks returns how many ticks a step onto p takes in mode m, or false if
// the tile can't be entered at all (walls, deep water, water when riding,
// riding indoors). Dynamic blockers such as NPCs are not considered.
func StepTicks(w *world.World, p world.Pos, m Mode) (int, bool) {
	if w.Solid(p) {
		return 0, false
	}
	t := w.Terrain(p)
	switch m {
	case Ride:
		if t != world.Land || !p.Map.IsMain() {
			return 0, false
		}
		return RideTicks, true
	default:
		switch t {
		case world.Land:
			return WalkTicks, true
		case world.Shallow:
			return SwimTicks, true
		}
		return 0, false
	}
}

// CanRide reports whether a mover standing on p may switch to riding.
func CanRide(w *world.World, p world.Pos) bool {
	_, ok := StepTicks(w, p, Ride)
	return ok
}
