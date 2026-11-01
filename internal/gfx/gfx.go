// Package gfx contains rendering helpers: tile sheets, text and UI boxes.
package gfx

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // register PNG decoder
	"io/fs"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"

	"github.com/noxer/worlds/internal/world"
)

// Sheets loads tile sheet images and hands out cached sub-images.
type Sheets struct {
	fsys   fs.FS
	sheets map[string]*sheet
}

type sheet struct {
	img                      *ebiten.Image
	cols, margin, spacing, n int
	tiles                    []*ebiten.Image
}

// NewSheets creates an empty sheet registry reading images from fsys.
func NewSheets(fsys fs.FS) *Sheets { return &Sheets{fsys: fsys, sheets: map[string]*sheet{}} }

// Register loads the image of a tileset, once per tileset name.
func (s *Sheets) Register(ts *world.Tileset) error {
	if _, ok := s.sheets[ts.Name]; ok {
		return nil
	}
	f, err := s.fsys.Open(ts.Image)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", ts.Image, err)
	}
	s.sheets[ts.Name] = &sheet{
		img:  ebiten.NewImageFromImage(img),
		cols: ts.Columns, margin: ts.Margin, spacing: ts.Spacing, n: ts.TileCount,
		tiles: make([]*ebiten.Image, ts.TileCount),
	}
	return nil
}

// Tile returns tile i of the named sheet, or nil.
func (s *Sheets) Tile(name string, i int) *ebiten.Image {
	sh := s.sheets[name]
	if sh == nil || i < 0 || i >= sh.n {
		return nil
	}
	if sh.tiles[i] == nil {
		const t = world.TileSize
		x := sh.margin + (i%sh.cols)*(t+sh.spacing)
		y := sh.margin + (i/sh.cols)*(t+sh.spacing)
		sh.tiles[i] = sh.img.SubImage(image.Rect(x, y, x+t, y+t)).(*ebiten.Image)
	}
	return sh.tiles[i]
}

// Palette used by the UI.
var (
	ColorInk    = color.RGBA{0x2b, 0x1d, 0x23, 0xff}
	ColorPaper  = color.RGBA{0xf4, 0xe9, 0xd2, 0xff}
	ColorAccent = color.RGBA{0xc8, 0x5a, 0x3c, 0xff}
	ColorVoid   = color.RGBA{0x1b, 0x1b, 0x28, 0xff}
)

// Face is the UI font: a crisp bitmap font that suits the pixel art.
var Face text.Face = text.NewGoXFace(basicfont.Face7x13)

// LineHeight is the distance between text lines in pixels.
const LineHeight = 13

var pixel *ebiten.Image

// FillRect fills a rectangle with a solid color.
func FillRect(dst *ebiten.Image, r image.Rectangle, c color.Color) {
	if pixel == nil {
		pixel = ebiten.NewImage(1, 1)
		pixel.Fill(color.White)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(r.Dx()), float64(r.Dy()))
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	op.ColorScale.ScaleWithColor(c)
	dst.DrawImage(pixel, op)
}

// Box draws a framed UI panel.
func Box(dst *ebiten.Image, r image.Rectangle) {
	FillRect(dst, r, ColorInk)
	FillRect(dst, r.Inset(1), ColorPaper)
	FillRect(dst, image.Rect(r.Min.X+2, r.Max.Y-2, r.Max.X-2, r.Max.Y-1), color.RGBA{0xd9, 0xc8, 0xa8, 0xff})
}

// Text draws a single line of text with its top-left corner at x, y.
func Text(dst *ebiten.Image, s string, x, y int, c color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, Face, op)
}

// TextWidth returns the width of s in pixels.
func TextWidth(s string) int { return int(text.Advance(s, Face)) }

// Wrap breaks s into lines no wider than width pixels.
func Wrap(s string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if line != "" && TextWidth(next) > width {
			lines = append(lines, line)
			next = word
		}
		line = next
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
