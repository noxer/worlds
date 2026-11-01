// Package gen contains the procedural generators of the main map.
//
// Generators must be deterministic across platforms: they use only integer
// hashing of (seed, coordinates), never global randomness, map iteration
// order or floating point that could round differently on different CPUs.
package gen

import (
	"fmt"
	"sort"

	"github.com/noxer/worlds/internal/world"
)

// Hash mixes a seed and coordinates into 64 well-distributed bits
// (SplitMix64 finaliser over each input).
func Hash(seed uint64, vals ...int64) uint64 {
	h := seed ^ 0x9e3779b97f4a7c15
	for _, v := range vals {
		h ^= uint64(v)
		h += 0x9e3779b97f4a7c15
		h = (h ^ h>>30) * 0xbf58476d1ce4e5b9
		h = (h ^ h>>27) * 0x94d049bb133111eb
		h ^= h >> 31
	}
	return h
}

// Constructor creates a generator once tilesets are loaded.
type Constructor func(*world.Tilesets) (world.Generator, error)

var registry = map[string]Constructor{
	"grassland": NewGrassland,
}

// ByName returns a registered generator.
func ByName(name string) (Constructor, error) {
	c, ok := registry[name]
	if !ok {
		names := make([]string, 0, len(registry))
		for n := range registry {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown generator %q (have %v)", name, names)
	}
	return c, nil
}
