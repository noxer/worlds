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
// Conditions and effects (flags, quests, skill checks) hook in here later.
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
	Text string `yaml:"text"`
	Next string `yaml:"next"`
	Do   string `yaml:"do"`
}

// Node is one step of a conversation.
type Node struct {
	Speaker string   `yaml:"speaker"`
	Lines   []string `yaml:"lines"`
	Choices []Choice `yaml:"choices"`
	Next    string   `yaml:"next"`
}

// Dialog is a conversation tree.
type Dialog struct {
	ID    string           `yaml:"-"`
	Start string           `yaml:"start"`
	Nodes map[string]*Node `yaml:"nodes"`
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
	ids := make([]string, 0, len(d.Nodes))
	for id := range d.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var errs []error
	for _, id := range ids {
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
	d    *Dialog
	node *Node
	line int
	do   []string // actions of the choices made
}

// Actions returns the actions of the choices made so far, in order.
func (r *Runner) Actions() []string { return r.do }

// NewRunner starts a dialog at its start node.
func NewRunner(d *Dialog) *Runner { return &Runner{d: d, node: d.Nodes[d.Start]} }

// Done reports whether the conversation is over.
func (r *Runner) Done() bool { return r.node == nil }

// Speaker returns the current speaker.
func (r *Runner) Speaker() string { return r.node.Speaker }

// Line returns the current line of text.
func (r *Runner) Line() string { return r.node.Lines[r.line] }

// Choices returns the choices to show with the current line. They are only
// offered on the last line of a node.
func (r *Runner) Choices() []Choice {
	if r.line == len(r.node.Lines)-1 {
		return r.node.Choices
	}
	return nil
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
	r.goTo(cs[i].Next)
}

func (r *Runner) goTo(id string) {
	r.line = 0
	if id == "" {
		r.node = nil
		return
	}
	r.node = r.d.Nodes[id]
}
