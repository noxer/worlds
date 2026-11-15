package world

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
)

// Prefab is a hand-made piece (a house, a garden, a well) that generators
// place, for example in villages. Its tiles override the generated ones
// where they are set; its objects (doors) become generated objects.
type Prefab struct {
	Name    MapID
	W, H    int64
	Props   Props
	Objects []Object // positions relative to the top-left corner
	src     *sourceMap
}

// Tile returns the prefab's tile at (x, y) in a layer.
func (p *Prefab) Tile(layer int, x, y int64) Tile { return p.src.tile(layer, x, y) }

// Prefabs returns the loaded prefabs, sorted by name.
func (w *World) Prefabs() []*Prefab { return w.prefabs }

func loadPrefabs(fsys fs.FS, dir string, ts *Tilesets) ([]*Prefab, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []*Prefab
	for _, e := range entries {
		if path.Ext(e.Name()) != ".tmj" {
			continue
		}
		src, err := loadSource(fsys, path.Join(dir, e.Name()), ts)
		if err != nil {
			return nil, err
		}
		p := &Prefab{Name: src.id, W: src.w, H: src.h, Props: src.props, Objects: src.objects, src: src}
		for _, o := range p.Objects {
			if o.W != 1 || o.H != 1 {
				return nil, fmt.Errorf("prefab %s: object %q must cover one tile", p.Name, o.Name)
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Bump returns the bump dialog of a generated object at a main-map
// position (objects of patches and detail maps are in Objects).
func (w *World) Bump(p Pos) (string, bool) {
	if !p.Map.IsMain() {
		return "", false
	}
	c, lx, ly := w.chunk(p)
	if c == nil || c.bumps == nil {
		return "", false
	}
	d, ok := c.bumps[int32(ly*ChunkSize+lx)]
	return d, ok
}
