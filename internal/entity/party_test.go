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
