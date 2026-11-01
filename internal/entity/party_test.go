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
	p.Step(w, d, to)
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
	a.BeginStep(world.Right, pos("main", 1, 0))
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
			p.Step(w, world.Right, to)
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
