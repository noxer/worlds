package world

import (
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
)

// Layer slots. Every chunk has the same three layers, so hand-made and
// generated content compose cell by cell.
const (
	LayerGround = iota
	LayerDeco
	LayerOverhead // drawn above characters, no collision
	NumLayers
)

// Object is an object from a Tiled object layer, converted to tiles. For
// main-map patches, X and Y are in main-map coordinates.
type Object struct {
	ID         int
	Name, Type string
	Pos        Pos
	W, H       int64 // tiles covered
	Tile       Tile  // sprite of tile objects (NPCs); empty otherwise
	Props      Props
}

// sourceMap is a finite Tiled map: a detail-map template or a main-map patch.
type sourceMap struct {
	id      MapID
	w, h    int64
	layers  [NumLayers][]Tile
	objects []Object // positions relative to the map's top-left corner
	props   Props
}

func (s *sourceMap) tile(layer int, x, y int64) Tile { return s.layers[layer][y*s.w+x] }

// owns reports whether the source paints cell (x, y) in any layer.
func (s *sourceMap) owns(x, y int64) bool {
	for l := 0; l < NumLayers; l++ {
		if !s.tile(l, x, y).Empty() {
			return true
		}
	}
	return false
}

func loadSource(fsys fs.FS, name string, ts *Tilesets) (*sourceMap, error) {
	var tm tiledMap
	if err := readJSON(fsys, name, &tm); err != nil {
		return nil, err
	}
	if tm.Infinite {
		return nil, fmt.Errorf("%s: infinite maps are not supported", name)
	}
	if tm.TileWidth != TileSize || tm.TileHeight != TileSize {
		return nil, fmt.Errorf("%s: tiles must be %dx%d", name, TileSize, TileSize)
	}
	s := &sourceMap{
		id:    MapID(strings.TrimSuffix(path.Base(name), path.Ext(name))),
		w:     int64(tm.Width),
		h:     int64(tm.Height),
		props: toProps(tm.Properties),
	}
	for l := range s.layers {
		s.layers[l] = make([]Tile, s.w*s.h)
	}

	type ref struct {
		first uint32
		set   int
	}
	var refs []ref
	for _, r := range tm.Tilesets {
		if r.Source == "" {
			return nil, fmt.Errorf("%s: embedded tilesets are not supported, export them as .tsj", name)
		}
		set, err := ts.load(fsys, path.Join(path.Dir(name), r.Source))
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref{r.FirstGID, set})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].first < refs[j].first })
	resolve := func(gid uint32) (Tile, error) {
		id := gid & gidMask
		if id == 0 {
			return 0, nil
		}
		for i := len(refs) - 1; i >= 0; i-- {
			if id >= refs[i].first {
				idx := int(id - refs[i].first)
				if idx >= ts.list[refs[i].set].TileCount {
					break
				}
				return makeTile(refs[i].set, idx, gid&gidFlipH != 0, gid&gidFlipV != 0), nil
			}
		}
		return 0, fmt.Errorf("%s: unknown gid %d", name, id)
	}

	var walk func(ls []tiledLayer) error
	walk = func(ls []tiledLayer) error {
		for _, l := range ls {
			switch l.Type {
			case "group":
				if err := walk(l.Layers); err != nil {
					return err
				}
			case "tilelayer":
				if int64(len(l.Data)) != s.w*s.h {
					return fmt.Errorf("%s: layer %q must use uncompressed CSV data of the map's size", name, l.Name)
				}
				slot := LayerDeco
				switch {
				case toProps(l.Properties).Bool("overhead") || l.Name == "overhead":
					slot = LayerOverhead
				case l.Name == "ground":
					slot = LayerGround
				}
				for i, gid := range l.Data {
					t, err := resolve(gid)
					if err != nil {
						return err
					}
					if t.Empty() {
						continue
					}
					if !s.layers[slot][i].Empty() {
						return fmt.Errorf("%s: layer %q overlaps another layer mapped to the same slot at cell %d", name, l.Name, i)
					}
					s.layers[slot][i] = t
				}
			case "objectgroup":
				for _, o := range l.Objects {
					obj, err := convertObject(o, resolve)
					if err != nil {
						return fmt.Errorf("%s: object %d: %w", name, o.ID, err)
					}
					s.objects = append(s.objects, obj)
				}
			}
		}
		return nil
	}
	if err := walk(tm.Layers); err != nil {
		return nil, err
	}
	return s, nil
}

func convertObject(o tiledObject, resolve func(uint32) (Tile, error)) (Object, error) {
	obj := Object{ID: o.ID, Name: o.Name, Type: o.Type, Props: toProps(o.Properties)}
	if obj.Type == "" {
		obj.Type = o.Class
	}
	x, y, w, h := o.X, o.Y, o.Width, o.Height
	if o.GID != 0 {
		t, err := resolve(o.GID)
		if err != nil {
			return obj, err
		}
		obj.Tile = t
		y -= h // tile objects are anchored at their bottom-left corner
	}
	tx := int64(math.Floor(x / TileSize))
	ty := int64(math.Floor(y / TileSize))
	obj.Pos = Pos{X: tx, Y: ty}
	obj.W = max(1, int64(math.Ceil((x+w)/TileSize))-tx)
	obj.H = max(1, int64(math.Ceil((y+h)/TileSize))-ty)
	return obj, nil
}
