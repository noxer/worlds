package world

import (
	"fmt"
	"io/fs"
	"path"
)

// Tileset is a tile sheet.
type Tileset struct {
	Name      string
	Image     string // path of the sheet image inside the asset FS
	Columns   int
	TileCount int
	Margin    int
	Spacing   int
	solid     []bool
}

// Solid reports whether tile index i blocks movement.
func (t *Tileset) Solid(i int) bool { return i >= 0 && i < len(t.solid) && t.solid[i] }

// Tile is a compact tile reference (4 bytes, chunks hold many of them).
// Layout: bit 31 flip H, bit 30 flip V, bits 20-29 tileset number + 1,
// bits 0-19 tile index. Zero is the empty tile.
type Tile uint32

const (
	tileFlipH    = 1 << 31
	tileFlipV    = 1 << 30
	tileSetShift = 20
	tileSetMask  = 0x3ff
	tileIdxMask  = 1<<tileSetShift - 1
)

// Empty reports whether the cell holds no tile.
func (t Tile) Empty() bool { return t == 0 }

// Set returns the number of the tileset in the world's registry.
func (t Tile) Set() int { return int(t>>tileSetShift&tileSetMask) - 1 }

// Index returns the tile index in its tileset.
func (t Tile) Index() int { return int(t & tileIdxMask) }

// FlipH reports horizontal mirroring.
func (t Tile) FlipH() bool { return t&tileFlipH != 0 }

// FlipV reports vertical mirroring.
func (t Tile) FlipV() bool { return t&tileFlipV != 0 }

// Tilesets is the registry of all tilesets used by the world.
type Tilesets struct {
	list   []*Tileset
	byPath map[string]int
	byName map[string]int
}

func newTilesets() *Tilesets {
	return &Tilesets{byPath: map[string]int{}, byName: map[string]int{}}
}

// All returns all tilesets; Tile.Set indexes into it.
func (ts *Tilesets) All() []*Tileset { return ts.list }

// Get returns tileset number i.
func (ts *Tilesets) Get(i int) *Tileset { return ts.list[i] }

// Tile returns a tile reference by tileset name and index.
func (ts *Tilesets) Tile(name string, index int) (Tile, error) {
	i, ok := ts.byName[name]
	if !ok {
		return 0, fmt.Errorf("unknown tileset %q", name)
	}
	if index < 0 || index >= ts.list[i].TileCount {
		return 0, fmt.Errorf("tileset %q has no tile %d", name, index)
	}
	return makeTile(i, index, false, false), nil
}

// MustTile is Tile for use in generator setup with known-good arguments.
func (ts *Tilesets) MustTile(name string, index int) Tile {
	t, err := ts.Tile(name, index)
	if err != nil {
		panic(err)
	}
	return t
}

// Solid reports whether a tile blocks movement.
func (ts *Tilesets) Solid(t Tile) bool {
	return !t.Empty() && ts.list[t.Set()].Solid(t.Index())
}

func makeTile(set, index int, flipH, flipV bool) Tile {
	t := Tile(set+1)<<tileSetShift | Tile(index)
	if flipH {
		t |= tileFlipH
	}
	if flipV {
		t |= tileFlipV
	}
	return t
}

// load returns the number of the tileset at name, loading it once.
func (ts *Tilesets) load(fsys fs.FS, name string) (int, error) {
	if i, ok := ts.byPath[name]; ok {
		return i, nil
	}
	var tt tiledTileset
	if err := readJSON(fsys, name, &tt); err != nil {
		return 0, err
	}
	if tt.TileWidth != TileSize || tt.TileHeight != TileSize {
		return 0, fmt.Errorf("%s: tiles must be %dx%d", name, TileSize, TileSize)
	}
	if tt.Image == "" {
		return 0, fmt.Errorf("%s: image collection tilesets are not supported", name)
	}
	if tt.TileCount > tileIdxMask {
		return 0, fmt.Errorf("%s: too many tiles", name)
	}
	if _, dup := ts.byName[tt.Name]; dup {
		return 0, fmt.Errorf("%s: duplicate tileset name %q", name, tt.Name)
	}
	if len(ts.list) >= tileSetMask {
		return 0, fmt.Errorf("too many tilesets")
	}
	set := &Tileset{
		Name:      tt.Name,
		Image:     path.Join(path.Dir(name), tt.Image),
		Columns:   tt.Columns,
		TileCount: tt.TileCount,
		Margin:    tt.Margin,
		Spacing:   tt.Spacing,
		solid:     make([]bool, tt.TileCount),
	}
	for _, t := range tt.Tiles {
		if t.ID < 0 || t.ID >= tt.TileCount {
			return 0, fmt.Errorf("%s: tile id %d out of range", name, t.ID)
		}
		set.solid[t.ID] = toProps(t.Properties).Bool("solid")
	}
	i := len(ts.list)
	ts.list = append(ts.list, set)
	ts.byPath[name] = i
	ts.byName[set.Name] = i
	return i, nil
}
