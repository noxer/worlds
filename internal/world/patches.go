package world

import (
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/noxer/worlds/internal/geom"
)

// AreaSize is the edge length in tiles of the areas of the main map that
// pool patches are placed in (at most one per area).
const AreaSize = 512

// Surroundings a patch can require (patch property "surround").
const (
	SurroundLand  = "land"
	SurroundOcean = "ocean"
)

// DefaultSurroundMargin is the width in tiles of a patch's surroundings
// when the patch sets "surround" without "margin".
const DefaultSurroundMargin = 8

// Placer is implemented by generators that can host pool patches. Suitable
// reports whether the rectangle (x, y, w, h), plus margin tiles around it,
// is open ocean in the generator's own terrain, ignoring patches. It must not
// call back into the world (Painted, Surround), since the world calls it
// while deciding where patches go.
type Placer interface {
	Suitable(seed uint64, x, y, w, h, margin int64) bool
}

// Prefetcher is implemented by generators that prepare data for an area
// ahead of time (in the background). World.Prefetch calls it when the
// anchor enters another chunk.
type Prefetcher interface {
	Prefetch(x, y int64)
}

// Previewer is implemented by generators that can estimate the terrain of
// any position cheaply, for overview and world maps.
type Previewer interface {
	Preview(seed uint64, x, y int64) Terrain
}

// Preview estimates the terrain at a main-map position without generating
// chunks; false if the generator can't.
func (w *World) Preview(x, y int64) (Terrain, bool) {
	p, ok := w.gen.(Previewer)
	if !ok {
		return Land, false
	}
	return p.Preview(w.Seed, x, y), true
}

// PoolChance is the chance, in percent, that an area gets a pool patch
// (if a suitable spot is found).
const PoolChance = 60

// patchSurround reads the "surround" and "margin" properties of a patch.
func patchSurround(src *sourceMap, defaultSurround string) (string, int64, error) {
	s := src.props.String("surround")
	if s == "" {
		s = defaultSurround
	}
	switch s {
	case "", SurroundLand, SurroundOcean:
	default:
		return "", 0, fmt.Errorf("patch %s: surround must be %q or %q, not %q", src.id, SurroundLand, SurroundOcean, s)
	}
	margin := int64(src.props.Int("margin", DefaultSurroundMargin))
	if margin < 0 || margin > AreaSize/4 {
		return "", 0, fmt.Errorf("patch %s: margin %d out of range", src.id, margin)
	}
	if s == "" {
		margin = 0
	}
	return s, margin, nil
}

// indexPatch registers a patch's footprint and its surroundings by chunk.
func (w *World) indexPatch(p *patch) {
	cw, ch := w.W/ChunkSize, w.H/ChunkSize
	each := func(x0, y0, x1, y1 int64, fn func(key [2]int64)) {
		for cy := geom.FloorDiv(y0, ChunkSize); cy <= geom.FloorDiv(y1, ChunkSize); cy++ {
			for cx := geom.FloorDiv(x0, ChunkSize); cx <= geom.FloorDiv(x1, ChunkSize); cx++ {
				fn([2]int64{geom.Mod(cx, cw), geom.Mod(cy, ch)})
			}
		}
	}
	each(p.x, p.y, p.x+p.src.w-1, p.y+p.src.h-1, func(k [2]int64) { w.patchIdx[k] = append(w.patchIdx[k], p) })
	if p.surround != "" {
		m := p.margin
		each(p.x-m, p.y-m, p.x+p.src.w-1+m, p.y+p.src.h-1+m, func(k [2]int64) { w.surroundIdx[k] = append(w.surroundIdx[k], p) })
	}
}

// patchesNear returns the fixed patches touching chunk (cx, cy) plus the pool
// patches placed in its area.
func (w *World) patchesNear(cx, cy int64, surroundings bool) []*patch {
	key := [2]int64{cx, cy}
	var ps []*patch
	if surroundings {
		ps = append(ps, w.surroundIdx[key]...)
	} else {
		ps = append(ps, w.patchIdx[key]...)
	}
	if len(w.pool) > 0 {
		ps = append(ps, w.poolIn(geom.FloorDiv(cx*ChunkSize, AreaSize), geom.FloorDiv(cy*ChunkSize, AreaSize))...)
	}
	return ps
}

// patchCell returns the patch painting main-map cell (x, y) and the cell's
// coordinates inside it.
func (w *World) patchCell(x, y int64) (p *patch, px, py int64) {
	x, y = geom.Mod(x, w.W), geom.Mod(y, w.H)
	for _, p := range w.patchesNear(geom.FloorDiv(x, ChunkSize), geom.FloorDiv(y, ChunkSize), false) {
		px, py := geom.Mod(x-p.x, w.W), geom.Mod(y-p.y, w.H)
		if px < p.src.w && py < p.src.h && p.src.owns(px, py) {
			return p, px, py
		}
	}
	return nil, 0, 0
}

// painted reports whether a patch paints main-map cell (x, y).
func (w *World) painted(x, y int64) bool {
	p, _, _ := w.patchCell(x, y)
	return p != nil
}

// paintedTerrain returns the terrain a patch paints at (x, y).
func (w *World) paintedTerrain(x, y int64) (Terrain, bool) {
	p, px, py := w.patchCell(x, y)
	if p == nil {
		return Land, false
	}
	return w.tilesets.Terrain(p.src.tile(LayerGround, px, py)), true
}

// surroundAt returns the surroundings required at (x, y) by the nearest
// patch whose margin covers it, and the distance in tiles to that patch
// (Chebyshev distance to its rectangle; 0 inside).
func (w *World) surroundAt(x, y int64) (kind string, dist int64, ok bool) {
	x, y = geom.Mod(x, w.W), geom.Mod(y, w.H)
	best := int64(-1)
	for _, p := range w.patchesNear(geom.FloorDiv(x, ChunkSize), geom.FloorDiv(y, ChunkSize), true) {
		if p.surround == "" {
			continue
		}
		dx := axisDist(geom.WrapDelta(p.x, x, w.W), p.src.w)
		dy := axisDist(geom.WrapDelta(p.y, y, w.H), p.src.h)
		d := max(dx, dy)
		if d <= p.margin && (best < 0 || d < best) {
			best, kind = d, p.surround
		}
	}
	return kind, best, best >= 0
}

// axisDist is the distance from offset d (relative to a span's start) to the
// span [0, size).
func axisDist(d, size int64) int64 {
	switch {
	case d < 0:
		return -d
	case d >= size:
		return d - size + 1
	}
	return 0
}

// loadPool reads the pool patches: hand-made pieces the world places itself.
func (w *World) loadPool(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if path.Ext(e.Name()) != ".tmj" {
			continue
		}
		src, err := loadSource(fsys, path.Join(dir, e.Name()), w.tilesets)
		if err != nil {
			return err
		}
		if len(src.objects) > 0 {
			return fmt.Errorf("pool patch %s: objects are not supported yet (they would need one instance per placement)", src.id)
		}
		surround, margin, err := patchSurround(src, SurroundOcean)
		if err != nil {
			return err
		}
		weight := src.props.Int("weight", 1)
		if weight <= 0 {
			return fmt.Errorf("pool patch %s: weight must be positive", src.id)
		}
		if src.w+2*margin > AreaSize/2 || src.h+2*margin > AreaSize/2 {
			return fmt.Errorf("pool patch %s is too large for an area of %d tiles", src.id, AreaSize)
		}
		w.pool = append(w.pool, poolEntry{src: src, surround: surround, margin: margin, weight: weight})
	}
	sort.Slice(w.pool, func(i, j int) bool { return w.pool[i].src.id < w.pool[j].src.id })
	return nil
}

type poolEntry struct {
	src      *sourceMap
	surround string
	margin   int64
	weight   int
}

// poolIn returns the pool patches placed in area (ax, ay). Placement is a
// pure function of the seed and the area, so every player finds the same
// islands, and it is computed lazily and cached.
func (w *World) poolIn(ax, ay int64) []*patch {
	if w.W%AreaSize != 0 || w.H%AreaSize != 0 {
		return nil // the world is too small for areas (tests)
	}
	ax, ay = geom.Mod(ax, w.W/AreaSize), geom.Mod(ay, w.H/AreaSize)
	key := [2]int64{ax, ay}
	// Chunks are built on background workers too, so the cache is locked.
	// Placement is deterministic: if two builders race, both compute the
	// same result.
	w.poolMu.Lock()
	ps, ok := w.poolCache[key]
	w.poolMu.Unlock()
	if ok {
		return ps
	}
	if p := w.placePool(ax, ay); p != nil {
		ps = []*patch{p}
	}
	w.poolMu.Lock()
	w.poolCache[key] = ps
	w.poolMu.Unlock()
	return ps
}

func (w *World) placePool(ax, ay int64) *patch {
	placer, ok := w.gen.(Placer)
	if !ok {
		return nil
	}
	h := mix(mix(w.Seed^0x706f6f6c, uint64(ax)), uint64(ay))
	if h%100 >= PoolChance {
		return nil
	}
	total := 0
	for _, e := range w.pool {
		total += e.weight
	}
	pick := int((h >> 8) % uint64(total))
	var e poolEntry
	for _, c := range w.pool {
		if pick < c.weight {
			e = c
			break
		}
		pick -= c.weight
	}
	// Somewhere inside the area, keeping the margin inside it too, so a
	// placement never affects chunks of another area.
	spanX := AreaSize - e.src.w - 2*e.margin
	spanY := AreaSize - e.src.h - 2*e.margin
	x := ax*AreaSize + e.margin + int64((h>>24)%uint64(spanX))
	y := ay*AreaSize + e.margin + int64((h>>40)%uint64(spanY))
	if !placer.Suitable(w.Seed, x, y, e.src.w, e.src.h, e.margin) {
		return nil
	}
	// Never next to a hand-placed patch.
	for cy := geom.FloorDiv(y-e.margin, ChunkSize); cy <= geom.FloorDiv(y+e.src.h+e.margin, ChunkSize); cy++ {
		for cx := geom.FloorDiv(x-e.margin, ChunkSize); cx <= geom.FloorDiv(x+e.src.w+e.margin, ChunkSize); cx++ {
			k := [2]int64{geom.Mod(cx, w.W/ChunkSize), geom.Mod(cy, w.H/ChunkSize)}
			if len(w.patchIdx[k]) > 0 || len(w.surroundIdx[k]) > 0 {
				return nil
			}
		}
	}
	return &patch{src: e.src, x: x, y: y, surround: e.surround, margin: e.margin, pool: true}
}

// PoolPlacement describes a pool patch placed by the world.
type PoolPlacement struct {
	Name   MapID
	Origin Pos
	W, H   int64
}

// PoolIn returns the pool patches placed in the area containing main-map
// position p.
func (w *World) PoolIn(p Pos) []PoolPlacement {
	var out []PoolPlacement
	for _, pp := range w.poolIn(geom.FloorDiv(p.X, AreaSize), geom.FloorDiv(p.Y, AreaSize)) {
		out = append(out, PoolPlacement{Name: pp.src.id, Origin: Pos{Map: Main, X: pp.x, Y: pp.y}, W: pp.src.w, H: pp.src.h})
	}
	return out
}
