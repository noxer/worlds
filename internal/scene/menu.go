package scene

import (
	"fmt"
	"image"
	"image/color"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/sim"
)

// Menu is the pause menu, opened with Menu (or Cancel) while exploring:
// Resume, Map (the world map), Party, Quests (the journal), Items, Save,
// Load and, on desktop, Quit. The world stands still while
// it is open. Save and Load list the slots with where and when.
type Menu struct {
	e      *Explore
	store  save.Store
	cursor int
	page   menuPage
	slots  []*save.State // the slot list, read when a slot page opens
	msg    string        // a message under the menu (an error)
	w, h   int
}

type menuPage int

const (
	menuMain menuPage = iota
	menuSave
	menuLoad
	menuItems
	menuQuests
)

type menuItem struct {
	text  string
	do    func(m *Menu, ctx *Context)
	off   bool   // shown greyed out, can't be chosen
	about string // shown under the menu while the cursor is on it
}

// NewMenu creates the pause menu over the exploration scene.
func NewMenu(e *Explore, store save.Store) *Menu { return &Menu{e: e, store: store} }

func (m *Menu) items() []menuItem {
	switch m.page {
	case menuQuests:
		var out []menuItem
		for _, q := range m.e.quests.Journal(m.e.story) {
			it := menuItem{text: q.Quest.Name, about: "Completed."}
			if q.Done {
				it.text += " (done)"
			} else if q.Stage != nil {
				it.about = q.Stage.Text
			}
			out = append(out, it)
		}
		if len(out) == 0 {
			out = append(out, menuItem{text: "- none yet -", off: true})
		}
		return append(out, menuItem{text: "Back", do: func(m *Menu, ctx *Context) { m.open(menuMain) }})
	case menuItems:
		var out []menuItem
		for _, it := range m.e.itemList() {
			text, about := it.name, it.text
			if it.n > 1 {
				text = fmt.Sprintf("%s x%d", it.name, it.n)
			}
			out = append(out, menuItem{text: text, about: about})
		}
		if len(out) == 0 {
			out = append(out, menuItem{text: "- nothing -", off: true})
		}
		return append(out, menuItem{text: "Back", do: func(m *Menu, ctx *Context) { m.open(menuMain) }})
	case menuSave, menuLoad:
		var out []menuItem
		for k, s := range m.slots {
			k := k
			text := fmt.Sprintf("%d  - empty -", k+1)
			if s != nil {
				text = fmt.Sprintf("%d  %s  %s", k+1, sim.ClockAt(s.Ticks), s.Place)
			}
			if m.page == menuSave {
				out = append(out, menuItem{text: text, do: func(m *Menu, ctx *Context) { m.saveTo(ctx, k) }})
			} else {
				out = append(out, menuItem{text: text, off: s == nil, do: func(m *Menu, ctx *Context) { m.loadFrom(ctx, k) }})
			}
		}
		return append(out, menuItem{text: "Back", do: func(m *Menu, ctx *Context) { m.open(menuMain) }})
	}
	out := []menuItem{
		{text: "Resume", do: func(m *Menu, ctx *Context) { ctx.Pop() }},
		{text: "Map", do: func(m *Menu, ctx *Context) { ctx.Pop(); ctx.Push(NewWorldMap(m.e)) }},
		{text: "Party", do: func(m *Menu, ctx *Context) { ctx.Pop(); ctx.Push(NewPartyScreen(m.e)) }},
		{text: "Quests", do: func(m *Menu, ctx *Context) { m.open(menuQuests) }},
		{text: "Items", do: func(m *Menu, ctx *Context) { m.open(menuItems) }},
		{text: "Save", off: m.store == nil, do: func(m *Menu, ctx *Context) { m.open(menuSave) }},
		{text: "Load", off: m.store == nil, do: func(m *Menu, ctx *Context) { m.open(menuLoad) }},
	}
	if runtime.GOOS != "js" { // a browser tab is closed, not quit
		out = append(out, menuItem{text: "Quit", do: func(m *Menu, ctx *Context) { ctx.Quit() }})
	}
	return out
}

// open switches to a page.
func (m *Menu) open(p menuPage) {
	m.page, m.cursor, m.msg = p, 0, ""
	if p == menuItems || p == menuQuests {
		if its := m.items(); its[0].off {
			m.cursor = 1 // nothing to list: on Back
		}
		return
	}
	if p != menuMain {
		m.slots = save.List(m.store)
		if p == menuLoad {
			for k, s := range m.slots { // start on the newest save
				if s != nil && (m.slots[m.cursor] == nil || s.Saved.After(m.slots[m.cursor].Saved)) {
					m.cursor = k
				}
			}
		}
	}
}

func (m *Menu) saveTo(ctx *Context, slot int) {
	st, err := m.e.Snapshot()
	if err == nil {
		var data []byte
		if data, err = save.Encode(st); err == nil {
			err = m.store.Save(slot, data)
		}
	}
	if err != nil {
		m.msg = capitalize(err.Error()) + "."
		return
	}
	ctx.Pop()
	m.e.say(fmt.Sprintf("Saved in slot %d.", slot+1))
}

func (m *Menu) loadFrom(ctx *Context, slot int) {
	data, err := m.store.Load(slot)
	var st save.State
	if err == nil {
		if st, err = save.Decode(data); err == nil {
			err = m.e.Restore(st)
		}
	}
	if err != nil {
		m.msg = capitalize(err.Error()) + "."
		return
	}
	ctx.Pop()
	m.e.pendingNotice = fmt.Sprintf("Loaded slot %d.", slot+1)
}

func capitalize(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// rects returns the menu's box and its rows.
func (m *Menu) rects() (box image.Rectangle, rows []image.Rectangle) {
	its := m.items()
	w := gfx.TextWidth("Menu")
	for _, it := range its {
		w = max(w, gfx.TextWidth(it.text))
	}
	w += 2*boxPad + 8
	h := (len(its)+1)*gfx.LineHeight + 2*boxPad + 4
	box = image.Rect(boxMargin, boxMargin, boxMargin+w, boxMargin+h)
	for i := range its {
		y := box.Min.Y + boxPad + gfx.LineHeight + 4 + i*gfx.LineHeight
		rows = append(rows, image.Rect(box.Min.X, y, box.Max.X, y+gfx.LineHeight))
	}
	return box, rows
}

// Update implements Scene.
func (m *Menu) Update(ctx *Context) {
	in := ctx.Input
	m.w, m.h = ctx.W, ctx.H
	its := m.items()
	if in.Pressed(input.Cancel) || in.Pressed(input.Menu) {
		if m.page != menuMain {
			m.open(menuMain)
		} else {
			ctx.Pop()
		}
		return
	}
	step := func(d int) {
		for range its {
			m.cursor = (m.cursor + d + len(its)) % len(its)
			if !its[m.cursor].off {
				return
			}
		}
	}
	if in.Pressed(input.Up) {
		step(-1)
	}
	if in.Pressed(input.Down) {
		step(1)
	}
	if p, ok := in.Tap(); ok {
		box, rows := m.rects()
		for i, r := range rows {
			if p.In(r) && !its[i].off {
				m.cursor = i
				if its[i].do != nil {
					its[i].do(m, ctx)
				}
				return
			}
		}
		if !p.In(box) { // a tap beside the menu goes back, or closes it
			if m.page != menuMain {
				m.open(menuMain)
			} else {
				ctx.Pop()
			}
			return
		}
	}
	if in.Pressed(input.Confirm) && m.cursor < len(its) && !its[m.cursor].off && its[m.cursor].do != nil {
		its[m.cursor].do(m, ctx)
	}
}

// Draw implements Scene.
func (m *Menu) Draw(dst *ebiten.Image) {
	box, rows := m.rects()
	gfx.Box(dst, box)
	title := map[menuPage]string{menuMain: "Menu", menuSave: "Save to", menuLoad: "Load from", menuItems: "Items", menuQuests: "Quests"}[m.page]
	gfx.Text(dst, title, box.Min.X+boxPad, box.Min.Y+boxPad-1, gfx.ColorAccent)
	for i, it := range m.items() {
		r := rows[i]
		col := gfx.ColorInk
		switch {
		case it.off:
			col = gfx.ColorMuted
		case i == m.cursor:
			col = gfx.ColorAccent
			gfx.Text(dst, ">", r.Min.X+boxPad-2, r.Min.Y-1, col)
		}
		gfx.Text(dst, it.text, r.Min.X+boxPad+6, r.Min.Y-1, col)
	}
	// Under the menu: a message, or what the entry under the cursor is.
	msg, col := m.msg, color.Color(gfx.ColorAccent)
	if its := m.items(); msg == "" && m.cursor < len(its) {
		msg, col = its[m.cursor].about, gfx.ColorInk
	}
	if msg != "" {
		lines := gfx.Wrap(msg, dst.Bounds().Dx()-2*boxMargin-2*boxPad)
		w := 0
		for _, l := range lines {
			w = max(w, gfx.TextWidth(l))
		}
		r := image.Rect(box.Min.X, box.Max.Y+2, box.Min.X+w+2*boxPad, box.Max.Y+2+len(lines)*gfx.LineHeight+2*boxPad-2)
		gfx.Box(dst, r)
		for i, l := range lines {
			gfx.Text(dst, l, r.Min.X+boxPad, r.Min.Y+boxPad-2+i*gfx.LineHeight, col)
		}
	}
}
