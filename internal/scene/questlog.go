package scene

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/quest"
)

// Quests in the scene: the book (package quest) is moved on whenever the
// story may have changed; its news is shown as banners at the top of the
// screen for a few seconds each, whatever the party does meanwhile; the
// pause menu's Quests page is the journal.

const (
	toastTicks  = 240 // how long a banner stays (4 seconds)
	questsEvery = 30  // ticks between looks at the quests while exploring
)

// toast is a banner: a title and a line or two under it.
type toast struct {
	title, text string
}

// updateQuests moves the quests on and queues their news.
func (e *Explore) updateQuests() {
	for _, ev := range e.quests.Update(e.story) {
		var t toast
		switch ev.Kind {
		case quest.Started:
			t = toast{"New quest: " + ev.Quest.Name, ev.Stage.Text}
		case quest.Advanced:
			t = toast{ev.Quest.Name, ev.Stage.Text}
		case quest.Completed:
			t = toast{"Quest completed: " + ev.Quest.Name, ""}
		}
		if got := e.changesText(ev.Changes); got != "" {
			if t.text != "" {
				// What the stage gave comes first, as its own banner.
				e.toasts = append(e.toasts, toast{ev.Quest.Name, got})
			} else {
				t.text = got
			}
		}
		e.toasts = append(e.toasts, t)
	}
}

// tickToasts shows each banner for its time.
func (e *Explore) tickToasts() {
	if len(e.toasts) == 0 {
		return
	}
	if e.toastAge++; e.toastAge >= toastTicks {
		e.toasts, e.toastAge = e.toasts[1:], 0
	}
}

// drawToast draws the banner on show, centred at the top.
func (e *Explore) drawToast(dst *ebiten.Image) {
	if len(e.toasts) == 0 {
		return
	}
	t := e.toasts[0]
	w := dst.Bounds().Dx()
	maxW := w - 2*(buttonSize+2*buttonMargin) // clear of the corner button, both sides alike
	lines := gfx.Wrap(t.title, maxW-2*boxPad)
	nTitle := len(lines)
	if t.text != "" {
		lines = append(lines, gfx.Wrap(t.text, maxW-2*boxPad)...)
	}
	bw := 0
	for _, l := range lines {
		bw = max(bw, gfx.TextWidth(l))
	}
	bw += 2 * boxPad
	r := image.Rect((w-bw)/2, boxMargin, (w+bw)/2, boxMargin+len(lines)*gfx.LineHeight+2*boxPad-4)
	gfx.Box(dst, r)
	for i, l := range lines {
		col := gfx.ColorInk
		if i < nTitle {
			col = gfx.ColorAccent
		}
		gfx.Text(dst, l, r.Min.X+boxPad, r.Min.Y+boxPad-3+i*gfx.LineHeight, col)
	}
}
