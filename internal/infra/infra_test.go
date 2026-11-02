package infra_test

import (
	"fmt"
	"hash/fnv"
	"sort"
	"testing"
	"time"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
)

const (
	testW = 1 << 18
	testH = 1 << 17
)

// relief is a synthetic terrain: land everywhere, gently rolling, except
// a band of deep sea at seaX0 <= x < seaX1 (if set).
type relief struct{ seaX0, seaX1 int64 }

func (r relief) Elevation(_ uint64, x, y int64) int64 {
	x = geom.Mod(x, testW)
	if x >= r.seaX0 && x < r.seaX1 {
		return 100
	}
	return 40000 + (x+y)%97
}

func (r relief) Height(_ uint64, x, y int64) int64 { return 40000 + geom.Mod(x, testW)/1024 }
func (r relief) Levels() (sea, deep int64)         { return 32300, 32000 }

func newInfra(t *testing.T, r relief, fixed ...infra.Fixed) *infra.Infra {
	t.Helper()
	in, err := infra.New(infra.Config{Seed: 42, W: testW, H: testH, Relief: r, Fixed: fixed})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestSettlementsDeterministic(t *testing.T) {
	a, b := newInfra(t, relief{}), newInfra(t, relief{})
	sa := a.Near(50000, 50000, 30000, infra.POI)
	// b asks in another order: bigger tiers first, then all.
	b.Near(50000, 50000, 30000, infra.City)
	sb := b.Near(50000, 50000, 30000, infra.POI)
	if len(sa) == 0 || fmt.Sprint(sa) != fmt.Sprint(sb) {
		t.Fatalf("settlements differ or are missing: %d vs %d", len(sa), len(sb))
	}
}

func TestSettlementSpacing(t *testing.T) {
	in := newInfra(t, relief{})
	ss := in.Near(0, 0, 40000, infra.POI) // across both seams
	counts := map[infra.Tier]int{}
	for i, a := range ss {
		counts[a.ID.Tier]++
		for _, b := range ss[i+1:] {
			d := in.Dist(a, b)
			small := min(a.ID.Tier, b.ID.Tier)
			if a.ID.Tier != b.ID.Tier {
				if need := a.Radius + b.Radius + infra.Gap(small); d < need {
					t.Errorf("%v and %v are %d apart, need %d", a.ID, b.ID, d, need)
				}
			} else if need := small.Cell() * 3 / 10; d < need {
				t.Errorf("%v and %v are %d apart, need %d", a.ID, b.ID, d, need)
			}
		}
	}
	for tier := infra.POI; tier <= infra.Town; tier++ {
		if counts[tier] == 0 {
			t.Errorf("no %s near the origin: %v", tier, counts)
		}
	}
}

func TestFixedSettlement(t *testing.T) {
	in := newInfra(t, relief{}, infra.Fixed{Name: "home", Tier: infra.Village, X: 20000, Y: 20000, Radius: 15})
	found := false
	for _, s := range in.Near(20000, 20000, 6000, infra.POI) {
		if s.Fixed == "home" {
			found = true
			continue
		}
		if s.ID.Tier <= infra.Village {
			if d := in.Dist(s, infra.Settlement{X: 20000, Y: 20000}); d < 15+s.Radius+infra.Gap(s.ID.Tier) {
				t.Errorf("%v at %d tiles crowds the hand-made village", s.ID, d)
			}
		}
	}
	if !found {
		t.Fatal("the hand-made village is missing")
	}
}

// TestNetworkGraph: the relative neighbourhood graph is symmetric, keeps
// to the maximum edge length, and connects all villages on open land.
func TestNetworkGraph(t *testing.T) {
	in := newInfra(t, relief{})
	ss := in.Near(30000, 30000, 8000, infra.Village)
	adj := map[infra.ID][]infra.ID{}
	for _, s := range ss {
		for _, e := range in.Edges(infra.Path, s) {
			if in.Dist(e.A, e.B) > infra.Path.MaxLen() {
				t.Errorf("edge %v-%v is longer than the maximum", e.A.ID, e.B.ID)
			}
			other := e.B
			if e.B.ID == s.ID {
				other = e.A
			}
			adj[s.ID] = append(adj[s.ID], other.ID)
			back := false
			for _, f := range in.Edges(infra.Path, other) {
				back = back || f == e
			}
			if !back {
				t.Errorf("edge %v-%v is missing at %v", e.A.ID, e.B.ID, other.ID)
			}
		}
	}
	// Connected through settlements in and somewhat around the area.
	center := infra.Settlement{X: 30000, Y: 30000}
	seen := map[infra.ID]bool{ss[0].ID: true}
	queue := []infra.ID{ss[0].ID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		s, _ := in.Get(id)
		if in.Dist(s, center) > 14000 {
			continue
		}
		for _, e := range in.Edges(infra.Path, s) {
			for _, o := range []infra.ID{e.A.ID, e.B.ID} {
				if !seen[o] {
					seen[o] = true
					queue = append(queue, o)
				}
			}
		}
	}
	for _, s := range ss {
		if !seen[s.ID] {
			t.Errorf("%v is not connected", s.ID)
		}
	}
}

// TestRoutes: routes run from A to B in grid steps, also across the seam.
func TestRoutes(t *testing.T) {
	in := newInfra(t, relief{})
	n := 0
	for _, s := range in.Near(0, 0, 15000, infra.Village) {
		for _, e := range in.Edges(infra.Path, s) {
			r := in.Route(e)
			first, last := r.Points[0], r.Points[len(r.Points)-1]
			if first != (infra.Point{X: e.A.X, Y: e.A.Y}) || last != (infra.Point{X: e.B.X, Y: e.B.Y}) {
				t.Fatalf("route %v-%v runs from %v to %v", e.A.ID, e.B.ID, first, last)
			}
			g := infra.Path.Grid()
			for i := 1; i < len(r.Points)-1; i++ {
				dx := geom.Abs(geom.WrapDelta(r.Points[i-1].X, r.Points[i].X, testW))
				dy := geom.Abs(geom.WrapDelta(r.Points[i-1].Y, r.Points[i].Y, testH))
				if max(dx, dy) != g || (dx != 0 && dx != g) || (dy != 0 && dy != g) {
					t.Fatalf("route %v-%v jumps %d,%d at point %d", e.A.ID, e.B.ID, dx, dy, i)
				}
			}
			n++
		}
	}
	if n == 0 {
		t.Fatal("no routes near the origin")
	}
}

// TestRoutesAvoidDeepSea: land networks don't cross deep sea; sea lanes
// cross it instead.
func TestRoutesAvoidDeepSea(t *testing.T) {
	in := newInfra(t, relief{seaX0: 60000, seaX1: 90000})
	for _, net := range []infra.Network{infra.Path, infra.Road} {
		for _, s := range in.Near(75000, 60000, 35000, net.MinTier()) {
			for _, e := range in.Edges(net, s) {
				west, east := e.A.X < 60000, e.B.X < 60000
				if west != east {
					t.Errorf("%s %v-%v crosses the deep sea", net, e.A.ID, e.B.ID)
				}
			}
		}
	}
}

// TestBrookhaven: the hand-made start village is part of the real world's
// networks.
func TestBrookhaven(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	town, _ := c.World.PatchOrigin("town")
	var home infra.Settlement
	for _, s := range c.Infra.Near(town.X, town.Y, 100, infra.POI) {
		if s.Fixed == "town" {
			home = s
		}
	}
	if home.Fixed == "" {
		t.Fatal("Brookhaven is not a settlement")
	}
	if home.ID.Tier != infra.Village {
		t.Errorf("Brookhaven is a %s", home.ID.Tier)
	}
	if len(c.Infra.Edges(infra.Path, home)) == 0 {
		t.Error("no path leads to Brookhaven")
	}
}

// TestFingerprint fails when settlements or routes of the real world
// change. Update the value when that is intended.
func TestFingerprint(t *testing.T) {
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	in := c.Infra
	h := fnv.New64a()
	town, _ := c.World.PatchOrigin("town")
	for _, s := range in.Near(town.X, town.Y, 30000, infra.POI) {
		fmt.Fprint(h, s)
	}
	for _, net := range []infra.Network{infra.Path, infra.Road, infra.SeaLane} {
		for _, e := range in.EdgesIn(net, town.X-15000, town.Y-15000, 30000, 30000) {
			fmt.Fprint(h, e.A.ID, e.B.ID, in.Route(e).Points)
		}
	}
	const want = 0x46c253698ce491b6
	if got := h.Sum64(); got != want {
		t.Errorf("infrastructure fingerprint %#x, want %#x", got, uint64(want))
	}
}

// TestConcurrent: the caches are shared between goroutines (run with -race).
func TestConcurrent(t *testing.T) {
	in := newInfra(t, relief{})
	ss := in.Near(10000, 10000, 9000, infra.Village)
	done := make(chan []infra.Edge)
	for range 4 {
		go func() {
			var all []infra.Edge
			for _, s := range ss {
				all = append(all, in.Edges(infra.Path, s)...)
			}
			done <- all
		}()
	}
	first := fmt.Sprint(<-done)
	for range 3 {
		if fmt.Sprint(<-done) != first {
			t.Error("goroutines saw different edges")
		}
	}
}

// TestPrefetch: blocks prepared in the background give the same ways as
// blocks computed on demand.
func TestPrefetch(t *testing.T) {
	bg, direct := newInfra(t, relief{}), newInfra(t, relief{})
	const x, y = 20000, 30000
	bg.Prefetch(x, y)
	deadline := time.Now().Add(20 * time.Second)
	for !bg.BlockReady(x, y) || !bg.BlockReady(x+infra.BlockSize, y+infra.BlockSize) {
		if time.Now().After(deadline) {
			t.Fatal("the blocks around the anchor never got ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ways := func(in *infra.Infra) string {
		var out []string
		for _, w := range in.WaysIn(x-1500, y-1500, 3000, 3000) {
			out = append(out, fmt.Sprint(w.Net, w.OX, w.OY, w.Pts))
		}
		sort.Strings(out)
		return fmt.Sprint(out)
	}
	if a, b := ways(bg), ways(direct); a != b || a == "[]" {
		t.Errorf("prefetched ways differ from direct ones (or there are none):\n%.200s\n%.200s", a, b)
	}
}

var testPrefabs = []infra.PrefabSpec{
	{Name: "house", Kind: "house", W: 4, H: 4, Weight: 3, DoorX: 2},
	{Name: "garden", Kind: "garden", W: 5, H: 3, Weight: 1},
	{Name: "well", Kind: "well", W: 3, H: 3, Weight: 1},
}

func newInfraWithPrefabs(t *testing.T) *infra.Infra {
	t.Helper()
	in, err := infra.New(infra.Config{Seed: 42, W: testW, H: testH, Relief: relief{}, Prefabs: testPrefabs})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// TestVillageLayouts: villages get a square with a well, houses that don't
// overlap, and a footpath from every front step to a street.
func TestVillageLayouts(t *testing.T) {
	in := newInfraWithPrefabs(t)
	sites := in.SitesIn(20000, 20000, 20000, 20000)
	if len(sites) < 5 {
		t.Fatalf("only %d villages", len(sites))
	}
	again := newInfraWithPrefabs(t).SitesIn(20000, 20000, 20000, 20000)
	for k, site := range sites {
		if fmt.Sprint(site.Buildings, site.Lamps) != fmt.Sprint(again[k].Buildings, again[k].Lamps) {
			t.Fatalf("village %v: layouts differ between runs", site.S.ID)
		}
		houses, wells := 0, 0
		for _, b := range site.Buildings {
			pf := testPrefabs[b.Prefab]
			i0 := geom.WrapDelta(site.X0, b.X, testW)
			j0 := geom.WrapDelta(site.Y0, b.Y, testH)
			for j := j0; j < j0+pf.H; j++ {
				for i := i0; i < i0+pf.W; i++ {
					c := site.At(i, j)
					step := pf.Kind == "house" && i == i0+pf.DoorX && j == j0+pf.H-1
					if (step && c != infra.Foot) || (!step && c != infra.Built) {
						t.Fatalf("village %v: %s at %d,%d has a cell of kind %d", site.S.ID, pf.Name, i, j, c)
					}
				}
			}
			switch pf.Kind {
			case "well":
				wells++
			case "house":
				houses++
				if !reachesStreet(site, i0+pf.DoorX, j0+pf.H-1) {
					t.Errorf("village %v: house at %d,%d has no footpath to a street", site.S.ID, b.X, b.Y)
				}
			}
		}
		if wells != 1 || houses < 4 {
			t.Errorf("village %v: %d wells, %d houses", site.S.ID, wells, houses)
		}
	}
}

// reachesStreet follows footpath cells from (i, j) to a street, lane or
// the square.
func reachesStreet(site *infra.Site, i, j int64) bool {
	seen := map[[2]int64]bool{{i, j}: true}
	queue := [][2]int64{{i, j}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range [4][2]int64{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
			q := [2]int64{p[0] + d[0], p[1] + d[1]}
			switch site.At(q[0], q[1]) {
			case infra.Street, infra.Lane, infra.Square:
				return true
			case infra.Foot:
				if !seen[q] {
					seen[q] = true
					queue = append(queue, q)
				}
			}
		}
	}
	return false
}

// TestRailIsStraight: rail ways run along the axes, so the track art
// needs only right-angle curves.
func TestRailIsStraight(t *testing.T) {
	in := newInfra(t, relief{})
	n := 0
	for _, s := range in.Near(60000, 60000, 70000, infra.City) {
		for _, e := range in.Edges(infra.Rail, s) {
			w := in.WayOf(e)
			n++
			for k := 1; k < len(w.Pts); k++ {
				a, b := w.Pts[k-1], w.Pts[k]
				if a[0] != b[0] && a[1] != b[1] {
					t.Fatalf("rail way %v-%v runs diagonally from %v to %v", e.A.ID, e.B.ID, a, b)
				}
			}
		}
		if n >= 3 {
			break
		}
	}
	if n == 0 {
		t.Fatal("no rail found")
	}
}
