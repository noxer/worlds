package scene

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/gfx"
)

// On-screen buttons, half transparent, for touch screens (they work with
// a mouse too): the menu button while exploring, zoom and close on the
// world map.

const (
	buttonSize   = 18
	buttonMargin = 4
)

// buttonIcon is what a button shows.
type buttonIcon int

const (
	iconMenu buttonIcon = iota // three bars
	iconPlus
	iconMinus
	iconClose
)

var (
	buttonFill = color.NRGBA{0xf4, 0xe9, 0xd2, 0x88}
	buttonEdge = color.NRGBA{0x2b, 0x1d, 0x23, 0xa0}
	buttonInk  = color.NRGBA{0x2b, 0x1d, 0x23, 0xd0}
)

// cornerButton returns the k-th button from the top-right corner of a
// screen w pixels wide (0 is the corner, 1 left of it, ...).
func cornerButton(w, k int) image.Rectangle {
	x1 := w - buttonMargin - k*(buttonSize+buttonMargin)
	return image.Rect(x1-buttonSize, buttonMargin, x1, buttonMargin+buttonSize)
}

// drawButton draws a button with its icon.
func drawButton(dst *ebiten.Image, r image.Rectangle, icon buttonIcon) {
	gfx.FillRect(dst, r, buttonEdge)
	gfx.FillRect(dst, r.Inset(1), buttonFill)
	c := r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))
	bar := func(x0, y0, x1, y1 int) { gfx.FillRect(dst, image.Rect(c.X+x0, c.Y+y0, c.X+x1, c.Y+y1), buttonInk) }
	switch icon {
	case iconMenu:
		for _, y := range []int{-5, -1, 3} {
			bar(-5, y, 6, y+2)
		}
	case iconPlus:
		bar(-5, -1, 6, 1)
		bar(-1, -5, 1, 6)
	case iconMinus:
		bar(-5, -1, 6, 1)
	case iconClose:
		for k := -4; k <= 4; k++ {
			bar(k, k, k+2, k+2)
			bar(k, -k, k+2, -k+2)
		}
	}
}
