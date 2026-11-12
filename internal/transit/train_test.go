package transit_test

import (
	"math"
	"testing"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

func realLine(t *testing.T) (*content.Content, *transit.Line) {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Infra.Near(0, 193770, 140000, infra.City) {
		for _, e := range c.Infra.Edges(infra.Rail, s) {
			if l := transit.NewLine(c.Infra, e, c.World.W, c.World.H, c.World.Seed); l != nil {
				return c, l
			}
		}
	}
	t.Fatal("no rail line")
	return nil, nil
}

func TestTrainRuns(t *testing.T) {
	c, l := realLine(t)
	w := c.World
	if l.Travel <= 0 || l.Travel > 4*60*sim.TicksPerMinute {
		t.Fatalf("a run takes %d ticks for %.0f tiles", l.Travel, l.Length)
	}
	rails := map[world.Tile]bool{}
	for m := range 16 {
		tile, _ := w.Tilesets().Named("worlds", "rail "+itoa(m))
		rails[tile] = true
	}
	prev := l.At(0)
	stops := 0
	for tick := int64(1); tick < l.Cycle+10; tick++ {
		st := l.At(float64(tick))
		if d := math.Abs(st.Front - prev.Front); d > 34 && st.Moving == prev.Moving {
			t.Fatalf("tick %d: the train jumped %.1f tiles", tick, d)
		}
		if !st.Moving {
			want := float64(l.Cars - 1)
			if !st.AtA {
				want = l.Length - float64(l.Cars-1)
			}
			if st.Front != want {
				t.Fatalf("tick %d: waiting at %.1f, not at the platform (%.1f)", tick, st.Front, want)
			}
			if prev.Moving {
				stops++
			}
		}
		if tick%97 == 0 {
			cars := l.CarsAt(st)
			for k, car := range cars {
				p := world.Pos{Map: world.Main, X: int64(math.Round(car.X)), Y: int64(math.Round(car.Y))}
				if tick%(97*20) == 0 && !rails[w.Tile(p, world.LayerDeco)] {
					t.Errorf("tick %d: car %d at %v is off the track", tick, k, p)
				}
				if k > 0 {
					a, b := cars[k-1], car
					dx, dy := math.Abs(a.X-b.X), math.Abs(a.Y-b.Y)
					dx, dy = min(dx, float64(w.W)-dx), min(dy, float64(w.H)-dy)
					if math.Abs(dx+dy-1) > 1e-6 && dx+dy > 1 {
						t.Fatalf("tick %d: cars %d and %d are %.2f apart", tick, k-1, k, dx+dy)
					}
				}
			}
		}
		prev = st
	}
	if stops != 2 {
		t.Errorf("%d arrivals in a round trip, want 2", stops)
	}
	// NextStop agrees with the state.
	for _, atA := range []bool{true, false} {
		arrive, leave := l.NextStop(12345, atA)
		if arrive < 12345-l.Dwell || leave-arrive != l.Dwell {
			t.Fatalf("NextStop(atA=%v) = %d, %d", atA, arrive, leave)
		}
		st := l.At(float64(arrive))
		if st.Moving || st.AtA != atA {
			t.Errorf("at the next stop (atA=%v) the train is %+v", atA, st)
		}
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}
