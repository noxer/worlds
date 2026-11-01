package gen

import "github.com/noxer/worlds/internal/world"

// Grassland is a placeholder generator until the biome generators exist:
// grass with flowers and scattered trees, using Tiny Town tiles.
type Grassland struct {
	grass, grass2, flowers world.Tile
	trees                  []world.Tile
}

// NewGrassland creates the grassland generator.
func NewGrassland(ts *world.Tilesets) (world.Generator, error) {
	g := &Grassland{}
	var err error
	tile := func(i int) world.Tile {
		t, e := ts.Tile("tinytown", i)
		if e != nil && err == nil {
			err = e
		}
		return t
	}
	g.grass, g.grass2, g.flowers = tile(0), tile(1), tile(2)
	g.trees = []world.Tile{tile(16), tile(28), tile(16), tile(5), tile(27)}
	return g, err
}

// Generate implements world.Generator.
func (g *Grassland) Generate(seed uint64, b *world.ChunkBuilder) {
	for ly := 0; ly < world.ChunkSize; ly++ {
		for lx := 0; lx < world.ChunkSize; lx++ {
			x, y := b.CX*world.ChunkSize+int64(lx), b.CY*world.ChunkSize+int64(ly)
			h := Hash(seed, x, y)
			switch r := h % 100; {
			case r < 4:
				b.Set(world.LayerGround, lx, ly, g.flowers)
			case r < 22:
				b.Set(world.LayerGround, lx, ly, g.grass2)
			default:
				b.Set(world.LayerGround, lx, ly, g.grass)
			}
			if (h>>16)%100 < 6 {
				b.Set(world.LayerDeco, lx, ly, g.trees[(h>>32)%uint64(len(g.trees))])
			}
		}
	}
}
