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
		moves, ok := p.Plan(cur, goal, nav.Foot, nil)
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
		if w.TemplateProps(id).String("vehicle") != "" {
			continue // vehicle interiors are reached by boarding
		}
		goals = append(goals, firstWalkable(t, w, world.Pos{Map: world.Detail(id)}, 64))
	}
	for _, name := range []world.MapID{"town", "meadow", "isle"} { // the isle: by swimming
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

// TestMultiTileObjects checks that multi-tile objects are complete: every
// tile with an "above" property in its tileset has that tile on the
// overhead layer above it, and upper parts never float. Patches and detail
// maps are already checked when they load; this scans the generated land
// around the patches and elsewhere in the world, including the seams.
func TestMultiTileObjects(t *testing.T) {
	c := load(t)
	w := c.World
	ts := w.Tilesets()
	check := func(origin world.Pos, wdt, hgt int64) {
		for y := int64(0); y < hgt; y++ {
			for x := int64(0); x < wdt; x++ {
				p, _ := w.Offset(origin, x, y)
				above, _ := w.Offset(p, 0, -1)
				below, _ := w.Offset(p, 0, 1)
				for _, l := range []int{world.LayerGround, world.LayerDeco} {
					if top, ok := ts.Above(w.Tile(p, l)); ok && w.Tile(above, world.LayerOverhead) != top {
						t.Errorf("%v: lower part without its upper part above", p)
					}
				}
				if ov := w.Tile(p, world.LayerOverhead); ts.IsTop(ov) {
					t1, ok1 := ts.Above(w.Tile(below, world.LayerGround))
					t2, ok2 := ts.Above(w.Tile(below, world.LayerDeco))
					if !(ok1 && t1 == ov) && !(ok2 && t2 == ov) {
						t.Errorf("%v: upper part without its lower part below", p)
					}
				}
			}
		}
	}
	town, _ := w.PatchOrigin("town")
	start, _ := w.Offset(town, -10, -10)
	check(start, 30+24+30, 20+8+20) // town, meadow and surroundings
	for _, ch := range [][2]int64{{0, 0}, {12345, 6789}, {32767, 16383}} {
		check(world.Pos{Map: world.Main, X: ch[0] * world.ChunkSize, Y: ch[1] * world.ChunkSize}, 2*world.ChunkSize, 2*world.ChunkSize)
	}
	// Generated cities (street trees, lamps), villages and their surroundings.
	check(world.Pos{Map: world.Main, X: 40027 - 150, Y: 90965 - 150}, 300, 300) // a city's centre
	check(world.Pos{Map: world.Main, X: 2802 - 80, Y: 193995 - 80}, 160, 160)   // a village
}

// TestAnimationFramesMatch checks that every frame of an animated tile has
// the same collision and terrain as the tile itself, so placing a frame
// tile directly in a map behaves the same as placing the animated tile.
func TestAnimationFramesMatch(t *testing.T) {
	c := load(t)
	n := 0
	for _, set := range c.World.Tilesets().All() {
		for i := 0; i < set.TileCount; i++ {
			a := set.Animation(i)
			if a == nil {
				continue
			}
			n++
			for _, f := range a.Frames {
				if set.Solid(f) != set.Solid(i) || set.Terrain(f) != set.Terrain(i) {
					t.Errorf("%s tile %d: frame %d has different solid/terrain", set.Name, i, f)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("expected animated tiles (water)")
	}
}
