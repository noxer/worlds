package quest_test

import (
	"testing"

	"github.com/noxer/worlds/internal/quest"
	"github.com/noxer/worlds/internal/story"
)

const book = `
- id: slimes
  name: Slimes in the meadow
  start: [flag job]
  stages:
    - {id: clear, text: Clear the field., done: [flag beat_1, flag beat_2]}
    - {id: report, text: Tell the elder., done: [flag reported], effects: [give coin 5]}
  effects: [give coin 10]
- id: crate
  name: The crate
  start: [flag asked]
  stages:
    - {id: fetch, text: Get the crate., done: [item crate]}
    - {id: back, text: Bring it back., done: [flag crate_back], effects: [take crate]}
`

func load(t *testing.T) quest.Book {
	t.Helper()
	b, err := quest.Load([]byte(book), func(item string) bool { return item == "coin" || item == "crate" })
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQuestRuns(t *testing.T) {
	b, st := load(t), story.New()
	if ev := b.Update(st); len(ev) != 0 || len(b.Journal(st)) != 0 {
		t.Fatalf("news %v before anything happened", ev)
	}
	st.SetFlag("job", true)
	ev := b.Update(st)
	if len(ev) != 1 || ev[0].Kind != quest.Started || ev[0].Quest.ID != "slimes" || ev[0].Stage.ID != "clear" {
		t.Fatalf("after taking the job: %+v", ev)
	}
	if ev := b.Update(st); len(ev) != 0 {
		t.Fatalf("the same news twice: %+v", ev)
	}
	if !st.Flag("quest.slimes") {
		t.Error("no flag that the quest has started")
	}
	st.SetFlag("beat_1", true)
	if ev := b.Update(st); len(ev) != 0 {
		t.Fatalf("one slime of two is news: %+v", ev)
	}
	st.SetFlag("beat_2", true)
	ev = b.Update(st)
	if len(ev) != 1 || ev[0].Kind != quest.Advanced || ev[0].Stage.ID != "report" || !st.Flag("quest.slimes.clear") {
		t.Fatalf("after the second slime: %+v", ev)
	}
	j := b.Journal(st)
	if len(j) != 1 || j[0].Done || j[0].Stage.ID != "report" {
		t.Fatalf("journal %+v", j)
	}
	st.SetFlag("reported", true)
	ev = b.Update(st)
	if len(ev) != 1 || ev[0].Kind != quest.Completed || st.Count("coin") != 15 || !st.Flag("quest.slimes.done") {
		t.Fatalf("after reporting: %+v, %d coins", ev, st.Count("coin"))
	}
	if len(ev[0].Changes) != 2 {
		t.Errorf("changes %v, want the stage's 5 and the quest's 10", ev[0].Changes)
	}
	if j := b.Journal(st); len(j) != 1 || !j[0].Done || j[0].Stage != nil {
		t.Fatalf("journal when done: %+v", j)
	}
	if ev := b.Update(st); len(ev) != 0 || st.Count("coin") != 15 {
		t.Fatalf("a completed quest pays again: %+v, %d coins", ev, st.Count("coin"))
	}
}

// A stage done stays done when its condition stops holding, and stages
// whose conditions already hold are passed at once.
func TestStagesStayDone(t *testing.T) {
	b, st := load(t), story.New()
	st.SetFlag("asked", true)
	st.Give("crate", 1)
	ev := b.Update(st)
	if len(ev) != 2 || ev[0].Kind != quest.Started || ev[1].Kind != quest.Advanced || ev[1].Stage.ID != "back" {
		t.Fatalf("with the crate already in hand: %+v", ev)
	}
	st.Take("crate", 1) // dropped it somewhere
	if ev := b.Update(st); len(ev) != 0 {
		t.Fatalf("losing the crate is news: %+v", ev)
	}
	if j := b.Journal(st); j[0].Stage.ID != "back" {
		t.Fatalf("the journal went back to %q", j[0].Stage.ID)
	}
	// Under way before completed, in the journal.
	st.SetFlag("job", true)
	st.SetFlag("crate_back", true)
	b.Update(st)
	if j := b.Journal(st); len(j) != 2 || j[0].Quest.ID != "slimes" || !j[1].Done {
		t.Fatalf("journal %+v, want the slimes under way, then the crate done", j)
	}
}

func TestQuestsAreChecked(t *testing.T) {
	for name, y := range map[string]string{
		"no id":        "- {name: X, start: [flag a], stages: [{id: s, text: T, done: [flag b]}]}",
		"no name":      "- {id: x, start: [flag a], stages: [{id: s, text: T, done: [flag b]}]}",
		"no start":     "- {id: x, name: X, stages: [{id: s, text: T, done: [flag b]}]}",
		"no stages":    "- {id: x, name: X, start: [flag a]}",
		"stage done":   "- {id: x, name: X, start: [flag a], stages: [{id: done, text: T, done: [flag b]}]}",
		"no condition": "- {id: x, name: X, start: [flag a], stages: [{id: s, text: T}]}",
		"bad wording":  "- {id: x, name: X, start: [flagg a], stages: [{id: s, text: T, done: [flag b]}]}",
		"unknown item": "- {id: x, name: X, start: [flag a], stages: [{id: s, text: T, done: [item gem]}]}",
		"twice":        "- {id: x, name: X, start: [flag a], stages: [{id: s, text: T, done: [flag b]}]}\n- {id: x, name: Y, start: [flag a], stages: [{id: s, text: T, done: [flag b]}]}",
	} {
		if _, err := quest.Load([]byte(y), func(string) bool { return false }); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}
