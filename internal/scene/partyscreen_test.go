package scene

import (
	"testing"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/input"
)

func TestPartyScreen(t *testing.T) {
	h, _ := newCrowdHarness(t)
	for _, n := range []string{"Bryn", "Cato"} {
		if err := h.e.party.Add(&entity.Actor{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	h.tick(input.Frame{})
	at := h.leader().Pos
	names := func() string {
		s := ""
		for _, m := range h.e.party.Members {
			s += m.Name + " "
		}
		return s
	}
	h.press(input.Menu)
	h.menuTo("Party")
	ps, ok := h.top().(*PartyScreen)
	if !ok {
		t.Fatalf("no party screen (top: %T)", h.top())
	}
	// Keys: pick the leader, then the third: Cato leads.
	h.press(input.Confirm)
	h.press(input.Down)
	h.press(input.Down)
	h.press(input.Confirm)
	if got := names(); got != "Cato Bryn leader " {
		t.Errorf("after swapping the first and third: %s", got)
	}
	if h.leader().Pos != at {
		t.Errorf("the new leader stands at %v, not where the party was (%v)", h.leader().Pos, at)
	}
	// Taps: the second, then the first: Bryn leads.
	_, rows := ps.rects()
	h.tapAt(centre(rows[1]))
	if ps.picked != 1 {
		t.Fatalf("picked %d after tapping the second", ps.picked)
	}
	h.tapAt(centre(rows[0]))
	if got := names(); got != "Bryn Cato leader " {
		t.Errorf("after tapping the second and first: %s", got)
	}
	// Cancel drops a pick first, then closes.
	h.press(input.Confirm)
	h.press(input.Cancel)
	if ps.picked != -1 || h.top() != Scene(ps) {
		t.Fatal("Cancel didn't just drop the pick")
	}
	h.press(input.Cancel)
	if h.top() != Scene(h.e) {
		t.Fatalf("the party screen stayed open (top: %T)", h.top())
	}
	// The party walks on behind its new leader.
	if h.leader().Name != "Bryn" {
		t.Errorf("%s leads", h.leader().Name)
	}
}
