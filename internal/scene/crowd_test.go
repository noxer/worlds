package scene

import (
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// newCrowdHarness is newHarness with the world's settlements, so the
// crowd turns up.
func newCrowdHarness(t *testing.T) (*harness, *infra.Infra) {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	party := entity.NewParty(&entity.Actor{Name: "leader"})
	e, err := NewExplore(ExploreConfig{World: c.World, Sheets: gfx.NewSheets(assets.FS), Dialogs: c.Dialogs,
		Party: party, Start: c.Start, Infra: c.Infra})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, w: c.World, in: &input.State{}, e: e}
	h.stack = NewStack(h.in, e)
	h.stack.SetSize(320, 180)
	return h, c.Infra
}

func (h *harness) ambient() []*NPC {
	var out []*NPC
	for _, n := range h.e.npcs {
		if n.ambient {
			out = append(out, n)
		}
	}
	return out
}

// cityPlaza returns a free tile on the plaza of a real city.
func cityPlaza(t *testing.T, in *infra.Infra) world.Pos {
	t.Helper()
	for _, s := range in.Near(0, 193770, 140000, infra.City) {
		return world.Pos{Map: world.Main, X: s.X, Y: s.Y + 10}
	}
	t.Fatal("no city")
	return world.Pos{}
}

func TestCrowdInTheCity(t *testing.T) {
	h, in := newCrowdHarness(t)
	plaza := cityPlaza(t, in)
	h.e.party.PlaceAll(plaza, world.Down)
	seen := map[*NPC]bool{}
	for tick := 0; tick < 1800; tick++ {
		h.tick(input.Frame{})
		taken := map[world.Pos]bool{}
		for _, n := range h.ambient() {
			if !seen[n] {
				seen[n] = true
				dx, dy, _ := h.w.Delta(plaza, n.Pos)
				if d := max(geom.Abs(dx), geom.Abs(dy)); d < crowdNear {
					t.Fatalf("someone turned up %d tiles away, in sight", d)
				}
			}
			if taken[n.Pos] || h.e.party.Reserves(n.Pos) {
				t.Fatalf("two at %v", n.Pos)
			}
			taken[n.Pos] = true
			if _, walk, urban := in.Footfall(n.Pos.X, n.Pos.Y); !walk || !urban {
				t.Fatalf("a pedestrian at %v, off the sidewalks", n.Pos)
			}
			if n.Sprite.Walk == nil {
				t.Fatal("city people should be animated")
			}
		}
	}
	if n := len(h.ambient()); n < 4 {
		t.Errorf("only %d people round the station plaza", n)
	}
	// Talking to one.
	n := h.ambient()[0]
	for _, d := range world.Dirs {
		if q, ok := h.w.Neighbor(n.Pos, d); ok && !h.w.Solid(q) && h.e.npcAt(q) == nil {
			n.wait = 1000 // stand still for it
			face, _ := h.w.DirBetween(q, n.Pos)
			h.e.party.PlaceAll(q, face)
			h.press(input.Confirm)
			if _, ok := h.top().(*DialogBox); !ok {
				t.Fatal("talking to a pedestrian should open a dialog")
			}
			break
		}
	}
	// Leaving: out in the countryside, nobody follows.
	far, _ := h.w.Offset(plaza, 0, 3000)
	h.stack.scenes = h.stack.scenes[:1]
	h.e.party.PlaceAll(far, world.Down)
	h.tick(input.Frame{})
	h.tick(input.Frame{})
	if n := len(h.ambient()); n > 0 {
		t.Errorf("%d people stayed behind with the party", n)
	}
}

func TestVillagersOnTheSquare(t *testing.T) {
	h, in := newCrowdHarness(t)
	town, _ := h.w.PatchOrigin("town")
	sites := in.SitesIn(town.X-6000, town.Y-6000, 12000, 12000)
	if len(sites) == 0 {
		t.Fatal("no village")
	}
	sq := world.Pos{Map: world.Main, X: sites[0].S.X - 2, Y: sites[0].S.Y + 2}
	h.e.party.PlaceAll(sq, world.Down)
	for range 3600 {
		h.tick(input.Frame{})
	}
	people := h.ambient()
	if len(people) == 0 {
		t.Fatal("nobody about in the village")
	}
	for _, n := range people {
		if n.Sprite.Sheet != "tinydungeon" {
			t.Errorf("a villager drawn from %s", n.Sprite.Sheet)
		}
	}
}

// TestDebugTravel: with the overlay on, T goes to the nearest village,
// town and city in turn, then home.
func TestDebugTravel(t *testing.T) {
	h, in := newCrowdHarness(t)
	home := h.leader().Pos
	h.press(input.Debug)
	for k, tier := range []infra.Tier{infra.Village, infra.Town, infra.City, 0} {
		h.press(input.DebugTravel)
		h.runUntil("the fade", 200, func() bool { return h.e.fade == 0 })
		p := h.leader().Pos
		if k == 3 {
			if p != home {
				t.Errorf("the fourth jump should go home, went to %v", p)
			}
			break
		}
		near := in.Near(p.X, p.Y, 200, tier)
		ok := false
		for _, s := range near {
			ok = ok || s.ID.Tier == tier
		}
		if !ok {
			t.Errorf("jump %d: %v is not in a %s", k, p, tier)
		}
		if h.w.Solid(p) {
			t.Errorf("jump %d landed on a solid tile %v", k, p)
		}
	}
}
