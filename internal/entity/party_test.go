package entity_test

import (
	"testing"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

var pos = worldtest.Pos

// A 64x64 world whose open floor straddles the east-west seam.
func testWorld(t *testing.T) *world.World {
	return worldtest.Load(t, worldtest.Spec{
		Patches: []worldtest.Map{{Name: "strip", X: -4, Y: 0, Rows: []string{"........", "........"}}},
	})
}

// walk moves the leader one tile and runs the animation to completion.
func walk(t *testing.T, w *world.World, p *entity.Party, d world.Dir) {
	t.Helper()
	to, ok := w.Neighbor(p.Leader().Pos, d)
	if !ok {
		t.Fatalf("cannot walk %v from %v", d, p.Leader().Pos)
	}
	p.Step(w, d, to, nil)
	for i := 0; i < entity.StepTicks; i++ {
		arrived := p.Update()
		if arrived != (i == entity.StepTicks-1) {
			t.Fatalf("arrival on tick %d", i)
		}
	}
	// Followers lag behind; let them catch up.
	for i := 0; p.Busy(); i++ {
		if i > entity.StepTicks*2 {
			t.Fatal("followers never caught up")
		}
		p.Update()
	}
}

func newParty(n int) *entity.Party {
	p := entity.NewParty(&entity.Actor{Name: "leader"})
	for i := 0; i < n; i++ {
		if err := p.Add(&entity.Actor{}); err != nil {
			panic(err)
		}
	}
	return p
}

func TestPartyUnfoldsAndFollowsAcrossSeam(t *testing.T) {
	w := testWorld(t)
	p := newParty(3)
	p.PlaceAll(pos("main", 61, 0), world.Right)

	walk(t, w, p, world.Right) // 62
	walk(t, w, p, world.Right) // 63
	walk(t, w, p, world.Right) // 0, across the seam
	walk(t, w, p, world.Down)  // (0,1)

	want := []world.Pos{pos("main", 0, 1), pos("main", 0, 0), pos("main", 63, 0), pos("main", 62, 0)}
	for i, m := range p.Members {
		if m.Pos != want[i] {
			t.Errorf("member %d at %v, want %v", i, m.Pos, want[i])
		}
	}
	if f := p.Members[1]; f.Facing != world.Right {
		t.Errorf("follower 1 faces %v, want right", f.Facing)
	}
}

func TestPartySwapWhenLeaderTurnsBack(t *testing.T) {
	w := testWorld(t)
	p := newParty(1)
	p.PlaceAll(pos("main", 0, 0), world.Right)
	walk(t, w, p, world.Right)
	walk(t, w, p, world.Left) // into the follower's tile: they swap
	if p.Leader().Pos != pos("main", 0, 0) || p.Members[1].Pos != pos("main", 1, 0) {
		t.Fatalf("leader %v follower %v", p.Leader().Pos, p.Members[1].Pos)
	}
}

func TestPartyLimit(t *testing.T) {
	p := newParty(entity.MaxFollowers)
	if err := p.Add(&entity.Actor{}); err != entity.ErrPartyFull {
		t.Fatalf("err = %v", err)
	}
}

func TestOffsetInterpolates(t *testing.T) {
	a := &entity.Actor{}
	a.BeginStep(world.Right, pos("main", 1, 0), 0)
	if dx, dy := a.Offset(); dx != -16 || dy != 0 {
		t.Fatalf("offset at start = %d,%d", dx, dy)
	}
	for i := 0; i < entity.StepTicks/2; i++ {
		a.Update()
	}
	if dx, _ := a.Offset(); dx != -8 {
		t.Fatalf("offset halfway = %d", dx)
	}
}

// TestFollowersRipple checks that each follower starts its step FollowDelay
// ticks after the member in front, and that continuous walking keeps that
// fixed lag instead of accumulating delay.
func TestFollowersRipple(t *testing.T) {
	w := testWorld(t)
	p := newParty(3)
	p.PlaceAll(pos("main", 0, 0), world.Right)
	// Unfold the queue first.
	for i := 0; i < 3; i++ {
		walk(t, w, p, world.Right)
	}
	const steps = 3
	started := make([][]int, len(p.Members)) // tick each member started a step
	for tick := 0; tick < steps*entity.StepTicks+entity.StepTicks; tick++ {
		l := p.Leader()
		if !l.Moving() && tick < steps*entity.StepTicks {
			to, _ := w.Neighbor(l.Pos, world.Right)
			p.Step(w, world.Right, to, nil)
		}
		for i, m := range p.Members {
			// A step that has just started is a full tile behind.
			if dx, _ := m.Offset(); m.Moving() && dx == -world.TileSize {
				started[i] = append(started[i], tick)
			}
		}
		p.Update()
	}
	for i := 1; i < len(p.Members); i++ {
		if len(started[i]) != steps {
			t.Fatalf("member %d took %d steps, want %d", i, len(started[i]), steps)
		}
		for s := range started[i] {
			if lag := started[i][s] - started[0][s]; lag != i*entity.FollowDelay {
				t.Errorf("member %d step %d lags %d ticks, want %d", i, s, lag, i*entity.FollowDelay)
			}
		}
	}
}

func TestStepDurations(t *testing.T) {
	a := &entity.Actor{}
	a.BeginStep(world.Right, pos("main", 1, 0), 8)
	for i := 0; i < 7; i++ {
		if a.Update() {
			t.Fatalf("arrived early on tick %d", i)
		}
	}
	if dx, _ := a.Offset(); dx != -2 {
		t.Fatalf("offset after 7 of 8 ticks = %d, want -2", dx)
	}
	if !a.Update() {
		t.Fatal("8-tick step did not arrive on tick 8")
	}
}

// When riding (8-tick steps) the follow delay shrinks, so the whole queue
// still starts within one step of the leader.
func TestFollowDelayScalesWithSpeed(t *testing.T) {
	w := testWorld(t)
	p := newParty(entity.MaxFollowers)
	p.PlaceAll(pos("main", 0, 0), world.Right)
	for i := 0; i < entity.MaxFollowers; i++ {
		walk(t, w, p, world.Right)
	}
	to, _ := w.Neighbor(p.Leader().Pos, world.Right)
	p.Step(w, world.Right, to, func(world.Pos) int { return 8 })
	ticks := 0
	for ; !p.Members[len(p.Members)-1].Moving(); ticks++ {
		p.Update()
	}
	if ticks >= 8 {
		t.Fatalf("last follower started %d ticks after the leader, want under one step (8)", ticks)
	}
}

// TestFollowersCatchUpAfterSwimming walks without stopping out of the water
// (24-tick steps) onto land (16-tick steps), three times. The leader leaves
// the water first, so each follower briefly falls behind; on land it must
// catch up instead of keeping the gap.
func TestFollowersCatchUpAfterSwimming(t *testing.T) {
	w := worldtest.Load(t, worldtest.Spec{Width: 256, Height: 64, Fill: '.'})
	p := newParty(entity.MaxFollowers)
	p.PlaceAll(pos("main", 0, 10), world.Right)
	ticks := func(q world.Pos) int {
		if q.X%30 < 8 {
			return 24 // water
		}
		return 16
	}
	pixel := func(a *entity.Actor) int {
		dx, _ := a.Offset()
		return int(a.Pos.X)*world.TileSize + dx
	}
	for steps := 0; steps < 90; {
		if !p.Leader().Moving() {
			to, _ := w.Neighbor(p.Leader().Pos, world.Right)
			p.Step(w, world.Right, to, ticks)
			steps++
			// Well after each exit from the water, on land, the queue must
			// be as tight as when it entered.
			if x := p.Leader().Pos.X % 30; x == 28 {
				for i := 1; i < len(p.Members); i++ {
					if gap := pixel(p.Members[i-1]) - pixel(p.Members[i]); gap > world.TileSize+2*entity.FollowDelay {
						t.Errorf("step %d: member %d trails %d px behind the one ahead", steps, i, gap)
					}
				}
			}
		}
		p.Update()
	}
}
