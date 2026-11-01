package nav_test

import (
	"testing"

	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

var pos = worldtest.Pos

// A 64x64 wall world with one patch straddling the east-west seam:
//
//	x: 58      63|0    4
//	   ######
//	   #..D.#    ....#      D = door into "house" at (61,1)
//	   #.....    ....#
//	   #.....    #####
//	   ######
//
// The house has a back door to a cellar, whose ladder leads up to (3,1). So
// the east part is reachable over the seam or via house and cellar.
func testWorld(t *testing.T) *world.World {
	return worldtest.Load(t, worldtest.Spec{
		Patches: []worldtest.Map{{
			Name: "field", X: -6, Y: 0,
			Rows: []string{
				"######     ",
				"#....#....#",
				"#.........#",
				"#.....#####",
				"######     ",
			},
			Spawns: map[string][2]int{"door": {3, 2}, "ladder": {9, 1}},
			Warps:  []worldtest.Warp{{X: 3, Y: 1, ToMap: "house", ToSpawn: "in"}},
		}},
		Details: []worldtest.Map{
			{
				Name: "house",
				Rows: []string{
					"#####",
					"#...#",
					"#...#",
				},
				Spawns: map[string][2]int{"in": {1, 1}},
				Warps: []worldtest.Warp{
					{X: 1, Y: 2, ToMap: "main", ToSpawn: "door"},
					{X: 3, Y: 2, ToMap: "cellar", ToSpawn: "down"},
				},
			},
			{
				Name: "cellar",
				Rows: []string{
					"####",
					"#..#",
					"####",
				},
				Spawns: map[string][2]int{"down": {1, 1}},
				Warps:  []worldtest.Warp{{X: 2, Y: 1, ToMap: "main", ToSpawn: "ladder"}},
			},
		},
	})
}

// follow executes plans leg by leg like the game does, returning the
// positions visited.
func follow(t *testing.T, w *world.World, p *nav.Planner, from, goal world.Pos, blocked nav.Blocked) []world.Pos {
	t.Helper()
	var visited []world.Pos
	cur := from
	for legs := 0; cur != goal; legs++ {
		if legs > 10 {
			t.Fatalf("too many legs")
		}
		moves, ok := p.Plan(cur, goal, blocked)
		if !ok {
			t.Fatalf("no path from %v to %v", cur, goal)
		}
		for i, m := range moves {
			next, ok := w.Neighbor(cur, m.Dir)
			if !ok || next != m.To {
				t.Fatalf("move %d from %v %v: Neighbor = %v,%v want %v", i, cur, m.Dir, next, ok, m.To)
			}
			if w.Solid(next) {
				t.Fatalf("path walks into solid %v", next)
			}
			if wp := w.WarpAt(next); wp != nil {
				if m.Warp != wp || i != len(moves)-1 {
					t.Fatalf("path steps onto unexpected warp at %v", next)
				}
				next = wp.To
			}
			cur = next
			visited = append(visited, cur)
		}
	}
	return visited
}

func TestPathAvoidsWarpTiles(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	moves, ok := p.Plan(pos("main", 59, 1), pos("main", 62, 1), nil)
	if !ok {
		t.Fatal("no path")
	}
	// The direct way leads over the door at (61,1); the path must go around.
	if len(moves) != 5 {
		t.Fatalf("got %d moves, want 5: %+v", len(moves), moves)
	}
}

func TestPathAcrossSeam(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	visited := follow(t, w, p, pos("main", 59, 2), pos("main", 2, 2), nil)
	if len(visited) != 7 {
		t.Fatalf("visited %d tiles, want 7: %v", len(visited), visited)
	}
}

func TestPathThroughWarps(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	// From inside the house to the east part: out the front door and over
	// the seam, or down the cellar and up the ladder.
	from, goal := pos("house", 2, 1), pos("main", 2, 1)

	p.WarpCost = 50
	if visited := follow(t, w, p, from, goal, nil); !contains(visited, pos("main", 61, 2)) {
		t.Fatalf("expected the walk over the seam, got %v", visited)
	}
	p.WarpCost = 0
	if visited := follow(t, w, p, from, goal, nil); !contains(visited, pos("cellar", 1, 1)) {
		t.Fatalf("expected the cellar route, got %v", visited)
	}
}

func TestPathIntoBuilding(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	visited := follow(t, w, p, pos("main", 3, 2), pos("house", 3, 1), nil)
	if !contains(visited, pos("house", 1, 1)) {
		t.Fatalf("expected to enter through the front door: %v", visited)
	}
}

func TestDynamicBlockers(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	npc := pos("main", 60, 2)
	blocked := func(q world.Pos) bool { return q == npc }
	visited := follow(t, w, p, pos("main", 59, 2), pos("main", 63, 2), blocked)
	if contains(visited, npc) {
		t.Fatalf("walked through the NPC: %v", visited)
	}
	if len(visited) != 6 {
		t.Fatalf("expected a detour of 6 steps, got %v", visited)
	}
}

func TestUnreachableAndInvalidation(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	if _, ok := p.Plan(pos("main", 59, 1), pos("main", 58, 0), nil); ok {
		t.Fatal("found a path into a wall")
	}
	from, goal := pos("main", 59, 2), pos("main", 1, 1)
	if _, ok := p.Plan(from, goal, nil); !ok {
		t.Fatal("expected path")
	}
	// Close the seam crossing and the house door: east becomes unreachable.
	w.SetBlocked(pos("main", 63, 2), true)
	w.SetBlocked(pos("main", 61, 1), true)
	if moves, ok := p.Plan(from, goal, nil); ok {
		t.Fatalf("expected no path after blocking, got %+v", moves)
	}
}

func TestRangeOnOpenGround(t *testing.T) {
	w := worldtest.Load(t, worldtest.Spec{Width: 512, Height: 512, Fill: '.'})
	p := nav.NewPlanner(w)
	moves, ok := p.Plan(pos("main", 500, 7), pos("main", 100, 7), nil)
	if !ok || len(moves) != 112 {
		t.Fatalf("across the seam: ok=%v len=%d, want 112", ok, len(moves))
	}
	if _, ok := p.Plan(pos("main", 0, 0), pos("main", 200, 200), nil); ok {
		t.Fatal("goal beyond MaxRange should not be planned locally")
	}
}

func contains(ps []world.Pos, q world.Pos) bool {
	for _, p := range ps {
		if p == q {
			return true
		}
	}
	return false
}
