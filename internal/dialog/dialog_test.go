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
