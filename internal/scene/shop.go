package scene

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/story"
)

// Shop is a shop's counter, opened by a dialog with the action
// "shop <id>" (shops.yaml): the shop's goods to buy, one at a time, at
// their price; or the party's items to sell, for half their price. Coins
// are the item "coin". Items without a price (coins, quest items) can't
// be sold.
type Shop struct {
	e       *Explore
	shop    story.Shop
	selling bool
	cursor  int
	msg     string
}

// NewShop opens a shop.
func NewShop(e *Explore, shop story.Shop) *Shop { return &Shop{e: e, shop: shop} }

type shopRow struct {
	text, price string
	about       string
	off         bool
	do          func(s *Shop, ctx *Context)
}

func (s *Shop) rows() []shopRow {
	var out []shopRow
	if !s.selling {
		for _, id := range s.shop.Sells {
			it := s.e.items[id]
			about := it.Text
			if n := s.e.story.Count(id); n > 0 {
				about += fmt.Sprintf(" You have %d.", n)
			}
			out = append(out, shopRow{text: it.Name, price: fmt.Sprint(it.Price), about: about,
				do: func(s *Shop, ctx *Context) { s.buy(id) }})
		}
		out = append(out, shopRow{text: "Sell...", about: "Sell what you carry, for half its price.",
			do: func(s *Shop, ctx *Context) { s.switchTo(true) }})
	} else {
		for _, it := range s.e.itemList() {
			p := s.e.items[it.id].SellPrice()
			if p == 0 {
				continue
			}
			text := it.name
			if it.n > 1 {
				text = fmt.Sprintf("%s x%d", it.name, it.n)
			}
			id := it.id
			out = append(out, shopRow{text: text, price: fmt.Sprint(p), about: it.text,
				do: func(s *Shop, ctx *Context) { s.sell(id) }})
		}
		if len(out) == 0 {
			out = append(out, shopRow{text: "- nothing to sell -", off: true})
		}
		out = append(out, shopRow{text: "Buy...", about: "Back to the shop's goods.",
			do: func(s *Shop, ctx *Context) { s.switchTo(false) }})
	}
	return append(out, shopRow{text: "Leave", do: func(s *Shop, ctx *Context) { ctx.Pop() }})
}

func (s *Shop) switchTo(selling bool) {
	s.selling, s.cursor, s.msg = selling, 0, ""
	if rs := s.rows(); rs[0].off {
		s.cursor = 1
	}
}

func (s *Shop) buy(id string) {
	it := s.e.items[id]
	if !s.e.story.Take("coin", it.Price) {
		s.msg = "You can't afford that."
		return
	}
	s.e.story.Give(id, 1)
	s.msg = "Bought: " + it.Name + "."
}

func (s *Shop) sell(id string) {
	it := s.e.items[id]
	if !s.e.story.Take(id, 1) {
		return
	}
	s.e.story.Give("coin", it.SellPrice())
	s.msg = fmt.Sprintf("Sold: %s, for %d coins.", it.Name, it.SellPrice())
	if rs := s.rows(); s.cursor >= len(rs) || rs[s.cursor].off || rs[s.cursor].do == nil {
		s.cursor = max(0, s.cursor-1)
	}
	if rs := s.rows(); rs[s.cursor].off {
		s.cursor++
	}
}

func (s *Shop) title() string {
	t := s.shop.Name
	if s.selling {
		t += ": selling"
	}
	return t
}

func (s *Shop) coins() string { return fmt.Sprintf("Coins %d", s.e.story.Count("coin")) }

// rects returns the panel and its rows.
func (s *Shop) rects() (box image.Rectangle, rows []image.Rectangle) {
	rs := s.rows()
	w := gfx.TextWidth(s.title()) + 12 + gfx.TextWidth(s.coins())
	for _, r := range rs {
		w = max(w, gfx.TextWidth(r.text)+12+gfx.TextWidth(r.price)+6)
	}
	w += 2*boxPad + 8
	h := (len(rs)+1)*gfx.LineHeight + 2*boxPad + 4
	box = image.Rect(boxMargin, boxMargin, boxMargin+w, boxMargin+h)
	for i := range rs {
		y := box.Min.Y + boxPad + gfx.LineHeight + 4 + i*gfx.LineHeight
		rows = append(rows, image.Rect(box.Min.X, y, box.Max.X, y+gfx.LineHeight))
	}
	return box, rows
}

// Update implements Scene.
func (s *Shop) Update(ctx *Context) {
	in := ctx.Input
	rs := s.rows()
	if in.Pressed(input.Cancel) || in.Pressed(input.Menu) {
		ctx.Pop()
		return
	}
	step := func(d int) {
		for range rs {
			s.cursor = (s.cursor + d + len(rs)) % len(rs)
			if !rs[s.cursor].off {
				return
			}
		}
	}
	if in.Pressed(input.Up) {
		step(-1)
		s.msg = ""
	}
	if in.Pressed(input.Down) {
		step(1)
		s.msg = ""
	}
	if p, ok := in.Tap(); ok {
		box, rows := s.rects()
		for i, r := range rows {
			if p.In(r) && !rs[i].off {
				s.cursor = i
				rs[i].do(s, ctx)
				return
			}
		}
		if !p.In(box) {
			ctx.Pop()
		}
		return
	}
	if in.Pressed(input.Confirm) && s.cursor < len(rs) && !rs[s.cursor].off {
		rs[s.cursor].do(s, ctx)
	}
}

// Draw implements Scene.
func (s *Shop) Draw(dst *ebiten.Image) {
	box, rows := s.rects()
	gfx.Box(dst, box)
	gfx.Text(dst, s.title(), box.Min.X+boxPad, box.Min.Y+boxPad-1, gfx.ColorAccent)
	gfx.Text(dst, s.coins(), box.Max.X-boxPad-gfx.TextWidth(s.coins()), box.Min.Y+boxPad-1, gfx.ColorInk)
	rs := s.rows()
	for i, r := range rs {
		row := rows[i]
		col := gfx.ColorInk
		switch {
		case r.off:
			col = gfx.ColorMuted
		case i == s.cursor:
			col = gfx.ColorAccent
			gfx.Text(dst, ">", row.Min.X+boxPad-2, row.Min.Y-1, col)
		}
		gfx.Text(dst, r.text, row.Min.X+boxPad+6, row.Min.Y-1, col)
		if r.price != "" {
			gfx.Text(dst, r.price, row.Max.X-boxPad-gfx.TextWidth(r.price), row.Min.Y-1, col)
		}
	}
	// Under the panel: what just happened, or what the row is.
	msg, col := s.msg, color.Color(gfx.ColorAccent)
	if msg == "" && s.cursor < len(rs) {
		msg, col = rs[s.cursor].about, gfx.ColorInk
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
