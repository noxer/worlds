// Package quest tracks quests: each is a list of stages, the story (flags
// and items, package story) says when it starts and when a stage is done.
//
//   - id: meadow_slimes
//     name: Slimes in the meadow
//     start: [flag slime_job]                 # begins when these hold
//     stages:
//   - id: clear
//     text: Clear the slimes out of the east field of Willow Meadow.
//     done: [flag defeated.slime_1, flag defeated.slime_2]
//   - id: report
//     text: Tell Elder Maren the field is clear.
//     done: [flag slime_job_done]
//     effects: [give coin 5]              # when the stage is done
//     effects: [give coin 10]                 # when the quest is done
//
// Progress is kept in the story as flags, so it is saved with it and
// dialogs can ask about it: "quest.<id>" once it has started,
// "quest.<id>.<stage>" for each stage done, "quest.<id>.done" at the end.
// A stage done stays done, whatever happens to its conditions later (an
// item handed over, say).
package quest

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/noxer/worlds/internal/story"
)

// Stage is a step of a quest.
type Stage struct {
	ID      string   `yaml:"id"`
	Text    string   `yaml:"text"`    // the objective, for the journal
	Done    []string `yaml:"done"`    // conditions: done when all hold
	Effects []string `yaml:"effects"` // carried out when it is done
}

// Quest is a quest.
type Quest struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Start   []string `yaml:"start"` // conditions: it begins when all hold
	Stages  []Stage  `yaml:"stages"`
	Effects []string `yaml:"effects"` // carried out when it is completed
}

// Book is all the quests, in the order of the file.
type Book []*Quest

// Flags of a quest's progress in the story.
func (q *Quest) startedFlag() string       { return "quest." + q.ID }
func (q *Quest) doneFlag() string          { return "quest." + q.ID + ".done" }
func (q *Quest) stageFlag(s *Stage) string { return "quest." + q.ID + "." + s.ID }

// Load reads and checks quests from YAML. known reports whether an item
// exists (nil: any does).
func Load(data []byte, known func(item string) bool) (Book, error) {
	var b Book
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("quests: %w", err)
	}
	seen := map[string]bool{}
	for _, q := range b {
		switch {
		case q.ID == "":
			return nil, fmt.Errorf("quests: a quest has no id")
		case seen[q.ID]:
			return nil, fmt.Errorf("quests: two quests have the id %q", q.ID)
		case q.Name == "":
			return nil, fmt.Errorf("quest %q has no name", q.ID)
		case len(q.Start) == 0:
			return nil, fmt.Errorf("quest %q has no start conditions", q.ID)
		case len(q.Stages) == 0:
			return nil, fmt.Errorf("quest %q has no stages", q.ID)
		}
		seen[q.ID] = true
		conds, effects := append([]string(nil), q.Start...), append([]string(nil), q.Effects...)
		stages := map[string]bool{}
		for i := range q.Stages {
			s := &q.Stages[i]
			switch {
			case s.ID == "" || s.ID == "done":
				return nil, fmt.Errorf("quest %q: stage %d has the id %q", q.ID, i+1, s.ID)
			case stages[s.ID]:
				return nil, fmt.Errorf("quest %q: two stages have the id %q", q.ID, s.ID)
			case s.Text == "":
				return nil, fmt.Errorf("quest %q: stage %q has no text", q.ID, s.ID)
			case len(s.Done) == 0:
				return nil, fmt.Errorf("quest %q: stage %q has no conditions", q.ID, s.ID)
			}
			stages[s.ID] = true
			conds, effects = append(conds, s.Done...), append(effects, s.Effects...)
		}
		check := func(what, item string, err error) error {
			if err != nil {
				return fmt.Errorf("quest %q: %s %w", q.ID, what, err)
			}
			if item != "" && known != nil && !known(item) {
				return fmt.Errorf("quest %q: unknown item %q", q.ID, item)
			}
			return nil
		}
		for _, c := range conds {
			item, err := story.CheckCondition(c)
			if err := check("condition", item, err); err != nil {
				return nil, err
			}
		}
		for _, e := range effects {
			item, err := story.CheckEffect(e)
			if err := check("effect", item, err); err != nil {
				return nil, err
			}
		}
	}
	return b, nil
}

// EventKind is what happened to a quest.
type EventKind uint8

const (
	Started EventKind = iota
	Advanced
	Completed
)

// Event is news about a quest, for telling the player.
type Event struct {
	Kind    EventKind
	Quest   *Quest
	Stage   *Stage         // Started, Advanced: the stage now to do
	Changes []story.Change // items the effects gave or took
}

// Update moves the quests on by the story as it stands: starts those
// whose time has come, marks stages done (carrying out their effects) and
// completes quests. It returns the news, in order; nothing if nothing
// changed. Call it whenever the story may have changed.
func (b Book) Update(st *story.State) []Event {
	var out []Event
	apply := func(effects []string) []story.Change {
		var cs []story.Change
		for _, e := range effects {
			if c := st.Apply(e); c.N != 0 {
				cs = append(cs, c)
			}
		}
		return cs
	}
	for _, q := range b {
		if st.Flag(q.doneFlag()) {
			continue
		}
		if !st.Flag(q.startedFlag()) {
			if !st.All(q.Start) {
				continue
			}
			st.SetFlag(q.startedFlag(), true)
			out = append(out, Event{Kind: Started, Quest: q, Stage: &q.Stages[0]})
		}
		for i := range q.Stages {
			s := &q.Stages[i]
			if st.Flag(q.stageFlag(s)) {
				continue
			}
			if !st.All(s.Done) {
				break // this is the stage to do
			}
			st.SetFlag(q.stageFlag(s), true)
			changes := apply(s.Effects)
			if i+1 < len(q.Stages) {
				out = append(out, Event{Kind: Advanced, Quest: q, Stage: &q.Stages[i+1], Changes: changes})
				continue
			}
			st.SetFlag(q.doneFlag(), true)
			out = append(out, Event{Kind: Completed, Quest: q, Changes: append(changes, apply(q.Effects)...)})
		}
	}
	return out
}

// Status is where a quest stands, for the journal.
type Status struct {
	Quest *Quest
	Done  bool
	Stage *Stage // the stage to do; nil when done
}

// Journal lists the quests that have started: those under way first, then
// the completed ones, each group in the order of the file.
func (b Book) Journal(st *story.State) []Status {
	var active, done []Status
	for _, q := range b {
		switch {
		case st.Flag(q.doneFlag()):
			done = append(done, Status{Quest: q, Done: true})
		case st.Flag(q.startedFlag()):
			s := Status{Quest: q}
			for i := range q.Stages {
				if !st.Flag(q.stageFlag(&q.Stages[i])) {
					s.Stage = &q.Stages[i]
					break
				}
			}
			active = append(active, s)
		}
	}
	return append(active, done...)
}
