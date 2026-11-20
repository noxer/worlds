package scene

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/story"
)

// The story in the scene: dialogs read and change it (flags and items,
// package story), the player is told what the party received or handed
// over, enemies beaten stay gone (a flag each), and all of it is saved.

// storyNote is the story as a dialog sees it, taking note of what changes
// hands.
type storyNote struct {
	st      *story.State
	changes []story.Change
}

func (n *storyNote) All(conds []string) bool { return n.st.All(conds) }

func (n *storyNote) Apply(effect string) {
	if c := n.st.Apply(effect); c.N != 0 {
		n.changes = append(n.changes, c)
	}
}

// converse opens a dialog that reads and changes the story; when it
// closes, the party is told what it received or handed over.
func (e *Explore) converse(ctx *Context, d *dialog.Dialog) {
	note := &storyNote{st: e.story}
	run := dialog.NewRunnerWith(d, note)
	ctx.Push(NewDialogBox(run, func() {
		e.dialogActions(run)
		if text := e.changesText(note.changes); text != "" {
			e.say(text)
		}
		e.updateQuests() // what was said may start or move on a quest
	}))
}

// itemName is the name of an item for the player.
func (e *Explore) itemName(id string) string {
	if it, ok := e.items[id]; ok {
		return it.Name
	}
	return id
}

// changesText says what was received and handed over.
func (e *Explore) changesText(cs []story.Change) string {
	var got, gave []string
	for _, c := range cs {
		n, list := c.N, &got
		if n < 0 {
			n, list = -n, &gave
		}
		name := e.itemName(c.Item)
		if n > 1 {
			name = fmt.Sprintf("%s x%d", name, n)
		}
		*list = append(*list, name)
	}
	var parts []string
	if len(got) > 0 {
		parts = append(parts, "Received: "+strings.Join(got, ", ")+".")
	}
	if len(gave) > 0 {
		parts = append(parts, "Handed over: "+strings.Join(gave, ", ")+".")
	}
	return strings.Join(parts, " ")
}

// itemList is the party's items for the menu, by name.
func (e *Explore) itemList() []itemEntry {
	var out []itemEntry
	for id, n := range e.story.Items() {
		out = append(out, itemEntry{id: id, name: e.itemName(id), n: n, text: e.items[id].Text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

type itemEntry struct {
	id, name, text string
	n              int
}

// defeatedFlag is the flag that an enemy has been beaten.
func defeatedFlag(n *NPC) string { return "defeated." + n.Name }

// applyDefeated takes the enemies beaten (by their flags) out of the
// world, and puts the others back: after a load.
func (e *Explore) applyDefeated() {
	for _, n := range e.npcs {
		if n.enemy != "" {
			n.defeated = e.story.Flag(defeatedFlag(n))
			n.active = !n.defeated
		}
	}
}
