// Package worldtest builds small worlds from ASCII art for tests.
package worldtest

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/noxer/worlds/internal/world"
)

// Map describes a test map. Rows use '.' for floor and '#' for walls.
type Map struct {
	Name string
	Rows []string
	// Placed maps are put into one shared region ("overworld") at Origin.
	Placed bool
	Origin [2]int
	Spawns map[string][2]int
	Warps  []Warp
}

// Warp is a single-tile warp to a spawn on another map.
type Warp struct {
	X, Y           int
	ToMap, ToSpawn string
}

// FS renders the maps as Tiled files.
func FS(maps ...Map) fstest.MapFS {
	fsys := fstest.MapFS{}
	put := func(name string, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		fsys[name] = &fstest.MapFile{Data: b}
	}
	put("tilesets/test.tsj", map[string]any{
		"name": "test", "tilewidth": 16, "tileheight": 16, "tilecount": 2, "columns": 2,
		"image": "test.png", "imagewidth": 32, "imageheight": 16,
		"tiles": []any{map[string]any{"id": 1, "properties": []any{
			map[string]any{"name": "solid", "type": "bool", "value": true}}}},
	})
	var placed []any
	for _, m := range maps {
		h, w := len(m.Rows), len(m.Rows[0])
		data := make([]uint32, 0, w*h)
		for _, row := range m.Rows {
			if len(row) != w {
				panic(fmt.Sprintf("map %s: ragged rows", m.Name))
			}
			for _, c := range row {
				if c == '#' {
					data = append(data, 2)
				} else {
					data = append(data, 1)
				}
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
		put("maps/"+m.Name+".tmj", map[string]any{
			"width": w, "height": h, "tilewidth": 16, "tileheight": 16,
			"tilesets": []any{map[string]any{"firstgid": 1, "source": "../tilesets/test.tsj"}},
			"layers": []any{
				map[string]any{"name": "ground", "type": "tilelayer", "width": w, "height": h, "data": data},
				map[string]any{"name": "objects", "type": "objectgroup", "objects": objs},
			},
		})
		if m.Placed {
			placed = append(placed, map[string]any{"fileName": m.Name + ".tmj",
				"x": m.Origin[0] * 16, "y": m.Origin[1] * 16, "width": w * 16, "height": h * 16})
		}
	}
	if len(placed) > 0 {
		put("maps/overworld.world", map[string]any{"type": "world", "maps": placed})
	}
	return fsys
}

// Load builds and loads a world, failing the test on error.
func Load(t testing.TB, maps ...Map) *world.World {
	t.Helper()
	w, err := world.Load(FS(maps...), "maps")
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	return w
}
