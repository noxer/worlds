package gen

import (
	"fmt"

	"github.com/noxer/worlds/internal/world"
)

// Grassland is a placeholder generator until the biome generators exist:
// grass with flowers, bushes, small trees and tall two-tile trees, using Tiny
// Town tiles.
type Grassland struct {
	grass, grass2, flowers world.Tile
	trees                  []tree
}

// tree is a single-tile tree, or a tall tree whose top goes on the overhead
// layer one tile above (the party can walk behind it). small is used instead
// of a tall tree when there is no room for its top.
type tree struct {
	bottom, top, small world.Tile
}

func (t tree) tall() bool { return !t.top.Empty() }

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
	// Which trees are tall, and their tops, comes from the tileset's "above"
	// property. small is the one-tile fallback for tall trees.
	for _, t := range []tree{
		{bottom: tile(16), small: tile(28)}, // tall green
		{bottom: tile(16), small: tile(28)},
		{bottom: tile(15), small: tile(27)}, // tall orange
		{bottom: tile(28)},                  // small green
		{bottom: tile(27)},                  // small orange
		{bottom: tile(5)},                   // bush
	} {
		if top, ok := ts.Above(t.bottom); ok {
			if t.small.Empty() && err == nil {
				err = fmt.Errorf("grassland: tall tree tile %d needs a one-tile fallback", t.bottom.Index())
			}
			t.top = top
		}
		g.trees = append(g.trees, t)
	}
	return g, err
}

// treeAt returns the tree rooted at (x, y), if any. A tall tree needs both
// its cells free of patches; otherwise it is replaced by its small variant.
// The decision depends only on the seed, the coordinates and the patches,
// so neighbouring chunks agree about trees whose top crosses their border.
func (g *Grassland) treeAt(seed uint64, b *world.ChunkBuilder, x, y int64) (bottom, top world.Tile, ok bool) {
	x, y = b.Wrap(x, y)
	h := Hash(seed, x, y)
	if (h>>16)%100 >= 6 {
		return 0, 0, false
	}
	t := g.trees[(h>>32)%uint64(len(g.trees))]
	if !t.tall() {
		return t.bottom, 0, true
	}
	if ax, ay := b.Wrap(x, y-1); b.Painted(x, y) || b.Painted(ax, ay) {
		return t.small, 0, true
	}
	return t.bottom, t.top, true
}

// Generate implements world.Generator.
func (g *Grassland) Generate(seed uint64, b *world.ChunkBuilder) {
	for ly := 0; ly < world.ChunkSize; ly++ {
		for lx := 0; lx < world.ChunkSize; lx++ {
			x, y := b.CX*world.ChunkSize+int64(lx), b.CY*world.ChunkSize+int64(ly)
			switch r := Hash(seed, x, y) % 100; {
			case r < 4:
				b.Set(world.LayerGround, lx, ly, g.flowers)
			case r < 22:
				b.Set(world.LayerGround, lx, ly, g.grass2)
			default:
				b.Set(world.LayerGround, lx, ly, g.grass)
			}
			if bottom, _, ok := g.treeAt(seed, b, x, y); ok {
				b.Set(world.LayerDeco, lx, ly, bottom)
			}
			// The top of a tall tree rooted one tile below.
			if _, top, ok := g.treeAt(seed, b, x, y+1); ok && !top.Empty() {
				b.Set(world.LayerOverhead, lx, ly, top)
			}
		}
	}
}
