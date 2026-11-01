package nav_test

import (
	"testing"

	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

// Layout (region coordinates):
//
//	west (0,0)          east (6,1)
//	######
//	#..W.#              ....#
//	#.....  <-edge->    ....#
//	#.....              #####
//	######
//
// W is the door into "house". The house has a back door to "cellar", which
// has a ladder up to "east". So east can be reached via the edge or via the
// house and cellar.
func testWorld(t *testing.T) *world.World {
	return worldtest.Load(t,
		worldtest.Map{
			Name: "west", Placed: true,
			Rows: []string{
				"######",
				"#....#",
				"#.....",
				"#.....",
				"######",
			},
			Spawns: map[string][2]int{"door": {3, 2}},
			Warps:  []worldtest.Warp{{X: 3, Y: 1, ToMap: "house", ToSpawn: "in"}},
		},
		worldtest.Map{
			Name: "east", Placed: true, Origin: [2]int{6, 1},
			Rows: []string{
				"....#",
				"....#",
				"#####",
			},
			Spawns: map[string][2]int{"ladder": {3, 0}},
		},
		worldtest.Map{
			Name: "house",
			Rows: []string{
				"#####",
				"#...#",
				"#...#",
			},
			Spawns: map[string][2]int{"in": {1, 1}, "back": {3, 1}},
			Warps: []worldtest.Warp{
				{X: 1, Y: 2, ToMap: "west", ToSpawn: "door"},
				{X: 3, Y: 2, ToMap: "cellar", ToSpawn: "down"},
			},
		},
		worldtest.Map{
			Name: "cellar",
			Rows: []string{
				"####",
				"#..#",
				"####",
			},
			Spawns: map[string][2]int{"down": {1, 1}},
			Warps:  []worldtest.Warp{{X: 2, Y: 1, ToMap: "east", ToSpawn: "ladder"}},
		},
	)
}

// follow executes plans leg by leg like the game does, returning the
// positions visited.
func follow(t *testing.T, w *world.World, p *nav.Planner, from, goal world.Pos, blocked nav.Blocked) []world.Pos {
	t.Helper()
	var visited []world.Pos
	pos := from
	for legs := 0; pos != goal; legs++ {
		if legs > 10 {
			t.Fatalf("too many legs")
		}
		moves, ok := p.Plan(pos, goal, blocked)
		if !ok {
			t.Fatalf("no path from %v to %v", pos, goal)
		}
		for i, m := range moves {
			next, ok := w.Neighbor(pos, m.Dir)
			if !ok || next != m.To {
				t.Fatalf("move %d from %v %v: Neighbor = %v,%v want %v", i, pos, m.Dir, next, ok, m.To)
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
			pos = next
			visited = append(visited, pos)
		}
	}
	return visited
}

func TestPathWithinMap(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	moves, ok := p.Plan(pos("west", 1, 1), pos("west", 4, 1), nil)
	if !ok {
		t.Fatal("no path")
	}
	// Has to go around the wall at (3,1) without stepping onto the door.
	if len(moves) != 5 {
		t.Fatalf("got %d moves, want 5: %+v", len(moves), moves)
	}
}

func TestPathAcrossEdge(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	goal := pos("east", 2, 1)
	visited := follow(t, w, p, pos("west", 1, 2), goal, nil)
	if len(visited) != 7 {
		t.Fatalf("visited %d tiles, want 7: %v", len(visited), visited)
	}
}

func TestPathThroughWarps(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	// From inside the house to east: exit via the front door and walk over the
	// edge, or take the cellar ladder. Both work; with a high warp cost the
	// walk wins, with no warp cost the ladder wins.
	from, goal := pos("house", 2, 1), pos("east", 2, 0)

	p.WarpCost = 50
	visited := follow(t, w, p, from, goal, nil)
	if !contains(visited, pos("west", 3, 2)) {
		t.Fatalf("expected the walk via west, got %v", visited)
	}

	p.WarpCost = 0
	visited = follow(t, w, p, from, goal, nil)
	if !contains(visited, pos("cellar", 1, 1)) {
		t.Fatalf("expected the cellar route, got %v", visited)
	}
}

func TestPathIntoBuilding(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	visited := follow(t, w, p, pos("east", 3, 1), pos("house", 3, 1), nil)
	if !contains(visited, pos("house", 1, 1)) {
		t.Fatalf("expected to enter through the front door: %v", visited)
	}
}

func TestDynamicBlockers(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	npc := pos("west", 2, 2)
	blocked := func(q world.Pos) bool { return q == npc }
	visited := follow(t, w, p, pos("west", 1, 2), pos("west", 5, 2), blocked)
	if contains(visited, npc) {
		t.Fatalf("walked through the NPC: %v", visited)
	}
	if len(visited) != 6 {
		t.Fatalf("expected detour of 6 steps, got %v", visited)
	}
}

func TestUnreachableAndInvalidation(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	if _, ok := p.Plan(pos("west", 1, 1), pos("west", 0, 0), nil); ok {
		t.Fatal("found a path into a wall")
	}
	from, goal := pos("west", 1, 2), pos("east", 1, 0)
	if _, ok := p.Plan(from, goal, nil); !ok {
		t.Fatal("expected path")
	}
	// Close the edge crossing and the house door: east must become unreachable.
	w.SetBlocked(pos("west", 5, 2), true)
	w.SetBlocked(pos("west", 3, 1), true)
	if moves, ok := p.Plan(from, goal, nil); ok {
		t.Fatalf("expected no path after blocking, got %+v", moves)
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

func pos(m string, x, y int) world.Pos { return world.Pos{Map: world.MapID(m), X: x, Y: y} }
