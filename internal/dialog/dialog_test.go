package dialog_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noxer/worlds/internal/dialog"
)

const tree = `
greet:
  start: a
  nodes:
    a:
      speaker: Bob
      lines: [one, two]
      choices:
        - {text: more, next: b}
        - {text: bye}
    b:
      lines: [three]
      next: a
`

func TestRunner(t *testing.T) {
	lib, err := dialog.Load(fstest.MapFS{"d/x.yaml": {Data: []byte(tree)}}, "d")
	if err != nil {
		t.Fatal(err)
	}
	r := dialog.NewRunner(lib["greet"])
	if r.Line() != "one" || r.Speaker() != "Bob" || r.Choices() != nil {
		t.Fatalf("first line wrong")
	}
	r.Advance()
	if r.Line() != "two" || len(r.Choices()) != 2 {
		t.Fatalf("second line should offer choices")
	}
	r.Advance() // ignored while choices are shown
	if r.Line() != "two" {
		t.Fatalf("Advance skipped choices")
	}
	r.Choose(0)
	if r.Line() != "three" {
		t.Fatalf("choice did not jump")
	}
	r.Advance()
	if r.Line() != "one" {
		t.Fatalf("next did not jump back")
	}
	r.Advance()
	r.Choose(1)
	if !r.Done() {
		t.Fatalf("choice without next should end")
	}
}

func TestValidation(t *testing.T) {
	bad := strings.Replace(tree, "next: a", "next: nowhere", 1)
	_, err := dialog.Load(fstest.MapFS{"d/x.yaml": {Data: []byte(bad)}}, "d")
	if err == nil || !strings.Contains(err.Error(), "unknown node") {
		t.Fatalf("err = %v", err)
	}
}

const storyTree = `
smith:
  start: hello
  starts:
    - {if: [done], node: thanks}
    - {if: [has crate], node: handover}
  nodes:
    hello:
      lines: [My delivery never came.]
      effects: [asked]
      choices:
        - {text: We'll look., effects: [looking]}
        - {text: About the cellar, if: [found], next: cellar}
    cellar:
      lines: [Be careful down there.]
    handover:
      lines: [You found it!]
      effects: [take crate, done]
    thanks:
      lines: [Thanks again.]
`

// fakeStory holds conditions that hold and records the effects applied.
type fakeStory struct {
	holds   map[string]bool
	applied []string
}

func (f *fakeStory) All(cs []string) bool {
	for _, c := range cs {
		if !f.holds[c] {
			return false
		}
	}
	return true
}
func (f *fakeStory) Apply(e string) { f.applied = append(f.applied, e) }

func TestStory(t *testing.T) {
	lib, err := dialog.Load(fstest.MapFS{"d/x.yaml": {Data: []byte(storyTree)}}, "d")
	if err != nil {
		t.Fatal(err)
	}
	d := lib["smith"]
	// Nothing has happened: the plain start, its effect, one choice only.
	st := &fakeStory{holds: map[string]bool{}}
	r := dialog.NewRunnerWith(d, st)
	if r.Line() != "My delivery never came." || len(st.applied) != 1 || st.applied[0] != "asked" {
		t.Fatalf("start: %q, applied %v", r.Line(), st.applied)
	}
	if cs := r.Choices(); len(cs) != 1 || cs[0].Text != "We'll look." {
		t.Fatalf("choices %v, want only the unconditional one", cs)
	}
	r.Choose(0)
	if !r.Done() || len(st.applied) != 2 || st.applied[1] != "looking" {
		t.Fatalf("after choosing: done %v, applied %v", r.Done(), st.applied)
	}
	// The cellar is known: the second choice is offered, and is index 1.
	st = &fakeStory{holds: map[string]bool{"found": true}}
	r = dialog.NewRunnerWith(d, st)
	if cs := r.Choices(); len(cs) != 2 {
		t.Fatalf("choices %v, want both", cs)
	}
	r.Choose(1)
	if r.Done() || r.Line() != "Be careful down there." {
		t.Fatal("the conditional choice didn't lead to the cellar")
	}
	// With the crate: the handover, which takes it and sets done.
	st = &fakeStory{holds: map[string]bool{"has crate": true}}
	r = dialog.NewRunnerWith(d, st)
	if r.Line() != "You found it!" || len(st.applied) != 2 || st.applied[0] != "take crate" {
		t.Fatalf("with the crate: %q, applied %v", r.Line(), st.applied)
	}
	// Done comes first, even with a crate.
	st = &fakeStory{holds: map[string]bool{"has crate": true, "done": true}}
	if r = dialog.NewRunnerWith(d, st); r.Line() != "Thanks again." {
		t.Fatalf("when done: %q", r.Line())
	}
	// Without a story: the plain start, no conditional choices, no effects.
	r = dialog.NewRunner(d)
	if r.Line() != "My delivery never came." || len(r.Choices()) != 1 {
		t.Fatalf("without a story: %q, %v", r.Line(), r.Choices())
	}
	if c, e := d.Conditions(), d.Effects(); len(c) != 3 || len(e) != 4 {
		t.Errorf("conditions %v, effects %v", c, e)
	}
}

func TestStartsAreChecked(t *testing.T) {
	for name, y := range map[string]string{
		"unknown node":  "x:\n  start: a\n  starts: [{if: [c], node: nope}]\n  nodes: {a: {lines: [hi]}}",
		"no conditions": "x:\n  start: a\n  starts: [{node: a}]\n  nodes: {a: {lines: [hi]}}",
	} {
		if _, err := dialog.Load(fstest.MapFS{"d/x.yaml": {Data: []byte(y)}}, "d"); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}
