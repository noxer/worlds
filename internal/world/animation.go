package world

// Animation is a tile animation defined in Tiled's tileset editor: a list of
// frames (tile indices of the same tileset) with durations. Maps place the
// first tile; only drawing shows the frames. Collision, terrain and other
// tile properties always come from the placed tile.
type Animation struct {
	Frames    []int
	Durations []int // milliseconds
	total     int64
	// Phased animations start at a different point on every cell (tile
	// property "phase": "random"), so a field of flowers or torches does not
	// flicker in unison. Unphased animations run in sync, like water.
	Phased bool
}

// at returns the frame index shown at time ms on cell (x, y).
func (a *Animation) at(ms, x, y int64) int {
	t := ms
	if a.Phased {
		t += int64(mix(uint64(x), uint64(y)) % uint64(a.total))
	}
	t %= a.total
	if t < 0 {
		t += a.total
	}
	for i, d := range a.Durations {
		if t < int64(d) {
			return a.Frames[i]
		}
		t -= int64(d)
	}
	return a.Frames[len(a.Frames)-1]
}

// mix hashes two values (SplitMix64 finaliser); used for animation phases.
func mix(a, b uint64) uint64 {
	h := a*0x9e3779b97f4a7c15 ^ b
	h = (h ^ h>>30) * 0xbf58476d1ce4e5b9
	h = (h ^ h>>27) * 0x94d049bb133111eb
	return h ^ h>>31
}

// Animation returns the animation of tile i, or nil.
func (t *Tileset) Animation(i int) *Animation { return t.anims[i] }

// Frame returns the tile to draw for t at time ms (milliseconds of game
// clock) on main-map or detail-map cell (x, y). Tiles without animation are
// returned unchanged.
func (ts *Tilesets) Frame(t Tile, ms, x, y int64) Tile {
	if t.Empty() {
		return t
	}
	set := ts.list[t.Set()]
	a := set.anims[t.Index()]
	if a == nil {
		return t
	}
	return makeTile(t.Set(), a.at(ms, x, y), t.FlipH(), t.FlipV())
}
