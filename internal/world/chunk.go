package world

import (
	"container/list"

	"github.com/noxer/worlds/internal/geom"
)

const chunkCells = ChunkSize * ChunkSize

// Chunk is a 32x32 block of tiles. Resolved chunks are immutable; dynamic
// state (blockers, later puzzle state and save deltas) is kept separately and
// survives cache eviction.
type Chunk struct {
	layers  [NumLayers][chunkCells]Tile
	solid   [chunkCells]bool
	terrain [chunkCells]Terrain
}

// Tile returns the tile at local coordinates in a layer.
func (c *Chunk) Tile(layer, lx, ly int) Tile { return c.layers[layer][ly*ChunkSize+lx] }

// computeDerived fills collision and terrain from the tiles.
func (c *Chunk) computeDerived(ts *Tilesets) {
	for i := range c.solid {
		c.solid[i] = ts.Solid(c.layers[LayerGround][i]) || ts.Solid(c.layers[LayerDeco][i])
		c.terrain[i] = ts.Terrain(c.layers[LayerGround][i])
	}
}

// ChunkBuilder is handed to generators to fill a chunk.
type ChunkBuilder struct {
	// CX, CY are the chunk coordinates; tile (lx, ly) of the chunk is at
	// main-map position (CX*ChunkSize+lx, CY*ChunkSize+ly).
	CX, CY int64
	c      *Chunk
	w      *World
	ww, wh int64
}

// Wrap normalises main-map coordinates. Generators must wrap the coordinates
// of neighbouring cells they look at, or features at the world's seams come
// out differently on both sides.
func (b *ChunkBuilder) Wrap(x, y int64) (int64, int64) {
	if b.ww <= 0 {
		return x, y
	}
	return geom.Mod(x, b.ww), geom.Mod(y, b.wh)
}

// Painted reports whether a hand-made patch paints the main-map cell (x, y),
// which may lie outside this chunk. Painted cells replace generated ones, so
// generators use this to keep multi-tile features (tall trees, later
// buildings) from being cut in half at a patch's border.
func (b *ChunkBuilder) Painted(x, y int64) bool { return b.w != nil && b.w.painted(x, y) }

// PaintedTerrain returns the terrain a patch paints at (x, y). Generators
// treat it as a fixed constraint, so coastlines continue into patches.
func (b *ChunkBuilder) PaintedTerrain(x, y int64) (Terrain, bool) {
	if b.w == nil {
		return Land, false
	}
	return b.w.paintedTerrain(x, y)
}

// Surround returns what a nearby patch requires around itself at (x, y):
// SurroundLand or SurroundOcean, and the distance in tiles to the patch.
func (b *ChunkBuilder) Surround(x, y int64) (kind string, dist int64, ok bool) {
	if b.w == nil {
		return "", 0, false
	}
	return b.w.surroundAt(x, y)
}

// Set places tile t at local coordinates in a layer.
func (b *ChunkBuilder) Set(layer, lx, ly int, t Tile) { b.c.layers[layer][ly*ChunkSize+lx] = t }

// Get returns the tile at local coordinates in a layer.
func (b *ChunkBuilder) Get(layer, lx, ly int) Tile { return b.c.layers[layer][ly*ChunkSize+lx] }

// GenInfo is what a generator is created with.
type GenInfo struct {
	Tilesets *Tilesets
	W, H     int64 // main map size in tiles; noise must repeat with these periods
}

// Generator produces the procedural base of main-map chunks. It must be a
// pure function of its arguments so that every player and every platform
// sees the same world, and chunks can be generated in any order.
type Generator interface {
	Generate(seed uint64, b *ChunkBuilder)
}

// chunkCache is a least-recently-used cache of resolved main-map chunks.
type chunkCache struct {
	cap   int
	order *list.List // front = most recently used
	items map[[2]int64]*list.Element
}

type cacheEntry struct {
	key [2]int64
	c   *Chunk
}

func newChunkCache(capacity int) *chunkCache {
	return &chunkCache{cap: capacity, order: list.New(), items: map[[2]int64]*list.Element{}}
}

func (cc *chunkCache) get(key [2]int64) *Chunk {
	if e, ok := cc.items[key]; ok {
		cc.order.MoveToFront(e)
		return e.Value.(*cacheEntry).c
	}
	return nil
}

func (cc *chunkCache) put(key [2]int64, c *Chunk) {
	cc.items[key] = cc.order.PushFront(&cacheEntry{key, c})
	for cc.order.Len() > cc.cap {
		e := cc.order.Back()
		cc.order.Remove(e)
		delete(cc.items, e.Value.(*cacheEntry).key)
	}
}

func (cc *chunkCache) len() int { return cc.order.Len() }
