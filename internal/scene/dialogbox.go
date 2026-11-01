package scene

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
)

const (
	boxMargin    = 4
	boxPad       = 6
	boxRows      = 3
	charsPerTick = 1
)

// DialogBox shows a conversation with a typewriter effect, paging and
// choices. Confirm or a tap advances; Up/Down (or a tap) selects choices.
type DialogBox struct {
	r      *dialog.Runner
	onDone func()

	w, h    int      // screen size at layout time
	page    []string // wrapped rows of the current page
	pages   [][]string
	pageIdx int
	shown   int // characters revealed on the current page
	cursor  int
	laidOut string // line the pages were built for
}

// NewDialogBox creates a dialog box. onDone runs after it closes.
func NewDialogBox(r *dialog.Runner, onDone func()) *DialogBox {
	return &DialogBox{r: r, onDone: onDone}
}

func (d *DialogBox) boxRect() image.Rectangle {
	h := boxRows*gfx.LineHeight + 2*boxPad
	return image.Rect(boxMargin, d.h-boxMargin-h, d.w-boxMargin, d.h-boxMargin)
}

func (d *DialogBox) layout(ctx *Context) {
	if d.laidOut == d.r.Line() && d.w == ctx.W && d.h == ctx.H {
		return
	}
	d.w, d.h = ctx.W, ctx.H
	rows := gfx.Wrap(d.r.Line(), d.boxRect().Dx()-2*boxPad)
	d.pages = d.pages[:0]
	for len(rows) > boxRows {
		d.pages = append(d.pages, rows[:boxRows])
		rows = rows[boxRows:]
	}
	d.pages = append(d.pages, rows)
	if d.laidOut != d.r.Line() {
		d.pageIdx, d.shown, d.cursor = 0, 0, 0
	}
	d.pageIdx = min(d.pageIdx, len(d.pages)-1)
	d.page = d.pages[d.pageIdx]
	d.laidOut = d.r.Line()
}

func (d *DialogBox) pageLen() int {
	n := 0
	for _, row := range d.page {
		n += len(row)
	}
	return n
}

// typed reports whether the current page is fully revealed.
func (d *DialogBox) typed() bool { return d.page != nil && d.shown >= d.pageLen() }

func (d *DialogBox) lastPage() bool { return d.pageIdx == len(d.pages)-1 }

func (d *DialogBox) choices() []dialog.Choice {
	if !d.lastPage() || !d.typed() {
		return nil
	}
	return d.r.Choices()
}

func (d *DialogBox) choiceRects() []image.Rectangle {
	cs := d.r.Choices()
	w := 0
	for _, c := range cs {
		w = max(w, gfx.TextWidth(c.Text))
	}
	w += 2*boxPad + 8
	h := len(cs)*gfx.LineHeight + 2*boxPad
	box := d.boxRect()
	outer := image.Rect(box.Max.X-w, box.Min.Y-h-2, box.Max.X, box.Min.Y-2)
	rects := make([]image.Rectangle, len(cs)+1)
	rects[0] = outer
	for i := range cs {
		y := outer.Min.Y + boxPad + i*gfx.LineHeight
		rects[i+1] = image.Rect(outer.Min.X, y, outer.Max.X, y+gfx.LineHeight)
	}
	return rects
}

// Update implements Scene.
func (d *DialogBox) Update(ctx *Context) {
	in := ctx.Input
	d.layout(ctx)
	if d.shown < d.pageLen() {
		d.shown += charsPerTick
		if in.Pressed(input.Confirm) || tapped(in) {
			d.shown = d.pageLen()
		}
		return
	}
	if cs := d.choices(); len(cs) > 0 {
		if in.Pressed(input.Up) {
			d.cursor = (d.cursor + len(cs) - 1) % len(cs)
		}
		if in.Pressed(input.Down) {
			d.cursor = (d.cursor + 1) % len(cs)
		}
		if p, ok := in.Tap(); ok {
			for i, r := range d.choiceRects()[1:] {
				if p.In(r) {
					d.cursor = i
					d.choose(ctx)
					return
				}
			}
		}
		if in.Pressed(input.Confirm) {
			d.choose(ctx)
		}
		return
	}
	if in.Pressed(input.Confirm) || in.Pressed(input.Cancel) || tapped(in) {
		if !d.lastPage() {
			d.pageIdx++
			d.page = d.pages[d.pageIdx]
			d.shown = 0
			return
		}
		d.r.Advance()
		d.after(ctx)
	}
}

func (d *DialogBox) choose(ctx *Context) {
	d.r.Choose(d.cursor)
	d.after(ctx)
}

func (d *DialogBox) after(ctx *Context) {
	if d.r.Done() {
		ctx.Pop()
		if d.onDone != nil {
			d.onDone()
		}
		return
	}
	d.laidOut = ""
	d.layout(ctx)
}

func tapped(in *input.State) bool {
	_, ok := in.Tap()
	return ok
}

// Draw implements Scene.
func (d *DialogBox) Draw(dst *ebiten.Image) {
	if d.page == nil {
		return
	}
	box := d.boxRect()
	gfx.Box(dst, box)
	if sp := d.r.Speaker(); sp != "" {
		tw := gfx.TextWidth(sp) + 2*boxPad
		tag := image.Rect(box.Min.X+4, box.Min.Y-gfx.LineHeight-4, box.Min.X+4+tw, box.Min.Y+1)
		gfx.Box(dst, tag)
		gfx.Text(dst, sp, tag.Min.X+boxPad, tag.Min.Y+1, gfx.ColorAccent)
	}
	left := d.shown
	for i, row := range d.page {
		if left <= 0 {
			break
		}
		s := row
		if len(s) > left {
			s = s[:left]
		}
		left -= len(row)
		gfx.Text(dst, s, box.Min.X+boxPad, box.Min.Y+boxPad-1+i*gfx.LineHeight, gfx.ColorInk)
	}
	if d.shown >= d.pageLen() && len(d.choices()) == 0 {
		// "more" indicator
		if (ebiten.Tick()/20)%2 == 0 {
			gfx.Text(dst, "v", box.Max.X-boxPad-6, box.Max.Y-boxPad-gfx.LineHeight+2, gfx.ColorAccent)
		}
	}
	if cs := d.choices(); len(cs) > 0 {
		rects := d.choiceRects()
		gfx.Box(dst, rects[0])
		for i, c := range cs {
			r := rects[i+1]
			col := gfx.ColorInk
			if i == d.cursor {
				col = gfx.ColorAccent
				gfx.Text(dst, ">", r.Min.X+boxPad-2, r.Min.Y-1, col)
			}
			gfx.Text(dst, c.Text, r.Min.X+boxPad+6, r.Min.Y-1, col)
		}
	}
}
