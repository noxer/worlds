package entity_test

import (
	"testing"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/world"
)

// TestWalkCycleStands: walking, the standing frame comes between the
// stepping frames, so a cycle is stand, left, stand, right.
func TestWalkCycleStands(t *testing.T) {
	frames := [4][]int{}
	for d := range frames {
		frames[d] = []int{10, 11, 12}
	}
	a := &entity.Actor{Sprite: entity.Sprite{Walk: &frames}, Facing: world.Down}
	var seen []int
	for step := 0; step < 2; step++ {
		a.BeginStep(world.Down, world.Pos{Map: world.Main, Y: int64(step + 1)}, 8)
		for tick := 0; tick < 8; tick++ {
			f, _ := a.Frame()
			if len(seen) == 0 || seen[len(seen)-1] != f {
				seen = append(seen, f)
			}
			a.Update()
		}
	}
	if f, _ := a.Frame(); f != 10 {
		t.Errorf("standing still shows frame %d, want 10", f)
	}
	// Either foot may lead.
	if len(seen) != 4 || seen[1] != 10 || seen[3] != 10 || seen[0] == seen[2] ||
		seen[0] < 11 || seen[0] > 12 || seen[2] < 11 || seen[2] > 12 {
		t.Fatalf("frames %v, want a stepping frame, 10, the other one, 10", seen)
	}
}
