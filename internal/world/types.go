// Package world holds the game world: the wrapping, chunked main map
// assembled from procedural generation and hand-made patches, and the
// bounded detail maps (interiors, caves) reached through warps. It has no
// dependency on Ebitengine so that movement and navigation are testable
// headless.
package world

import "fmt"

// TileSize is the edge length of a tile in pixels.
const TileSize = 16

// ChunkSize is the edge length of a chunk in tiles.
const ChunkSize = 32

// MapID names a map: MainMap or the id of a detail-map template (its file
// name without extension).
type MapID string

// MainMap is the id of the open-world main map.
const MainMap MapID = "main"

// MapRef identifies a concrete map: the main map, or one instance of a
// detail-map template. Instance 0 is the default instance.
type MapRef struct {
	Template MapID
	Instance uint64
}

// Main is the reference to the main map.
var Main = MapRef{Template: MainMap}

// Detail returns the default instance of a detail-map template.
func Detail(id MapID) MapRef { return MapRef{Template: id} }

func (m MapRef) String() string {
	if m.Instance == 0 {
		return string(m.Template)
	}
	return fmt.Sprintf("%s#%d", m.Template, m.Instance)
}

// IsMain reports whether m is the main map.
func (m MapRef) IsMain() bool { return m.Template == MainMap }

// Pos is a tile position on a map. Positions on the main map are kept
// normalised to [0,W)x[0,H).
type Pos struct {
	Map  MapRef
	X, Y int64
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
func (d Dir) Delta() (dx, dy int64) {
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
