package scene

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// PartyScreen shows the party, opened from the pause menu: each member
// with their portrait, the first in line leading. Pick a member, then
// another, to swap them (with keys or taps); whoever ends up first leads.
// Cancel (or the close button) goes back.
type PartyScreen struct {
	e      *Explore
	cursor int
	picked int // the member picked first, -1 for none
	w, h   int
}

const partyRowH = 20

// NewPartyScreen opens the party screen.
func NewPartyScreen(e *Explore) *PartyScreen { return &PartyScreen{e: e, picked: -1} }

// rects returns the panel and a row per member.
func (s *PartyScreen) rects() (box image.Rectangle, rows []image.Rectangle) {
	n := len(s.e.party.Members)
	w := 0
	for _, m := range s.e.party.Members {
		w = max(w, gfx.TextWidth(m.Name+"  leads"))
	}
	w = max(w+2*boxPad+28, gfx.TextWidth(partyHint)+2*boxPad)
	h := gfx.LineHeight + 4 + n*partyRowH + gfx.LineHeight + 2*boxPad
	box = image.Rect(boxMargin, boxMargin, boxMargin+w, boxMargin+h)
	for i := range n {
		y := box.Min.Y + boxPad + gfx.LineHeight + 4 + i*partyRowH
		rows = append(rows, image.Rect(box.Min.X+2, y, box.Max.X-2, y+partyRowH))
	}
	return box, rows
}

const partyHint = "Pick two to swap them."

// choose picks member i, or swaps it with the one picked before.
func (s *PartyScreen) choose(i int) {
	s.cursor = i
	switch {
	case s.picked < 0:
		s.picked = i
	case s.picked == i:
		s.picked = -1
	default:
		s.swap(s.picked, i)
		s.picked = -1
	}
}

// swap exchanges two members. If the leader changes, the party gathers
// on the leader's tile and unfolds again behind the new one (as after a
// door).
func (s *PartyScreen) swap(a, b int) {
	ms := s.e.party.Members
	l := ms[0]
	pos, facing := l.Pos, l.Facing
	ms[a], ms[b] = ms[b], ms[a]
	if a == 0 || b == 0 {
		s.e.party.PlaceAll(pos, facing)
	}
}

// Update implements Scene.
func (s *PartyScreen) Update(ctx *Context) {
	in := ctx.Input
	s.w, s.h = ctx.W, ctx.H
	n := len(s.e.party.Members)
	if in.Pressed(input.Cancel) || in.Pressed(input.Menu) {
		if s.picked >= 0 {
			s.picked = -1
			return
		}
		ctx.Pop()
		return
	}
	if p, ok := in.Tap(); ok {
		if p.In(cornerButton(s.w, 0)) {
			ctx.Pop()
			return
		}
		box, rows := s.rects()
		for i, r := range rows {
			if p.In(r) {
				s.choose(i)
				return
			}
		}
		if !p.In(box) {
			ctx.Pop()
		}
		return
	}
	if in.Pressed(input.Up) {
		s.cursor = (s.cursor + n - 1) % n
	}
	if in.Pressed(input.Down) {
		s.cursor = (s.cursor + 1) % n
	}
	if in.Pressed(input.Confirm) {
		s.choose(s.cursor)
	}
}

// Draw implements Scene.
func (s *PartyScreen) Draw(dst *ebiten.Image) {
	box, rows := s.rects()
	gfx.Box(dst, box)
	gfx.Text(dst, "Party", box.Min.X+boxPad, box.Min.Y+boxPad-1, gfx.ColorAccent)
	for i, m := range s.e.party.Members {
		r := rows[i]
		if i == s.picked {
			gfx.FillRect(dst, r, gfx.ColorMuted)
		}
		col := gfx.ColorInk
		if i == s.cursor {
			col = gfx.ColorAccent
			gfx.Text(dst, ">", r.Min.X+boxPad-4, r.Min.Y+3, col)
		}
		// The portrait: the member's standing sprite, facing down.
		if img := s.portrait(m); img != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(r.Min.X+boxPad+6), float64(r.Min.Y+2))
			dst.DrawImage(img, op)
		}
		name := m.Name
		if i == 0 {
			name += "  leads"
		}
		gfx.Text(dst, name, r.Min.X+boxPad+26, r.Min.Y+3, col)
	}
	gfx.Text(dst, partyHint, box.Min.X+boxPad, box.Max.Y-boxPad-gfx.LineHeight+1, gfx.ColorMuted)
	drawButton(dst, cornerButton(dst.Bounds().Dx(), 0), iconClose)
}

// portrait returns a member's sprite standing, facing down.
func (s *PartyScreen) portrait(a *entity.Actor) *ebiten.Image {
	if a.Sprite.Sheet == "" || s.e.sheets == nil {
		return nil
	}
	idx := a.Sprite.Index
	if a.Sprite.Walk != nil {
		idx = a.Sprite.Walk[world.Down][0]
	}
	return s.e.sheets.Tile(a.Sprite.Sheet, idx)
}
