package gen_test

import (
	"testing"
	"time"

	"github.com/noxer/worlds/internal/world"
)

// waitStreaming pumps until all prefetch requests are delivered.
func waitStreaming(t *testing.T, w *world.World) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for w.Pending() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("prefetch did not finish: %d pending", w.Pending())
		}
		w.Pump()
		time.Sleep(time.Millisecond)
	}
	w.Pump()
}

// TestStreamingMatchesSync builds chunks on background workers and compares
// them with chunks built on demand in a separate world.
func TestStreamingMatchesSync(t *testing.T) {
	bg, fg := loadWorld(t), loadWorld(t)
	bg.StartStreaming(2)
	defer bg.StopStreaming()
	base := bg.Stats().SyncBuilt                           // loading validates warps, which builds a chunk
	anchor := world.Pos{Map: world.Main, X: 40, Y: 193820} // the coast near Gull Isle
	bg.Prefetch(anchor, world.Right, true)
	waitStreaming(t, bg)
	if bg.Stats().Prefetched < 25 {
		t.Fatalf("only %d chunks prefetched", bg.Stats().Prefetched)
	}
	// Everything within the prefetch radius is cached: reading it builds
	// nothing on demand, and matches the foreground world tile for tile.
	acx, acy := anchor.X/world.ChunkSize, anchor.Y/world.ChunkSize
	for cy := acy - world.PrefetchRadius; cy <= acy+world.PrefetchRadius; cy++ {
		for cx := acx - world.PrefetchRadius; cx <= acx+world.PrefetchRadius; cx++ {
			if !bg.IsCached(world.Pos{Map: world.Main, X: cx * world.ChunkSize, Y: cy * world.ChunkSize}) {
				t.Errorf("chunk (%d,%d) not prefetched (anchor chunk %d,%d)", cx, cy, acx, acy)
			}
		}
	}
	for cy := acy - world.PrefetchRadius; cy <= acy+world.PrefetchRadius; cy++ {
		for cx := acx - world.PrefetchRadius; cx <= acx+world.PrefetchRadius; cx++ {
			for y := int64(0); y < world.ChunkSize; y += 3 {
				for x := int64(0); x < world.ChunkSize; x += 3 {
					p, _ := bg.Normalize(world.Pos{Map: world.Main, X: cx*world.ChunkSize + x, Y: cy*world.ChunkSize + y})
					for l := 0; l < world.NumLayers; l++ {
						if a, b := bg.Tile(p, l), fg.Tile(p, l); a != b {
							t.Fatalf("%v layer %d: background %v, foreground %v", p, l, a, b)
						}
					}
				}
			}
		}
	}
	if n := bg.Stats().SyncBuilt - base; n != 0 {
		t.Fatalf("%d chunks had to be built on demand despite prefetching", n)
	}
}

// TestNoStallsAtRidingSpeed moves the party's anchor east at riding speed
// (one tile per 8 ticks) for 40 chunks with inline streaming, the browser's
// mode, touching everything a 320x180 screen shows each tick like drawing
// does. After the first moment, no chunk may have to be built on demand.
func TestNoStallsAtRidingSpeed(t *testing.T) {
	w := loadWorld(t)
	w.StartStreaming(0)
	defer w.StopStreaming()
	pos := world.Pos{Map: world.Main, X: 0, Y: 1000}
	view := func() {
		for dy := int64(-6); dy <= 6; dy++ {
			for dx := int64(-11); dx <= 11; dx++ {
				p, _ := w.Offset(pos, dx, dy)
				w.Solid(p)
			}
		}
	}
	// Settle at the start (the first screen is always built on demand).
	w.Prefetch(pos, world.Right, false)
	view()
	for i := 0; i < 60; i++ {
		w.Pump()
	}
	before := w.Stats().SyncBuilt
	const tilesPerTick = 8
	for tick := 0; tick < 40*world.ChunkSize*tilesPerTick; tick++ {
		if tick%tilesPerTick == 0 {
			pos, _ = w.Offset(pos, 1, 0)
		}
		w.Prefetch(pos, world.Right, true)
		w.Pump()
		view()
	}
	if n := w.Stats().SyncBuilt - before; n != 0 {
		t.Fatalf("%d chunks built on demand while riding (prefetched %d)", n, w.Stats().Prefetched)
	}
	t.Logf("prefetched %d chunks, %d on demand at the start", w.Stats().Prefetched, before)
}
