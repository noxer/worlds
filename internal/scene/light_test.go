package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

func TestAmbientFollowsTimeOutdoors(t *testing.T) {
	h := newHarness(t)
	h.setTime(12, 0)
	if a := h.e.ambient(); a != sim.Daylight {
		t.Errorf("noon in town: %v, want daylight", a)
	}
	h.setTime(23, 0)
	if a := h.e.ambient(); a != sim.Night {
		t.Errorf("23:00 in town: %v, want night", a)
	}
	smithy, _ := h.w.Spawn(world.MapID("smithy"), "entrance")
	h.e.party.PlaceAll(smithy.Pos, world.Up)
	if a := h.e.ambient(); a != sim.Daylight {
		t.Errorf("23:00 in the smithy: %v, want it lit", a)
	}
}

func TestLampsInView(t *testing.T) {
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	lantern, _ := h.w.Offset(town, 9, 7) // the top of the lamp post at (9,8)
	// A 320x180 view centred on the lantern.
	camX, camY := lantern.X*tile+tile/2-160, lantern.Y*tile+tile/2-90
	spots := h.e.lightsInView(world.Main, camX, camY, 320, 180)
	var lamp, windows int
	for _, s := range spots {
		switch s.Radius {
		case 3.5:
			lamp++
			if s.x == 160 && s.y == 90 {
				lamp += 100 // the one in the middle
			}
		case 1.25:
			windows++
		}
	}
	if lamp < 100 {
		t.Errorf("the lantern in the middle of the view is missing: %+v", spots)
	}
	if windows == 0 {
		t.Error("house windows should give off light")
	}
	// Lights just outside the view still shine into it.
	camX = lantern.X*tile + 2*tile // the lantern is now two tiles left of the view
	found := false
	for _, s := range h.e.lightsInView(world.Main, camX, camY, 320, 180) {
		found = found || (s.Radius == 3.5 && s.x == tile/2-2*tile)
	}
	if !found {
		t.Error("a lamp just outside the view should be included")
	}
}

func TestGlowBands(t *testing.T) {
	if glowAt(0) != 1 || glowAt(1) != 0 || glowAt(2) != 0 {
		t.Error("glow should be full in the middle and nothing at the radius")
	}
	prev := 1.0
	for d := 0.0; d < 1; d += 0.01 {
		g := glowAt(d)
		if g > prev {
			t.Fatalf("glow grows at %v", d)
		}
		if g*lightBands != float64(int(g*lightBands)) {
			t.Fatalf("glow %v at %v is not a band", g, d)
		}
		prev = g
	}
}
