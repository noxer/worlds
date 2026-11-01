package world_test

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

// west and east are edge-connected; east is shifted down by 2 tiles so only
// part of their shared edge connects. house is reached through a warp.
func testMaps() []worldtest.Map {
	return []worldtest.Map{
		{
			Name: "west", Placed: true, Origin: [2]int{0, 0},
			Rows: []string{
				"#####",
				"#...#",
				"#....",
				"#....",
				"#####",
			},
			Spawns: map[string][2]int{"door": {2, 2}},
			Warps:  []worldtest.Warp{{X: 2, Y: 1, ToMap: "house", ToSpawn: "in"}},
		},
		{
			Name: "east", Placed: true, Origin: [2]int{5, 2},
			Rows: []string{
				"....#",
				"....#",
				"#####",
			},
		},
		{
			Name: "house",
			Rows: []string{
				"###",
				"#.#",
				"#.#",
			},
			Spawns: map[string][2]int{"in": {1, 1}},
			Warps:  []worldtest.Warp{{X: 1, Y: 2, ToMap: "west", ToSpawn: "door"}},
		},
	}
}

func TestRegions(t *testing.T) {
	w := worldtest.Load(t, testMaps()...)
	if got := w.Map("west").Region; got != w.Map("east").Region {
		t.Fatalf("west and east should share a region")
	}
	if w.Map("house").Region == w.Map("west").Region {
		t.Fatalf("house should be its own region")
	}
	r := w.Map("west").Region
	if r.W != 10 || r.H != 5 {
		t.Fatalf("region bounds = %dx%d, want 10x5", r.W, r.H)
	}
}

func TestNeighborCrossesEdges(t *testing.T) {
	w := worldtest.Load(t, testMaps()...)
	cases := []struct {
		from world.Pos
		dir  world.Dir
		want world.Pos
		ok   bool
	}{
		{pos("west", 3, 2), world.Right, pos("west", 4, 2), true},
		{pos("west", 4, 2), world.Right, pos("east", 0, 0), true},
		{pos("west", 4, 3), world.Right, pos("east", 0, 1), true},
		{pos("east", 0, 1), world.Left, pos("west", 4, 3), true},
		{pos("west", 4, 1), world.Right, world.Pos{}, false}, // void next to east map
		{pos("house", 1, 1), world.Up, pos("house", 1, 0), true},
		{pos("house", 1, 0), world.Up, world.Pos{}, false},
	}
	for _, c := range cases {
		got, ok := w.Neighbor(c.from, c.dir)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Neighbor(%v, %v) = %v, %v; want %v, %v", c.from, c.dir, got, ok, c.want, c.ok)
		}
	}
	if d, ok := w.DirBetween(pos("west", 4, 2), pos("east", 0, 0)); !ok || d != world.Right {
		t.Errorf("DirBetween across edge = %v, %v", d, ok)
	}
}

func TestWarpsAndSolid(t *testing.T) {
	w := worldtest.Load(t, testMaps()...)
	wp := w.WarpAt(pos("west", 2, 1))
	if wp == nil || wp.To != (pos("house", 1, 1)) {
		t.Fatalf("warp = %+v", wp)
	}
	if !w.Solid(pos("west", 0, 0)) || w.Solid(pos("west", 1, 1)) {
		t.Fatalf("unexpected collision")
	}
	v := w.Version()
	p := pos("west", 1, 1)
	w.SetBlocked(p, true)
	if !w.Solid(p) || w.Version() == v {
		t.Fatalf("SetBlocked did not block or bump version")
	}
	w.SetBlocked(p, false)
	if w.Solid(p) {
		t.Fatalf("SetBlocked(false) did not unblock")
	}
}

func TestValidation(t *testing.T) {
	maps := testMaps()
	maps[0].Warps = append(maps[0].Warps, worldtest.Warp{X: 1, Y: 1, ToMap: "house", ToSpawn: "nope"})
	_, err := world.Load(worldtest.FS(maps...), "maps")
	if err == nil || !strings.Contains(err.Error(), "unknown spawn") {
		t.Fatalf("expected unknown spawn error, got %v", err)
	}

	maps = testMaps()
	maps[1].Origin = [2]int{4, 2} // overlaps west
	_, err = world.Load(worldtest.FS(maps...), "maps")
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap error, got %v", err)
	}
}

func pos(m string, x, y int) world.Pos { return world.Pos{Map: world.MapID(m), X: x, Y: y} }
