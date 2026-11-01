package world

import "fmt"

// Terrain is the kind of ground of a cell. It decides how movers can cross
// it (see package nav). The ground layer's tile sets it, through the tile
// property "terrain" in the tileset.
type Terrain uint8

const (
	// Land is anything walkable on foot (grass, sand, paths, floors).
	Land Terrain = iota
	// Shallow water can be swum through.
	Shallow
	// Deep water can only be crossed by boat.
	Deep
)

var terrainNames = [...]string{"land", "shallow", "deep"}

func (t Terrain) String() string { return terrainNames[t] }

// IsWater reports whether the terrain is water.
func (t Terrain) IsWater() bool { return t == Shallow || t == Deep }

// ParseTerrain parses a terrain name; "" is land.
func ParseTerrain(s string) (Terrain, error) {
	if s == "" {
		return Land, nil
	}
	for i, n := range terrainNames {
		if n == s {
			return Terrain(i), nil
		}
	}
	return Land, fmt.Errorf("unknown terrain %q (have %v)", s, terrainNames)
}
