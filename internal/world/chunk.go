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
	blocked [chunkCells]bool // solid whatever the tiles say (generated objects)
	bumps   map[int32]string // generated objects' bump dialogs by cell
}

// Tile returns the tile at local coordinates in a layer.
func (c *Chunk) Tile(layer, lx, ly int) Tile { return c.layers[layer][ly*ChunkSize+lx] }

// computeDerived fills collision and terrain from the tiles.
func (c *Chunk) computeDerived(ts *Tilesets) {
	for i := range c.solid {
		c.solid[i] = ts.Solid(c.layers[LayerGround][i]) || ts.Solid(c.layers[LayerDeco][i]) || c.blocked[i]
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

// W and H are the main map's size (for wrapping offsets).
func (b *ChunkBuilder) W() int64 { return b.ww }

// H is the main map's height.
func (b *ChunkBuilder) H() int64 { return b.wh }

// Block makes a cell solid whatever its tiles (a generated object such as
// a locked door).
func (b *ChunkBuilder) Block(lx, ly int) { b.c.blocked[ly*ChunkSize+lx] = true }

// Bump gives a cell a bump dialog: walking into it shows the dialog's
// first line as a passive notice (like the "bump" property of objects).
func (b *ChunkBuilder) Bump(lx, ly int, dialog string) {
	if b.c.bumps == nil {
		b.c.bumps = map[int32]string{}
	}
	b.c.bumps[int32(ly*ChunkSize+lx)] = dialog
}

// Get returns the tile at local coordinates in a layer.
func (b *ChunkBuilder) Get(layer, lx, ly int) Tile { return b.c.layers[layer][ly*ChunkSize+lx] }

// GenInfo is what a generator is created with.
type GenInfo struct {
	Tilesets *Tilesets
	W, H     int64 // main map size in tiles; noise must repeat with these periods
	Seed     uint64
	Patches  []PatchInfo // the hand-made patches (not the pool)
	Prefabs  []*Prefab   // pieces for generators to place, sorted by name
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

// has reports whether key is cached, without counting as a use.
func (cc *chunkCache) has(key [2]int64) bool {
	_, ok := cc.items[key]
	return ok
}
