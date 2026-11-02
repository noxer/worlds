package scene

import (
	"image"
	"testing"
	"time"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

// harness runs the real scene stack on the shipped content, tick by tick,
// with injected input. Nothing is drawn.
type harness struct {
	t     *testing.T
	w     *world.World
	in    *input.State
	stack *Stack
	e     *Explore
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	c, err := content.Load(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	w := c.World
	party := entity.NewParty(&entity.Actor{Name: "leader"})
	for i := 0; i < 3; i++ {
		if err := party.Add(&entity.Actor{}); err != nil {
			t.Fatal(err)
		}
	}
	e, err := NewExplore(ExploreConfig{World: w, Sheets: gfx.NewSheets(assets.FS), Dialogs: c.Dialogs, Party: party, Start: c.Start})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, w: w, in: &input.State{}, e: e}
	h.stack = NewStack(h.in, e)
	h.stack.SetSize(320, 180)
	h.tick(input.Frame{})
	return h
}

func (h *harness) tick(f input.Frame) {
	h.in.Apply(f)
	h.stack.Update()
}

func (h *harness) press(a input.Action) {
	var f input.Frame
	f.Held[a] = true
	h.tick(f)
	h.tick(input.Frame{})
}

func (h *harness) hold(a input.Action, ticks int) {
	var f input.Frame
	f.Held[a] = true
	for i := 0; i < ticks; i++ {
		h.tick(f)
	}
	h.tick(input.Frame{})
}

func (h *harness) runUntil(what string, max int, cond func() bool) {
	h.t.Helper()
	for i := 0; i < max; i++ {
		if cond() {
			return
		}
		h.tick(input.Frame{})
	}
	h.t.Fatalf("timed out waiting for %s; leader at %v", what, h.leader().Pos)
}

func (h *harness) leader() *entity.Actor { return h.e.party.Leader() }

func (h *harness) top() Scene { return h.stack.scenes[len(h.stack.scenes)-1] }

func (h *harness) npc(name string) *NPC {
	for _, n := range h.e.npcs {
		if n.Name == name {
			return n
		}
	}
	h.t.Fatalf("no npc %q", name)
	return nil
}

func (h *harness) idle() bool {
	return !h.e.party.Busy() && h.e.fade == 0 && !h.e.hasGoal
}

// checkQueue verifies the party forms a connected queue. Followers lag a
// few ticks behind, so it compares where everyone is heading.
func (h *harness) checkQueue() {
	h.t.Helper()
	ms := h.e.party.Members
	for i := 1; i < len(ms); i++ {
		a, b := ms[i].Dest(), ms[i-1].Dest()
		if a == b {
			continue
		}
		if _, ok := h.w.DirBetween(a, b); !ok {
			h.t.Errorf("member %d heading to %v is not next to member %d heading to %v", i, a, i-1, b)
		}
	}
}

func TestKeyboardTurnThenWalk(t *testing.T) {
	h := newHarness(t)
	start := h.leader().Pos
	// A short tap only turns on the spot.
	h.press(input.Right)
	h.runUntil("idle", 100, h.idle)
	if h.leader().Pos != start || h.leader().Facing != world.Right {
		t.Fatalf("tap should only turn: at %v facing %v", h.leader().Pos, h.leader().Facing)
	}
	// Holding walks; three tiles take 3*StepTicks.
	h.hold(input.Right, 3*entity.StepTicks)
	h.runUntil("idle", 100, h.idle)
	if want, _ := h.w.Offset(start, 3, 0); h.leader().Pos != want {
		t.Fatalf("leader at %v, want 3 tiles right of %v", h.leader().Pos, start)
	}
	h.checkQueue()
}

// TestCameraDoesNotBob checks that the view moves smoothly with the leader:
// walking sideways must never move the camera vertically (the walk hop only
// moves the sprite).
func TestCameraDoesNotBob(t *testing.T) {
	h := newHarness(t)
	h.press(input.Right) // turn first
	h.runUntil("idle", 100, h.idle)
	y := h.e.camY
	lastX := h.e.camX
	var f input.Frame
	f.Held[input.Right] = true
	for i := 0; i < 3*entity.StepTicks; i++ {
		h.tick(f)
		if h.e.camY != y {
			t.Fatalf("tick %d: camera moved vertically from %d to %d", i, y, h.e.camY)
		}
		if d := h.e.camX - lastX; d < 0 || d > 1 {
			t.Fatalf("tick %d: camera moved %d px horizontally, want 0 or 1", i, d)
		}
		lastX = h.e.camX
	}
}

func TestNPCBlocksMovement(t *testing.T) {
	h := newHarness(t)
	elder := h.npc("elder")
	above, _ := h.w.Neighbor(elder.Pos, world.Up)
	h.e.party.PlaceAll(above, world.Down)
	h.hold(input.Down, 3*entity.StepTicks)
	if h.leader().Pos != above {
		t.Fatalf("walked into the elder: %v", h.leader().Pos)
	}
}

func TestTalkToNPC(t *testing.T) {
	h := newHarness(t)
	elder := h.npc("elder")
	h.e.goTo(elder.Pos)
	h.runUntil("dialog", 2000, func() bool { _, ok := h.top().(*DialogBox); return ok })

	if d, ok := h.w.DirBetween(h.leader().Pos, elder.Pos); !ok || d != h.leader().Facing {
		t.Fatalf("leader at %v facing %v is not facing the elder at %v", h.leader().Pos, h.leader().Facing, elder.Pos)
	}
	if d, _ := h.w.DirBetween(elder.Pos, h.leader().Pos); elder.Facing != d {
		t.Fatalf("elder should turn to the leader")
	}

	box := h.top().(*DialogBox)
	// The typewriter reveals one character per tick.
	h.runUntil("first line typed", 200, box.typed)
	if box.r.Line() != "Welcome to Brookhaven, travellers." {
		t.Fatalf("unexpected first line %q", box.r.Line())
	}
	h.press(input.Confirm)
	h.runUntil("second line typed", 200, box.typed)
	if len(box.choices()) != 3 {
		t.Fatalf("expected 3 choices, got %d", len(box.choices()))
	}
	// Pick "Farewell." (last choice, reached by wrapping upwards).
	h.press(input.Up)
	h.press(input.Confirm)
	if h.top() != Scene(h.e) {
		t.Fatalf("dialog should be closed")
	}
}

func TestTapToMove(t *testing.T) {
	h := newHarness(t)
	target, _ := h.w.Offset(h.leader().Pos, -2, 1)
	x, y := h.e.screen(target.Map, target.X*tile, target.Y*tile)
	p := image.Pt(x+8, y+8)
	h.tick(input.Frame{Tap: p, Tapped: true})
	h.runUntil("arrival", 500, func() bool { return h.idle() && h.leader().Pos == target })
}

// TestTravelThroughWarpAndAcrossSeam sends the party from inside the smithy
// to the meadow: out through the door (warp), along the street, across the
// east-west wrap seam of the world (the town straddles it) into the meadow.
func TestTravelThroughWarpAndAcrossSeam(t *testing.T) {
	h := newHarness(t)
	in, _ := h.w.Spawn("smithy", "entrance")
	h.e.party.PlaceAll(in.Pos, world.Up)
	meadow, _ := h.w.PatchOrigin("meadow")
	goal, _ := h.w.Offset(meadow, 20, 17)
	h.e.goTo(goal)

	var maps []world.MapRef
	crossed := false
	last := h.leader().Pos
	h.runUntil("arrival in the meadow", 5000, func() bool {
		l := h.leader().Pos
		if len(maps) == 0 || maps[len(maps)-1] != l.Map {
			maps = append(maps, l.Map)
		}
		if l.Map.IsMain() && last.Map.IsMain() && last.X == h.w.W-1 && l.X == 0 {
			crossed = true
		}
		last = l
		h.checkQueue()
		return h.idle() && l == goal
	})
	if len(maps) != 2 || maps[0] != world.Detail("smithy") || maps[1] != world.Main {
		t.Fatalf("visited maps %v, want [smithy main]", maps)
	}
	if !crossed {
		t.Fatal("the route should cross the wrap seam")
	}
	h.checkQueue()
	for i, m := range h.e.party.Members[1:] {
		if m.Pos == goal {
			t.Errorf("follower %d still stacked on the leader", i+1)
		}
	}
}

func TestEnterBuildingThroughDoor(t *testing.T) {
	h := newHarness(t)
	goal := world.Pos{Map: world.Detail("smithy"), X: 6, Y: 3}
	h.e.goTo(goal)
	h.runUntil("arrival in the smithy", 3000, func() bool { return h.idle() && h.leader().Pos == goal })
}

// TestBumpShowsPassiveNotice walks into the locked door: a message appears
// without opening a dialog, input keeps working, and it goes away when the
// party walks off.
func TestBumpShowsPassiveNotice(t *testing.T) {
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	door, _ := h.w.Offset(town, 22, 5)
	below, _ := h.w.Offset(door, 0, 1)
	h.e.party.PlaceAll(below, world.Up)

	h.hold(input.Up, 5)
	if h.e.notice == nil || h.e.notice.text != "The door is locked." {
		t.Fatalf("expected the locked-door notice, got %+v", h.e.notice)
	}
	if h.top() != Scene(h.e) {
		t.Fatal("bumping must not open a modal dialog")
	}
	if h.leader().Pos != below {
		t.Fatal("walked into the locked door")
	}
	// It stays while the party stands still.
	for i := 0; i < 120; i++ {
		h.tick(input.Frame{})
	}
	if h.e.notice == nil {
		t.Fatal("notice vanished without moving")
	}
	// Walking away clears it straight away, no confirmation needed.
	var f input.Frame
	f.Held[input.Down] = true
	h.tick(f)
	for i := 0; i < turnDelay+2 && h.e.notice != nil; i++ {
		h.tick(f)
	}
	if h.e.notice != nil {
		t.Fatal("notice should disappear when walking away")
	}

	// Facing the door and pressing confirm still opens the full dialog.
	h.e.party.PlaceAll(below, world.Up)
	h.press(input.Confirm)
	if _, ok := h.top().(*DialogBox); !ok {
		t.Fatal("confirm at the door should open the dialog")
	}
}

// meadowAt returns a position in the meadow patch.
func (h *harness) meadowAt(x, y int64) world.Pos {
	o, _ := h.w.PatchOrigin("meadow")
	p, _ := h.w.Offset(o, x, y)
	return p
}

// stepTime presses a direction for one tick (the leader must already face
// it) and returns how many ticks the resulting step takes.
func (h *harness) stepTime(d input.Action) int {
	h.t.Helper()
	var f input.Frame
	f.Held[d] = true
	start := h.leader().Pos
	h.tick(f)
	if h.leader().Pos == start {
		h.t.Fatalf("leader did not move from %v", start)
	}
	for i := 0; i < 100; i++ {
		if !h.leader().Moving() {
			return i
		}
		h.tick(input.Frame{})
	}
	h.t.Fatal("step never ended")
	return 0
}

func TestSwimmingInShallowWater(t *testing.T) {
	h := newHarness(t)
	shore := h.meadowAt(2, 12)
	if h.w.Terrain(shore) != world.Land || h.w.Terrain(h.meadowAt(2, 13)) != world.Shallow {
		t.Fatal("test setup: expected land at meadow (2,12) and shallow water below")
	}
	h.e.party.PlaceAll(shore, world.Down)
	if got := h.stepTime(input.Down); got != nav.SwimTicks {
		t.Fatalf("stepping into the water took %d ticks, want %d", got, nav.SwimTicks)
	}
	if h.w.Terrain(h.leader().Pos) != world.Shallow {
		t.Fatal("leader should be swimming")
	}
	// Deep water cannot be entered; a notice says why.
	h.e.party.PlaceAll(h.meadowAt(2, 15), world.Right)
	h.hold(input.Right, 10)
	if h.leader().Pos != h.meadowAt(2, 15) {
		t.Fatal("swam into deep water")
	}
	if h.e.notice == nil || h.e.notice.text != "The water is too deep to swim." {
		t.Fatalf("notice = %+v", h.e.notice)
	}
}

func TestRiding(t *testing.T) {
	h := newHarness(t)
	h.press(input.Ride)
	if !h.e.riding {
		t.Fatalf("could not mount: notice %+v", h.e.notice)
	}
	h.press(input.Right) // turn
	h.runUntil("idle", 50, h.idle)
	if got := h.stepTime(input.Right); got != nav.RideTicks {
		t.Fatalf("a riding step took %d ticks, want %d", got, nav.RideTicks)
	}
	// Riders can't go into the water.
	shore := h.meadowAt(2, 12)
	h.e.party.PlaceAll(shore, world.Down)
	h.hold(input.Down, 10)
	if h.leader().Pos != shore {
		t.Fatal("rode into the water")
	}
	if h.e.notice == nil || h.e.notice.text != "You can't ride into the water." {
		t.Fatalf("notice = %+v", h.e.notice)
	}
	// Clicking a water tile gets the party off the bikes and swimming there.
	goal := h.meadowAt(2, 14)
	h.e.goTo(goal)
	if h.e.riding {
		t.Fatal("should dismount to reach the water")
	}
	h.runUntil("swim to the goal", 500, func() bool { return h.idle() && h.leader().Pos == goal })
	h.press(input.Ride)
	if h.e.riding {
		t.Fatal("mounted a bike in the water")
	}
}

func TestRidingIndoorsAndThroughDoors(t *testing.T) {
	h := newHarness(t)
	h.press(input.Ride)
	goal := world.Pos{Map: world.Detail("smithy"), X: 6, Y: 3}
	h.e.goTo(goal)
	if !h.e.riding {
		t.Fatal("the way to the door is on land: keep riding")
	}
	h.runUntil("arrival in the smithy", 3000, func() bool { return h.idle() && h.leader().Pos == goal })
	if h.e.riding {
		t.Fatal("the party should get off the bikes at the door")
	}
	h.press(input.Ride)
	if h.e.riding || h.e.notice == nil || h.e.notice.text != "You can't ride indoors." {
		t.Fatalf("riding indoors: riding=%v notice=%+v", h.e.riding, h.e.notice)
	}
}

// screenJitter moves the party in direction d for several steps and counts,
// per member, how often its on-screen position along the direction of travel
// reverses. A smooth queue drifts at most once or twice (when the party sets
// off and stops); jitter reverses every few ticks.
func (h *harness) screenJitter(d input.Action, ticks int, only func(*entity.Actor) bool) []int {
	h.t.Helper()
	var f input.Frame
	f.Held[d] = true
	flips := make([]int, len(h.e.party.Members))
	last := make([]int, len(h.e.party.Members))
	dir := make([]int, len(h.e.party.Members))
	seen := make([]bool, len(h.e.party.Members))
	for tick := 0; tick < ticks; tick++ {
		h.tick(f)
		for i, a := range h.e.party.Members {
			if only != nil && !only(a) {
				seen[i] = false
				continue
			}
			x, y, _ := h.e.placement(a, h.e.riding)
			v := x
			if d == input.Up || d == input.Down {
				v = y
			}
			if seen[i] && v != last[i] {
				s := 1
				if v < last[i] {
					s = -1
				}
				if dir[i] != 0 && s != dir[i] {
					flips[i]++
				}
				dir[i] = s
			}
			last[i], seen[i] = v, true
		}
	}
	return flips
}

// TestNoJitterOnScreen checks walking, swimming and riding: party members
// must not wobble back and forth on screen while the camera follows the
// leader. That needs the follow delay to be a whole number of pixels' worth
// of ticks at every speed.
func TestNoJitterOnScreen(t *testing.T) {
	check := func(name string, flips []int) {
		t.Helper()
		for i, n := range flips {
			if n > 2 {
				t.Errorf("%s: member %d reversed on screen %d times", name, i, n)
			}
		}
	}
	// Walking along the town street.
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	street, _ := h.w.Offset(town, 4, 9)
	h.e.party.PlaceAll(street, world.Right)
	check("walking", h.screenJitter(input.Right, 8*nav.WalkTicks, nil))

	// Riding along the same street.
	h = newHarness(t)
	h.e.party.PlaceAll(street, world.Right)
	h.press(input.Ride)
	if !h.e.riding {
		t.Fatal("could not mount")
	}
	check("riding", h.screenJitter(input.Right, 12*nav.RideTicks, nil))

	// Swimming across the pond (row 13, its top shore, is shallow from x=1
	// to x=7).
	h = newHarness(t)
	h.e.party.PlaceAll(h.meadowAt(1, 13), world.Right)
	swimming := func(a *entity.Actor) bool { return h.w.Terrain(a.Pos) == world.Shallow }
	check("swimming", h.screenJitter(input.Right, 6*nav.SwimTicks, swimming))
}

// TestLongTrip sends the riding party to a spot far beyond the local
// planning range. The scene follows the planner's stretches until arrival.
func TestLongTrip(t *testing.T) {
	h := newHarness(t)
	exit := h.meadowAt(23, 9)
	h.e.party.PlaceAll(exit, world.Right)
	h.press(input.Ride)
	if !h.e.riding {
		t.Fatal("could not mount")
	}
	var goal world.Pos
	for i := int64(0); i < 64; i++ {
		q, _ := h.w.Offset(exit, 600+i, i%7)
		if _, ok := nav.StepTicks(h.w, q, nav.Ride); ok {
			goal = q
			break
		}
	}
	if goal == (world.Pos{}) {
		t.Fatal("no goal found")
	}
	h.e.goTo(goal)
	if !h.e.hasGoal {
		t.Fatal("goTo refused a far goal")
	}
	h.runUntil("arrival", 40000, func() bool {
		h.checkQueue()
		return h.idle() && h.leader().Pos == goal
	})
}

// TestDetourAroundTreeBorder clicks just outside the town's west tree line.
// The only way there is out through the meadow and around both patches,
// a detour far wider than the straight distance.
func TestDetourAroundTreeBorder(t *testing.T) {
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	inside, _ := h.w.Offset(town, 2, 10)
	outside, _ := h.w.Offset(town, -2, 10)
	if h.w.Solid(inside) || h.w.Solid(outside) {
		t.Fatal("test setup: both tiles must be walkable")
	}
	h.e.party.PlaceAll(inside, world.Left)
	h.e.goTo(outside)
	if !h.e.hasGoal {
		t.Fatalf("goTo refused the goal; notice %+v", h.e.notice)
	}
	h.runUntil("arrival outside the tree line", 20000, func() bool { return h.idle() && h.leader().Pos == outside })
}

// TestUnreachableClickExplains: a walkable spot that can't be reached gets
// a notice instead of silently doing nothing.
func TestUnreachableClickExplains(t *testing.T) {
	h := newHarness(t)
	town, _ := h.w.PatchOrigin("town")
	spot, _ := h.w.Offset(town, 12, 12)
	for _, d := range world.Dirs {
		n, _ := h.w.Neighbor(spot, d)
		h.w.SetBlocked(n, true)
	}
	t0 := time.Now()
	h.e.goTo(spot)
	if took := time.Since(t0); took > 200*time.Millisecond {
		t.Errorf("giving up took %v", took)
	}
	if h.e.hasGoal {
		t.Fatal("walled-in spot accepted as goal")
	}
	if h.e.notice == nil || h.e.notice.text != "There's no way there." {
		t.Fatalf("notice = %+v", h.e.notice)
	}
}
