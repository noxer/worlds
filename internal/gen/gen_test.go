package gen_test

import (
	"hash/fnv"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/gen"
	"github.com/noxer/worlds/internal/world"
)

// TestFingerprint pins the generated world. If this fails, a change altered
// what players see: update the value only if that was intended.
func TestFingerprint(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	w := c.World
	h := fnv.New64a()
	chunks := [][2]int64{
		{0, 0}, {12345, 6789}, {w.W/world.ChunkSize - 1, w.H/world.ChunkSize - 1}, // inland
		{1, 193824 / world.ChunkSize},                       // Gull Isle: a patch with generated shores
		{3, 193920 / world.ChunkSize},                       // coast and open sea south of the start
		{w.W/world.ChunkSize - 1, 193792 / world.ChunkSize}, // the path from Brookhaven's south gate
		{2802 / world.ChunkSize, 193995 / world.ChunkSize},  // the square of the village east of it
		{21704 / world.ChunkSize, 44829 / world.ChunkSize},  // a rail curve
		{9392 / world.ChunkSize, 203778 / world.ChunkSize},  // a pier
		{40027 / world.ChunkSize, 90950 / world.ChunkSize},  // a city's station
	}
	for _, ch := range chunks {
		for y := int64(0); y < world.ChunkSize; y++ {
			for x := int64(0); x < world.ChunkSize; x++ {
				p := world.Pos{Map: world.Main, X: ch[0]*world.ChunkSize + x, Y: ch[1]*world.ChunkSize + y}
				for l := 0; l < world.NumLayers; l++ {
					v := uint32(w.Tile(p, l))
					h.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
				}
			}
		}
	}
	const want = 0x685f1dff8ad62d2a
	if got := h.Sum64(); got != want {
		t.Errorf("fingerprint = %#x, want %#x", got, uint64(want))
	}
}

func TestHashSpreads(t *testing.T) {
	seen := map[uint64]bool{}
	for x := int64(-50); x < 50; x++ {
		for y := int64(-50); y < 50; y++ {
			seen[gen.Hash(1, x, y)] = true
		}
	}
	if len(seen) != 10000 {
		t.Fatalf("hash collisions: %d distinct of 10000", len(seen))
	}
	if gen.Hash(1, 2, 3) == gen.Hash(2, 2, 3) || gen.Hash(1, 2, 3) == gen.Hash(1, 3, 2) {
		t.Fatal("hash ignores seed or argument order")
	}
}
