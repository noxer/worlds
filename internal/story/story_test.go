package story_test

import (
	"testing"

	"github.com/noxer/worlds/internal/story"
)

func TestFlagsAndItems(t *testing.T) {
	s := story.New()
	if s.Flag("met") || s.Count("coin") != 0 {
		t.Fatal("a new story isn't empty")
	}
	s.SetFlag("met", true)
	s.Give("coin", 5)
	if !s.Take("coin", 3) || s.Count("coin") != 2 {
		t.Errorf("after giving 5 and taking 3: %d", s.Count("coin"))
	}
	if s.Take("coin", 3) || s.Count("coin") != 2 {
		t.Errorf("took more than there was: %d left", s.Count("coin"))
	}
	if !s.Take("coin", 2) || len(s.Items()) != 0 {
		t.Errorf("items after taking the rest: %v", s.Items())
	}
	s.SetFlag("b", true)
	if f := s.Flags(); len(f) != 2 || f[0] != "b" || f[1] != "met" {
		t.Errorf("flags %v, want b and met, sorted", f)
	}
	s.SetFlag("met", false)
	if s.Flag("met") {
		t.Error("the flag is still set")
	}
}

func TestConditions(t *testing.T) {
	s := story.New()
	s.SetFlag("met", true)
	s.Give("coin", 2)
	for c, want := range map[string]bool{
		"flag met": true, "!flag met": false, "flag other": false, "!flag other": true, "! flag other": true,
		"item coin": true, "item coin 2": true, "item coin 3": false, "!item coin 3": true, "item cake": false,
		"nonsense": false, "flag": false, "flag a b c": false, "flag met 2": false,
	} {
		if got := s.Check(c); got != want {
			t.Errorf("%q: %v, want %v", c, got, want)
		}
	}
	if !s.All(nil) || !s.All([]string{"flag met", "item coin"}) || s.All([]string{"flag met", "item cake"}) {
		t.Error("All is wrong")
	}
}

func TestEffects(t *testing.T) {
	s := story.New()
	if c := s.Apply("give cake"); c != (story.Change{Item: "cake", N: 1}) || s.Count("cake") != 1 {
		t.Errorf("give cake: %+v, %d", c, s.Count("cake"))
	}
	if c := s.Apply("give coin 3"); c.N != 3 {
		t.Errorf("give coin 3: %+v", c)
	}
	if c := s.Apply("take coin 2"); c.N != -2 || s.Count("coin") != 1 {
		t.Errorf("take coin 2: %+v, %d", c, s.Count("coin"))
	}
	if c := s.Apply("take coin 2"); c != (story.Change{}) || s.Count("coin") != 1 {
		t.Errorf("taking too many: %+v, %d", c, s.Count("coin"))
	}
	s.Apply("set done")
	if !s.Flag("done") {
		t.Error("set done didn't")
	}
	s.Apply("clear done")
	if s.Flag("done") {
		t.Error("clear done didn't")
	}
}

func TestWordingIsChecked(t *testing.T) {
	for _, c := range []string{"flag met", "!flag met", "item coin 3"} {
		if _, err := story.CheckCondition(c); err != nil {
			t.Errorf("condition %q: %v", c, err)
		}
	}
	for _, c := range []string{"", "flag", "set met", "item coin x", "item coin 0", "flag met 2"} {
		if _, err := story.CheckCondition(c); err == nil {
			t.Errorf("condition %q passed", c)
		}
	}
	for _, c := range []string{"set met", "clear met", "give coin", "take coin 2"} {
		if _, err := story.CheckEffect(c); err != nil {
			t.Errorf("effect %q: %v", c, err)
		}
	}
	for _, c := range []string{"flag met", "!set met", "give", "set met 2"} {
		if _, err := story.CheckEffect(c); err == nil {
			t.Errorf("effect %q passed", c)
		}
	}
	if item, _ := story.CheckEffect("give cake 2"); item != "cake" {
		t.Errorf("give cake 2 mentions %q", item)
	}
}

func TestRestore(t *testing.T) {
	s := story.New()
	s.SetFlag("old", true)
	s.Give("old", 1)
	s.Restore([]string{"a", "b"}, map[string]int{"coin": 4, "none": 0})
	if s.Flag("old") || !s.Flag("a") || s.Count("old") != 0 || s.Count("coin") != 4 || len(s.Items()) != 1 {
		t.Errorf("after restoring: flags %v, items %v", s.Flags(), s.Items())
	}
}
