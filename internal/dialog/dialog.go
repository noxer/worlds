// Package dialog loads dialog trees from YAML and steps through them.
//
// A dialog file maps dialog ids to trees:
//
//	elder:
//	  start: hello
//	  nodes:
//	    hello:
//	      speaker: Elder Maren
//	      lines: ["Welcome!", "What brings you here?"]
//	      choices:
//	        - {text: "Work.", next: work}
//	        - {text: "Nothing."}          # no next: ends the dialog
//	    work:
//	      lines: ["Talk to the smith."]   # no choices and no next: ends
//
// Dialogs can read and change the story (flags and items; the wording of
// conditions and effects is package story's):
//
//	smith:
//	  start: hello                      # unless one of the starts applies
//	  starts:
//	    - {if: [flag crate_back], node: thanks}
//	    - {if: [item crate], node: handover}
//	  nodes:
//	    hello:
//	      lines: ["My delivery never came."]
//	      effects: [set smith_asked]    # when the node is reached
//	      choices:
//	        - {text: "We'll look.", effects: [set looking]}
//	        - {text: "About the cellar...", if: [flag found_cellar], next: cellar}
//
// A choice with conditions is only offered while they all hold.
package dialog

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Choice is a player answer. Do names an action the game carries out when
// it is chosen (dialogs built by the game, such as a station's timetable,
// use it; the game decides what the names mean).
type Choice struct {
	Text    string   `yaml:"text"`
	Next    string   `yaml:"next"`
	Do      string   `yaml:"do"`
	If      []string `yaml:"if"`      // conditions: offered only while all hold
	Effects []string `yaml:"effects"` // carried out when it is chosen
}

// Node is one step of a conversation.
type Node struct {
	Speaker string   `yaml:"speaker"`
	Lines   []string `yaml:"lines"`
	Choices []Choice `yaml:"choices"`
	Next    string   `yaml:"next"`
	Effects []string `yaml:"effects"` // carried out when the node is reached
}

// Start is a conditional first node.
type Start struct {
	If   []string `yaml:"if"`
	Node string   `yaml:"node"`
}

// State is the story a dialog reads and changes (package story's State
// does; so can a wrapper that takes note of what changes).
type State interface {
	All(conditions []string) bool
	Apply(effect string)
}

// Dialog is a conversation tree.
type Dialog struct {
	ID     string           `yaml:"-"`
	Start  string           `yaml:"start"`
	Starts []Start          `yaml:"starts"` // the first whose conditions hold, else Start
	Nodes  map[string]*Node `yaml:"nodes"`
}

// Conditions returns every condition the dialog uses, and Effects every
// effect, for checking their wording.
func (d *Dialog) Conditions() []string {
	var out []string
	for _, s := range d.Starts {
		out = append(out, s.If...)
	}
	for _, id := range d.nodeIDs() {
		for _, c := range d.Nodes[id].Choices {
			out = append(out, c.If...)
		}
	}
	return out
}

// Effects returns every effect the dialog uses.
func (d *Dialog) Effects() []string {
	var out []string
	for _, id := range d.nodeIDs() {
		n := d.Nodes[id]
		out = append(out, n.Effects...)
		for _, c := range n.Choices {
			out = append(out, c.Effects...)
		}
	}
	return out
}

// Actions returns the actions (Do) of every choice, in node order.
func (d *Dialog) Actions() []string {
	var out []string
	for _, id := range d.nodeIDs() {
		for _, c := range d.Nodes[id].Choices {
			if c.Do != "" {
				out = append(out, c.Do)
			}
		}
	}
	return out
}

func (d *Dialog) nodeIDs() []string {
	ids := make([]string, 0, len(d.Nodes))
	for id := range d.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Library holds all dialogs by id.
type Library map[string]*Dialog

// Load reads all *.yaml files in dir and validates node references.
func Load(fsys fs.FS, dir string) (Library, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	lib := Library{}
	var errs []error
	for _, e := range entries {
		if path.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := path.Join(dir, e.Name())
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var file map[string]*Dialog
		if err := yaml.Unmarshal(b, &file); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for id, d := range file {
			if _, dup := lib[id]; dup {
				errs = append(errs, fmt.Errorf("%s: duplicate dialog id %q", name, id))
				continue
			}
			d.ID = id
			if err := d.validate(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
			lib[id] = d
		}
	}
	return lib, errors.Join(errs...)
}

func (d *Dialog) validate() error {
	if d.Nodes[d.Start] == nil {
		return fmt.Errorf("dialog %q: start node %q does not exist", d.ID, d.Start)
	}
	var errs []error
	for _, st := range d.Starts {
		if d.Nodes[st.Node] == nil {
			errs = append(errs, fmt.Errorf("dialog %q: starts refer to unknown node %q", d.ID, st.Node))
		}
		if len(st.If) == 0 {
			errs = append(errs, fmt.Errorf("dialog %q: the start at node %q has no conditions", d.ID, st.Node))
		}
	}
	for _, id := range d.nodeIDs() {
		n := d.Nodes[id]
		if len(n.Lines) == 0 {
			errs = append(errs, fmt.Errorf("dialog %q: node %q has no lines", d.ID, id))
		}
		if n.Next != "" && len(n.Choices) > 0 {
			errs = append(errs, fmt.Errorf("dialog %q: node %q has both next and choices", d.ID, id))
		}
		refs := []string{n.Next}
		for _, c := range n.Choices {
			refs = append(refs, c.Next)
		}
		for _, r := range refs {
			if r != "" && d.Nodes[r] == nil {
				errs = append(errs, fmt.Errorf("dialog %q: node %q refers to unknown node %q", d.ID, id, r))
			}
		}
	}
	return errors.Join(errs...)
}

// Runner walks through a dialog one line at a time.
type Runner struct {
	d     *Dialog
	state State // nil: no story (conditions never hold, effects do nothing)
	node  *Node
	line  int
	do    []string // actions of the choices made
}

// Actions returns the actions of the choices made so far, in order.
func (r *Runner) Actions() []string { return r.do }

// NewRunner starts a dialog at its start node, without a story: choices
// with conditions aren't offered, effects do nothing.
func NewRunner(d *Dialog) *Runner { return NewRunnerWith(d, nil) }

// NewRunnerWith starts a dialog that reads and changes the story st: at
// the first of its starts whose conditions hold, else at its start node.
func NewRunnerWith(d *Dialog, st State) *Runner {
	r := &Runner{d: d, state: st}
	start := d.Start
	if st != nil {
		for _, s := range d.Starts {
			if st.All(s.If) {
				start = s.Node
				break
			}
		}
	}
	r.goTo(start)
	return r
}

// Done reports whether the conversation is over.
func (r *Runner) Done() bool { return r.node == nil }

// Speaker returns the current speaker.
func (r *Runner) Speaker() string { return r.node.Speaker }

// Line returns the current line of text.
func (r *Runner) Line() string { return r.node.Lines[r.line] }

// Choices returns the choices to show with the current line. They are only
// offered on the last line of a node.
func (r *Runner) Choices() []Choice {
	if r.line != len(r.node.Lines)-1 {
		return nil
	}
	var out []Choice
	for _, c := range r.node.Choices {
		if len(c.If) == 0 || (r.state != nil && r.state.All(c.If)) {
			out = append(out, c)
		}
	}
	return out
}

// Advance moves to the next line. It must not be called while choices are
// shown; use Choose instead.
func (r *Runner) Advance() {
	if r.Done() || len(r.Choices()) > 0 {
		return
	}
	if r.line < len(r.node.Lines)-1 {
		r.line++
		return
	}
	r.goTo(r.node.Next)
}

// Choose picks choice i on the current line.
func (r *Runner) Choose(i int) {
	cs := r.Choices()
	if i < 0 || i >= len(cs) {
		return
	}
	if cs[i].Do != "" {
		r.do = append(r.do, cs[i].Do)
	}
	r.apply(cs[i].Effects)
	r.goTo(cs[i].Next)
}

func (r *Runner) goTo(id string) {
	r.line = 0
	if id == "" {
		r.node = nil
		return
	}
	r.node = r.d.Nodes[id]
	r.apply(r.node.Effects)
}

func (r *Runner) apply(effects []string) {
	if r.state == nil {
		return
	}
	for _, e := range effects {
		r.state.Apply(e)
	}
}
