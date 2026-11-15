package transit_test

import (
	"math"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/transit"
)

// TestFerryRuns: a ferry shuttles between two pier tips, lies in line with
// the pier while moored, and its two halves always touch.
func TestFerryRuns(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	in, w := c.Infra, c.World
	for _, s := range in.Near(0, 193770, 400000, infra.Town) {
		for _, e := range in.Edges(infra.SeaLane, s) {
			l := transit.NewFerry(in, e, w.W, w.H, w.Seed)
			if l == nil {
				continue
			}
			pa, _ := in.Harbour(e.A)
			pb, _ := in.Harbour(e.B)
			at := func(st transit.State, p infra.Pier) {
				t.Helper()
				cars := l.CarsAt(st)
				stern := cars[1]
				if int64(math.Round(stern.X)) != p.Tip.X || int64(math.Round(stern.Y)) != p.Tip.Y {
					t.Errorf("moored stern at %.1f,%.1f, not at the tip %v", stern.X, stern.Y, p.Tip)
				}
				bx, by := cars[0].X-stern.X, cars[0].Y-stern.Y
				if math.Abs(bx-float64(p.DX)) > 1e-6 || math.Abs(by-float64(p.DY)) > 1e-6 {
					t.Errorf("moored bow %.2f,%.2f from the stern, not out along the pier %d,%d", bx, by, p.DX, p.DY)
				}
			}
			arrive, _ := l.NextStop(0, true)
			at(l.At(float64(arrive)), pa)
			arrive, _ = l.NextStop(0, false)
			at(l.At(float64(arrive)), pb)
			for tick := int64(0); tick < l.Cycle; tick += 37 {
				cars := l.CarsAt(l.At(float64(tick)))
				dx, dy := math.Abs(cars[0].X-cars[1].X), math.Abs(cars[0].Y-cars[1].Y)
				if math.Abs(dx+dy-1) > 1e-6 && math.Abs(dx+dy-1) < float64(w.W)/2 {
					t.Fatalf("tick %d: the halves are %.2f,%.2f apart", tick, dx, dy)
				}
			}
			return
		}
	}
	t.Fatal("no ferry")
}
