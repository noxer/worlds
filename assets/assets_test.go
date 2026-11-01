package assets_test

import (
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

// TestContent validates the shipped maps and dialogs and checks that every
// map is reachable from the start, and that every referenced dialog exists.
func TestContent(t *testing.T) {
	w, err := world.Load(assets.FS, "maps")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := dialog.Load(assets.FS, "dialog")
	if err != nil {
		t.Fatal(err)
	}
	start, ok := w.Map("town").Spawn("start")
	if !ok {
		t.Fatal("town has no start spawn")
	}
	p := nav.NewPlanner(w)
	for id, m := range w.Maps {
		for _, o := range m.Objects {
			if d := o.Props.String("dialog"); d != "" && lib[d] == nil {
				t.Errorf("map %s object %q: unknown dialog %q", id, o.Name, d)
			}
		}
		// Find any walkable tile and check it can be reached from the start.
		reached := false
		for y := 0; y < m.H && !reached; y++ {
			for x := 0; x < m.W && !reached; x++ {
				goal := world.Pos{Map: id, X: x, Y: y}
				if w.Solid(goal) || w.WarpAt(goal) != nil {
					continue
				}
				_, reached = p.Plan(start.Pos, goal, nil)
			}
		}
		if !reached {
			t.Errorf("map %s is not reachable from the start", id)
		}
	}
}
