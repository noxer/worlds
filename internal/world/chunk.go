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
	layers [NumLayers][chunkCells]Tile
	solid  [chunkCells]bool
}

// Tile returns the tile at local coordinates in a layer.
func (c *Chunk) Tile(layer, lx, ly int) Tile { return c.layers[layer][ly*ChunkSize+lx] }

func (c *Chunk) computeSolid(ts *Tilesets) {
	for i := range c.solid {
		c.solid[i] = ts.Solid(c.layers[LayerGround][i]) || ts.Solid(c.layers[LayerDeco][i])
	}
}

// ChunkBuilder is handed to generators to fill a chunk.
type ChunkBuilder struct {
	// CX, CY are the chunk coordinates; tile (lx, ly) of the chunk is at
	// main-map position (CX*ChunkSize+lx, CY*ChunkSize+ly).
	CX, CY int64
	c      *Chunk
	owned  func(x, y int64) bool
	w, h   int64
}

// Wrap normalises main-map coordinates. Generators must wrap the coordinates
// of neighbouring cells they look at, or features at the world's seams come
// out differently on both sides.
func (b *ChunkBuilder) Wrap(x, y int64) (int64, int64) {
	if b.w <= 0 {
		return x, y
	}
	return geom.Mod(x, b.w), geom.Mod(y, b.h)
}

// Painted reports whether a hand-made patch paints the main-map cell (x, y),
// which may lie outside this chunk. Painted cells replace generated ones, so
// generators use this to keep multi-tile features (tall trees, later
// buildings) from being cut in half at a patch's border.
func (b *ChunkBuilder) Painted(x, y int64) bool { return b.owned != nil && b.owned(x, y) }

// Set places tile t at local coordinates in a layer.
func (b *ChunkBuilder) Set(layer, lx, ly int, t Tile) { b.c.layers[layer][ly*ChunkSize+lx] = t }

// Get returns the tile at local coordinates in a layer.
func (b *ChunkBuilder) Get(layer, lx, ly int) Tile { return b.c.layers[layer][ly*ChunkSize+lx] }

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
