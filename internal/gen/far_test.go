package gen_test

import (
	"testing"
	"time"

	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

// TestFarTrips plans long trips in the real world, on foot and by bike, and
// follows them stretch by stretch, checking every step is legal.
func TestFarTrips(t *testing.T) {
	w := loadWorld(t)
	meadow, _ := w.PatchOrigin("meadow")
	start, _ := w.Offset(meadow, 23, 9) // the meadow's east exit
	// Goals: land tiles at different distances and directions, found with
	// the preview and confirmed on the real tiles.
	var goals []world.Pos
	for _, d := range [][2]int64{{800, 0}, {0, -1500}, {-1500, 1500}, {2000, -1000}} {
		found := false
		for r := int64(0); r < 200 && !found; r += 7 {
			c, _ := w.Offset(start, d[0]+r, d[1]+r/2)
			if tr, _ := w.Preview(c.X, c.Y); tr != world.Land {
				continue
			}
			for i := int64(0); i < 32 && !found; i++ {
				q, _ := w.Offset(c, i, i%5)
				if _, ok := nav.StepTicks(w, q, nav.Ride); ok {
					goals = append(goals, q)
					found = true
				}
			}
		}
	}
	if len(goals) < 3 {
		t.Fatalf("found only %d goals", len(goals))
	}
	for _, mode := range []nav.Mode{nav.Foot, nav.Ride} {
		for _, goal := range goals {
			p := nav.NewPlanner(w)
			cur, steps, legs := start, 0, 0
			var planning time.Duration
			for cur != goal {
				if legs++; legs > 400 {
					t.Fatalf("%v to %v: too many legs", mode, goal)
				}
				t0 := time.Now()
				moves, ok := p.Plan(cur, goal, mode, nil)
				planning += time.Since(t0)
				if !ok || len(moves) == 0 {
					t.Errorf("%v: no route from %v to %v after %d steps", mode, cur, goal, steps)
					break
				}
				for _, mv := range moves {
					next, ok := w.Neighbor(cur, mv.Dir)
					if !ok || next != mv.To {
						t.Fatalf("broken move from %v", cur)
					}
					if _, ok := nav.StepTicks(w, next, mode); !ok {
						t.Fatalf("%v: illegal step onto %v (%v)", mode, next, w.Terrain(next))
					}
					cur = next
					steps++
				}
			}
			dx, dy, _ := w.Delta(start, goal)
			t.Logf("%v to %v: %d steps for distance %d, %d stretches, planning %v", mode, goal, steps, abs(dx)+abs(dy), legs, planning)
		}
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
