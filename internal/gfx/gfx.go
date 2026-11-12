// Package gfx contains rendering helpers: tile sheets, text and UI boxes.
package gfx

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // register PNG decoder
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
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
	ColorMuted  = color.RGBA{0xa8, 0x9a, 0x88, 0xff}
	ColorVoid   = color.RGBA{0x1b, 0x1b, 0x28, 0xff}
)

// The UI font is the 7x13 bitmap font from x/image: crisp at any integer
// scale and it suits the pixel art. Glyphs are drawn from an atlas image;
// this avoids Ebitengine's text/v2, whose shaping engine adds about 1.5 MB
// to the WebAssembly build.
var face = basicfont.Face7x13

// LineHeight is the distance between text lines in pixels.
const LineHeight = 13

var (
	fontAtlas *ebiten.Image
	glyphs    = map[rune]*ebiten.Image{}
)

// glyph returns the image of r, or of '?' for runes the font lacks.
func glyph(r rune) *ebiten.Image {
	if g, ok := glyphs[r]; ok {
		return g
	}
	if fontAtlas == nil {
		// The mask is an alpha image; draw it as white with that alpha so
		// that ColorScale can tint it.
		b := face.Mask.Bounds()
		rgba := image.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, a := face.Mask.At(x, y).RGBA()
				rgba.Set(x, y, color.RGBA{uint8(a >> 8), uint8(a >> 8), uint8(a >> 8), uint8(a >> 8)})
			}
		}
		fontAtlas = ebiten.NewImageFromImage(rgba)
	}
	h := face.Ascent + face.Descent
	var g *ebiten.Image
	for _, rg := range face.Ranges {
		if r >= rg.Low && r < rg.High {
			y := (int(r-rg.Low) + rg.Offset) * h
			g = fontAtlas.SubImage(image.Rect(0, y, face.Width, y+h)).(*ebiten.Image)
			break
		}
	}
	if g == nil && r != '?' {
		g = glyph('?')
	}
	glyphs[r] = g
	return g
}

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
	op := &ebiten.DrawImageOptions{}
	op.ColorScale.ScaleWithColor(c)
	for _, r := range s {
		if r != ' ' {
			op.GeoM.Reset()
			op.GeoM.Translate(float64(x), float64(y))
			dst.DrawImage(glyph(r), op)
		}
		x += face.Advance
	}
}

// TextWidth returns the width of s in pixels.
func TextWidth(s string) int { return utf8.RuneCountInString(s) * face.Advance }

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
