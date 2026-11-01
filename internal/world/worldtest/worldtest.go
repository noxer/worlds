// Package worldtest builds small worlds from ASCII art for tests.
package worldtest

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/noxer/worlds/internal/world"
)

// Map describes a patch or detail map. Rows use '.' for floor and '#' for
// walls; ' ' leaves a patch cell unpainted (generated). 'T' is the lower part
// of a two-tile tree (solid), 't' its upper part on the overhead layer above
// a floor cell. 'w' is shallow water, 'W' deep water.
type Map struct {
	Name   string
	X, Y   int64 // patch origin on the main map (ignored for detail maps)
	Rows   []string
	Spawns map[string][2]int
	Warps  []Warp
}

// Warp is a single-tile warp to a spawn. ToMap is "main" or a detail map.
type Warp struct {
	X, Y           int
	ToMap, ToSpawn string
}

// Spec describes a test world.
type Spec struct {
	Width, Height int64 // default 64x64
	// Fill is what the generator produces outside patches: '.' floor,
	// '#' wall (default).
	Fill    byte
	Patches []Map
	Details []Map
}

// fill generates every cell with one tile.
type fill struct{ t world.Tile }

func (f fill) Generate(_ uint64, b *world.ChunkBuilder) {
	for y := 0; y < world.ChunkSize; y++ {
		for x := 0; x < world.ChunkSize; x++ {
			b.Set(world.LayerGround, x, y, f.t)
		}
	}
}

// Config renders the spec as Tiled files and returns a world config for it.
func (s Spec) Config() (fstest.MapFS, world.Config) {
	if s.Width == 0 {
		s.Width, s.Height = 64, 64
	}
	fsys := fstest.MapFS{}
	put := func(name string, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		fsys[name] = &fstest.MapFile{Data: b}
	}
	put("tilesets/test.tsj", map[string]any{
		"name": "test", "tilewidth": 16, "tileheight": 16, "tilecount": 6, "columns": 6,
		"image": "test.png", "imagewidth": 96, "imageheight": 16,
		"tiles": []any{
			map[string]any{"id": 1, "properties": []any{
				map[string]any{"name": "solid", "type": "bool", "value": true}}},
			map[string]any{"id": 2, "properties": []any{
				map[string]any{"name": "solid", "type": "bool", "value": true},
				map[string]any{"name": "above", "type": "int", "value": 3}}},
			map[string]any{"id": 4, "properties": []any{
				map[string]any{"name": "terrain", "type": "string", "value": "shallow"}}},
			map[string]any{"id": 5, "properties": []any{
				map[string]any{"name": "terrain", "type": "string", "value": "deep"}}},
		},
	})
	tmj := func(m Map) map[string]any {
		h, w := len(m.Rows), len(m.Rows[0])
		data := make([]uint32, 0, w*h)
		over := make([]uint32, 0, w*h)
		for _, row := range m.Rows {
			if len(row) != w {
				panic(fmt.Sprintf("map %s: ragged rows", m.Name))
			}
			for _, c := range row {
				o := uint32(0)
				switch c {
				case '#':
					data = append(data, 2)
				case ' ':
					data = append(data, 0)
				case 'T':
					data = append(data, 3)
				case 't':
					data = append(data, 1)
					o = 4
				case 'w':
					data = append(data, 5)
				case 'W':
					data = append(data, 6)
				default:
					data = append(data, 1)
				}
				over = append(over, o)
			}
		}
		var objs []any
		id := 1
		for name, p := range m.Spawns {
			objs = append(objs, map[string]any{"id": id, "name": name, "type": "spawn",
				"x": p[0] * 16, "y": p[1] * 16, "width": 16, "height": 16})
			id++
		}
		for _, wp := range m.Warps {
			objs = append(objs, map[string]any{"id": id, "type": "warp",
				"x": wp.X * 16, "y": wp.Y * 16, "width": 16, "height": 16,
				"properties": []any{
					map[string]any{"name": "to_map", "type": "string", "value": wp.ToMap},
					map[string]any{"name": "to_spawn", "type": "string", "value": wp.ToSpawn},
				}})
			id++
		}
		return map[string]any{
			"width": w, "height": h, "tilewidth": 16, "tileheight": 16,
			"tilesets": []any{map[string]any{"firstgid": 1, "source": "../tilesets/test.tsj"}},
			"layers": []any{
				map[string]any{"name": "ground", "type": "tilelayer", "width": w, "height": h, "data": data},
				map[string]any{"name": "overhead", "type": "tilelayer", "width": w, "height": h, "data": over},
				map[string]any{"name": "objects", "type": "objectgroup", "objects": objs},
			},
		}
	}
	var placed []any
	for _, m := range s.Patches {
		put("patches/"+m.Name+".tmj", tmj(m))
		placed = append(placed, map[string]any{"fileName": m.Name + ".tmj",
			"x": m.X * 16, "y": m.Y * 16, "width": len(m.Rows[0]) * 16, "height": len(m.Rows) * 16})
	}
	put("patches/main.world", map[string]any{"type": "world", "maps": placed})
	for _, m := range s.Details {
		put("detail/"+m.Name+".tmj", tmj(m))
	}
	fsys["detail/.keep"] = &fstest.MapFile{}
	idx := 1
	if s.Fill == '.' {
		idx = 0
	}
	cfg := world.Config{
		Seed: 1, Width: s.Width, Height: s.Height,
		Tilesets: []string{"tilesets/test.tsj"},
		Patches:  "patches/main.world",
		Details:  "detail",
		Generator: func(g world.GenInfo) (world.Generator, error) {
			return fill{g.Tilesets.MustTile("test", idx)}, nil
		},
	}
	return fsys, cfg
}

// Load builds and loads a world, failing the test on error.
func Load(t testing.TB, s Spec) *world.World {
	t.Helper()
	fsys, cfg := s.Config()
	w, err := world.Load(fsys, cfg)
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	return w
}

// Pos is shorthand for a position on a map: "main" or a detail map.
func Pos(m string, x, y int64) world.Pos {
	if m == "main" {
		return world.Pos{Map: world.Main, X: x, Y: y}
	}
	return world.Pos{Map: world.Detail(world.MapID(m)), X: x, Y: y}
}
