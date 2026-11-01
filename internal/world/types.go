// Package world holds the static description of the game world: maps loaded
// from Tiled, the regions that stitch edge-connected maps together, warps,
// spawns and collision. It deliberately has no dependency on Ebitengine so
// that movement and navigation can be tested headless.
package world

import "fmt"

// TileSize is the edge length of a tile in pixels.
const TileSize = 16

// MapID identifies a map. It is the map's file name without extension.
type MapID string

// Pos is a tile position on a specific map.
type Pos struct {
	Map  MapID
	X, Y int
}

func (p Pos) String() string { return fmt.Sprintf("%s(%d,%d)", p.Map, p.X, p.Y) }

// Dir is one of the four movement directions. The game uses 4-directional
// movement everywhere, in exploration as well as in combat.
type Dir uint8

const (
	Down Dir = iota
	Left
	Right
	Up
)

// Dirs lists all directions in a stable order.
var Dirs = [4]Dir{Up, Down, Left, Right}

// Delta returns the tile offset of one step in direction d.
func (d Dir) Delta() (dx, dy int) {
	switch d {
	case Up:
		return 0, -1
	case Down:
		return 0, 1
	case Left:
		return -1, 0
	default:
		return 1, 0
	}
}

// Opposite returns the reverse direction.
func (d Dir) Opposite() Dir {
	switch d {
	case Up:
		return Down
	case Down:
		return Up
	case Left:
		return Right
	default:
		return Left
	}
}

func (d Dir) String() string {
	return [...]string{"down", "left", "right", "up"}[d]
}

// ParseDir parses "up", "down", "left" or "right".
func ParseDir(s string) (Dir, bool) {
	for _, d := range Dirs {
		if d.String() == s {
			return d, true
		}
	}
	return Down, false
}
