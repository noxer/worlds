package sim_test

import (
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

var pos = worldtest.Pos

// A corridor with spots "a" (x=1) and "b" (x=11), and a house entered
// through a door at x=6 on the corridor's north wall.
func testWorld(t *testing.T) *world.World {
	return worldtest.Load(t, worldtest.Spec{
		Patches: []worldtest.Map{{
			Name: "street", X: 0, Y: 10,
			Rows: []string{
				"######.#######",
				"#.............",
				"##############",
			},
			Spawns: map[string][2]int{"a": {1, 1}, "b": {11, 1}, "porch": {6, 1}},
			Warps:  []worldtest.Warp{{X: 6, Y: 0, ToMap: "house", ToSpawn: "in"}},
		}},
		Details: []worldtest.Map{{
			Name:   "house",
			Rows:   []string{"#####", "#...#", "##.##"},
			Spawns: map[string][2]int{"in": {2, 1}},
			Warps:  []worldtest.Warp{{X: 2, Y: 2, ToMap: "main", ToSpawn: "porch"}},
		}},
	})
}

func TestClock(t *testing.T) {
	c := sim.NewClock(3, 23, 59)
	for i := 0; i < sim.TicksPerMinute; i++ {
		c.Tick()
	}
	if c.String() != "day 4, 00:00" {
		t.Fatalf("clock = %s", c)
	}
}

func TestParseSchedule(t *testing.T) {
	w := testWorld(t)
	s, err := sim.ParseSchedule(w, "12:00 b; 08:30 a ; 20:00 house/in")
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 3 || s[0].At != 8*60+30 || s[2].Place != pos("house", 2, 1) {
		t.Fatalf("schedule = %+v", s)
	}
	for _, bad := range []string{"", "8 a", "25:00 a", "08:00 nowhere", "08:00 a; 08:00 b", "08:00 house/nowhere"} {
		if _, err := sim.ParseSchedule(w, bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestCurrentWrapsAroundMidnight(t *testing.T) {
	w := testWorld(t)
	s, _ := sim.ParseSchedule(w, "08:00 a; 20:00 b")
	day2 := int64(2 * sim.MinutesPerDay)
	cur, prev, since := s.Current(day2 + 7*60) // 07:00: still last night's entry
	if cur.Name != "b" || prev.Name != "a" || since != day2-4*60 {
		t.Fatalf("at 07:00: cur %s prev %s since %d", cur.Name, prev.Name, since)
	}
	cur, _, since = s.Current(day2 + 9*60)
	if cur.Name != "a" || since != day2+8*60 {
		t.Fatalf("at 09:00: cur %s since %d", cur.Name, since)
	}
}

func TestWhere(t *testing.T) {
	w := testWorld(t)
	p := nav.NewPlanner(w)
	s, _ := sim.ParseSchedule(w, "08:00 a; 12:00 b; 18:00 house/in")
	day := int64(sim.MinutesPerDay)
	at := func(h, m int) world.Pos { return sim.Where(p, w, s, day+int64(h*60+m)) }
	// At noon she sets off from a towards b (10 steps of 16 ticks = 160
	// ticks, under 3 game minutes).
	if got := at(12, 0); got != pos("main", 1, 11) {
		t.Errorf("12:00 at %v, want at a", got)
	}
	if got := at(12, 2); got != pos("main", 8, 11) { // 120 ticks: 7 steps
		t.Errorf("12:02 at %v, want 7 steps along", got)
	}
	if got := at(13, 0); got != pos("main", 11, 11) {
		t.Errorf("13:00 at %v, want at b", got)
	}
	// In the evening she goes indoors through the door.
	if got := at(19, 0); got != pos("house", 2, 1) {
		t.Errorf("19:00 at %v, want inside", got)
	}
	if !strings.HasPrefix(sim.NewClock(1, 9, 5).String(), "day 1") {
		t.Error("clock string")
	}
}
