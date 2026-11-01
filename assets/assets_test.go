package assets_test

import (
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

func load(t *testing.T) *content.Content {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// reachable follows plans leg by leg, applying warps, like the game does.
func reachable(w *world.World, p *nav.Planner, from, goal world.Pos) bool {
	cur := from
	for legs := 0; legs < 10; legs++ {
		if cur == goal {
			return true
		}
		moves, ok := p.Plan(cur, goal, nil)
		if !ok || len(moves) == 0 {
			return false
		}
		last := moves[len(moves)-1]
		cur = last.To
		if last.Warp != nil {
			cur = last.Warp.To
		}
	}
	return cur == goal
}

func TestDialogReferences(t *testing.T) {
	c := load(t)
	for _, o := range c.World.Objects() {
		if d := o.Props.String("dialog"); d != "" && c.Dialogs[d] == nil {
			t.Errorf("object %q at %v: unknown dialog %q", o.Name, o.Pos, d)
		}
	}
}

// TestReachability checks that every detail map, every patch and the
// generated land beyond them can be reached from the start.
func TestReachability(t *testing.T) {
	c := load(t)
	w := c.World
	p := nav.NewPlanner(w)

	var goals []world.Pos
	for _, id := range w.Templates() {
		goals = append(goals, firstWalkable(t, w, world.Pos{Map: world.Detail(id)}, 64))
	}
	for _, name := range []world.MapID{"town", "meadow"} {
		o, ok := w.PatchOrigin(name)
		if !ok {
			t.Fatalf("patch %s missing", name)
		}
		goals = append(goals, firstWalkable(t, w, o, 20))
	}
	// Generated land east of the meadow's exit.
	meadow, _ := w.PatchOrigin("meadow")
	east, _ := w.Offset(meadow, 30, 9)
	goals = append(goals, firstWalkable(t, w, east, 10))

	for _, g := range goals {
		if !reachable(w, p, c.Start.Pos, g) {
			t.Errorf("%v is not reachable from the start %v", g, c.Start.Pos)
		}
	}
}

func firstWalkable(t *testing.T, w *world.World, origin world.Pos, n int64) world.Pos {
	t.Helper()
	for y := int64(0); y < n; y++ {
		for x := int64(0); x < n; x++ {
			if p, ok := w.Offset(origin, x, y); ok && !w.Solid(p) && w.WarpAt(p) == nil {
				return p
			}
		}
	}
	t.Fatalf("no walkable tile near %v", origin)
	return world.Pos{}
}
