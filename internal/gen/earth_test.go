package gen_test

import (
	"testing"
	"time"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/gen"
	"github.com/noxer/worlds/internal/world"
)

func loadWorld(t testing.TB) *world.World {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	return c.World
}

// coastalSample returns the top-left corners of 64x64 tile windows along
// coasts in different parts of the world (found with the elevation
// preview), plus the start area.
func coastalSample(t testing.TB, w *world.World, n int) []world.Pos {
	var out []world.Pos
	out = append(out, world.Pos{Map: world.Main, X: 0, Y: 193760})
	for i := int64(0); len(out) < n && i < 4000; i++ {
		x := gen.Hash(7, i, 0) % uint64(w.W)
		y := gen.Hash(7, i, 1) % uint64(w.H)
		a, _ := w.Preview(int64(x), int64(y))
		b, _ := w.Preview(int64(x)+48, int64(y)+48)
		if (a == world.Land) != (b == world.Land) {
			out = append(out, world.Pos{Map: world.Main, X: int64(x), Y: int64(y)})
		}
	}
	return out
}

// TestCoastRules checks the generated terrain: deep water never touches land
// (not even diagonally), and generated water and land cells come in 2x2
// blocks, so the coastline art always fits.
func TestCoastRules(t *testing.T) {
	w := loadWorld(t)
	at := func(p world.Pos, dx, dy int64) world.Terrain {
		q, _ := w.Offset(p, dx, dy)
		return w.Terrain(q)
	}
	water := 0
	for _, o := range coastalSample(t, w, 12) {
		for y := int64(1); y < 63; y++ {
			for x := int64(1); x < 63; x++ {
				p, _ := w.Offset(o, x, y)
				tr := w.Terrain(p)
				if tr == world.Deep {
					for dy := int64(-1); dy <= 1; dy++ {
						for dx := int64(-1); dx <= 1; dx++ {
							if at(p, dx, dy) == world.Land {
								t.Fatalf("%v: deep water next to land", p)
							}
						}
					}
				}
				if tr.IsWater() {
					water++
				}
				isWater := tr.IsWater()
				inBlock := false
				for _, d := range [4][2]int64{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
					all := true
					for _, c := range [4][2]int64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
						if at(p, d[0]+c[0], d[1]+c[1]).IsWater() != isWater {
							all = false
						}
					}
					if all {
						inBlock = true
					}
				}
				if !inBlock {
					t.Errorf("%v: %v cell not part of a 2x2 block", p, tr)
				}
			}
		}
	}
	if water < 1000 {
		t.Fatalf("sample has too little water (%d cells)", water)
	}
}

// TestAutotilerFallbacks counts water cells without an exact tile for their
// neighbourhood. A few odd corners are acceptable; more means the cleanup
// rules and the tile set don't match.
func TestAutotilerFallbacks(t *testing.T) {
	w := loadWorld(t)
	before := gen.Fallbacks()
	cells := 0
	for _, o := range coastalSample(t, w, 40) {
		for y := int64(0); y < 64; y += 32 {
			for x := int64(0); x < 64; x += 32 {
				p, _ := w.Offset(o, x, y)
				w.Terrain(p) // generates the chunk
				cells += world.ChunkSize * world.ChunkSize
			}
		}
	}
	n := gen.Fallbacks() - before
	t.Logf("%d fallbacks in about %d cells", n, cells)
	if float64(n) > 0.001*float64(cells) {
		t.Errorf("%d fallbacks in %d cells is too many", n, cells)
	}
}

// TestChunksAreDeterministic regenerates chunks after eviction and compares.
func TestChunksAreDeterministic(t *testing.T) {
	read := func(w *world.World, o world.Pos) []world.Tile {
		var ts []world.Tile
		for y := int64(0); y < 64; y++ {
			for x := int64(0); x < 64; x++ {
				p, _ := w.Offset(o, x, y)
				for l := 0; l < world.NumLayers; l++ {
					ts = append(ts, w.Tile(p, l))
				}
			}
		}
		return ts
	}
	w1, w2 := loadWorld(t), loadWorld(t)
	for _, o := range coastalSample(t, w1, 6) {
		// Generate in different orders: w2 starts from the far corner.
		far, _ := w2.Offset(o, 40, 40)
		w2.Terrain(far)
		a, b := read(w1, o), read(w2, o)
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("window at %v differs at %d", o, i)
			}
		}
	}
}

// TestPoolIslands checks that pool islands are placed in open sea only,
// keep their shape, and get shores generated around them.
func TestPoolIslands(t *testing.T) {
	w := loadWorld(t)
	found := 0
	for i := int64(0); i < 400 && found < 5; i++ {
		x := int64(gen.Hash(11, i, 0) % uint64(w.W))
		y := int64(gen.Hash(11, i, 1) % uint64(w.H))
		for _, pl := range w.PoolIn(world.Pos{Map: world.Main, X: x, Y: y}) {
			found++
			// The centre is the islet's land, the ring 2 tiles outside its
			// rectangle is water.
			c, _ := w.Offset(pl.Origin, pl.W/2, pl.H/2)
			if w.Terrain(c) != world.Land {
				t.Errorf("%s at %v: centre is %v", pl.Name, pl.Origin, w.Terrain(c))
			}
			for dx := int64(-2); dx < pl.W+2; dx++ {
				for _, dy := range []int64{-2, pl.H + 1} {
					p, _ := w.Offset(pl.Origin, dx, dy)
					if !w.Terrain(p).IsWater() {
						t.Errorf("%s at %v: land at %v next to the islet", pl.Name, pl.Origin, p)
					}
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no pool islands placed in 400 areas")
	}
}

// TestGenerationSpeed keeps chunk generation fast enough to stream.
func TestGenerationSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	w := loadWorld(t)
	o := world.Pos{Map: world.Main, X: 0, Y: 193760}
	start := time.Now()
	n := 0
	for cy := int64(0); cy < 8; cy++ {
		for cx := int64(0); cx < 8; cx++ {
			p, _ := w.Offset(o, cx*world.ChunkSize, cy*world.ChunkSize)
			w.Terrain(p)
			n++
		}
	}
	per := time.Since(start) / time.Duration(n)
	t.Logf("%v per chunk", per)
	if per > 20*time.Millisecond {
		t.Errorf("generating a chunk takes %v", per)
	}
}
