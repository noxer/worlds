// Command worldmap renders part of the generated world to a PNG, for
// checking generation and choosing where to put hand-made content.
//
//	go run ./cmd/worldmap -x 0 -y 1000 -w 256 -h 256 -o map.png          # tiles
//	go run ./cmd/worldmap -step 512 -o planet.png                         # whole world
//
// With -step 1 every tile is generated and coloured by terrain (trees and
// other obstacles darker). With a larger step, one pixel stands for step x
// step tiles and only the cheap elevation preview is used. With -tiles the
// real tile art is drawn (16x16 pixels per tile, first animation frame).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/world"
)

var palette = map[world.Terrain]color.RGBA{
	world.Land:    {0x7f, 0xc3, 0x5f, 0xff},
	world.Shallow: {0x75, 0xe3, 0xff, 0xff},
	world.Deep:    {0x29, 0x7c, 0xd9, 0xff},
}

func main() {
	x := flag.Int64("x", 0, "left edge (tiles)")
	y := flag.Int64("y", 0, "top edge (tiles)")
	w := flag.Int64("w", 0, "width in tiles (default: whole world)")
	h := flag.Int64("h", 0, "height in tiles (default: whole world)")
	step := flag.Int64("step", 1, "tiles per pixel")
	seed := flag.Uint64("seed", 0, "override the world seed")
	out := flag.String("o", "worldmap.png", "output file")
	tiles := flag.Bool("tiles", false, "draw the tile art (16 px per tile)")
	flag.Parse()

	var opt content.Options
	if *seed != 0 {
		opt.Seed = seed
	}
	c, err := content.LoadWith(assets.FS, opt)
	if err != nil {
		log.Fatal(err)
	}
	wd := c.World
	if *w == 0 {
		*w = wd.W
	}
	if *h == 0 {
		*h = wd.H
	}
	if *tiles {
		if err := drawTiles(wd, *x, *y, *w, *h, *out); err != nil {
			log.Fatal(err)
		}
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, int(*w / *step), int(*h / *step)))
	counts := map[world.Terrain]int{}
	for py := 0; py < img.Bounds().Dy(); py++ {
		for px := 0; px < img.Bounds().Dx(); px++ {
			tx, ty := *x+int64(px)**step, *y+int64(py)**step
			p, _ := wd.Normalize(world.Pos{Map: world.Main, X: tx, Y: ty})
			var t world.Terrain
			c := color.RGBA{}
			if *step == 1 {
				t = wd.Terrain(p)
				c = palette[t]
				if wd.Solid(p) {
					c = color.RGBA{c.R / 2, c.G / 2, c.B / 2, 0xff}
				}
			} else {
				var ok bool
				if t, ok = wd.Preview(p.X, p.Y); !ok {
					log.Fatal("the generator has no preview; use -step 1")
				}
				c = palette[t]
			}
			counts[t]++
			img.SetRGBA(px, py, c)
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	total := float64(img.Bounds().Dx() * img.Bounds().Dy())
	fmt.Printf("%s: %dx%d px; land %.0f%%, shallow %.0f%%, deep %.0f%%\n", *out, img.Bounds().Dx(), img.Bounds().Dy(),
		100*float64(counts[world.Land])/total, 100*float64(counts[world.Shallow])/total, 100*float64(counts[world.Deep])/total)
}

// drawTiles renders the area with the real tile art.
func drawTiles(wd *world.World, x0, y0, w, h int64, out string) error {
	sets := wd.Tilesets()
	sheets := map[int]image.Image{}
	sheet := func(i int) (image.Image, error) {
		if img, ok := sheets[i]; ok {
			return img, nil
		}
		f, err := assets.FS.Open(sets.Get(i).Image)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, err
		}
		sheets[i] = img
		return img, nil
	}
	const ts = world.TileSize
	img := image.NewRGBA(image.Rect(0, 0, int(w*ts), int(h*ts)))
	for ty := int64(0); ty < h; ty++ {
		for tx := int64(0); tx < w; tx++ {
			p, _ := wd.Normalize(world.Pos{Map: world.Main, X: x0 + tx, Y: y0 + ty})
			for l := 0; l < world.NumLayers; l++ {
				t := wd.Tile(p, l)
				if t.Empty() {
					continue
				}
				src, err := sheet(t.Set())
				if err != nil {
					return err
				}
				set := sets.Get(t.Set())
				sx := set.Margin + (t.Index()%set.Columns)*(ts+set.Spacing)
				sy := set.Margin + (t.Index()/set.Columns)*(ts+set.Spacing)
				dst := image.Rect(int(tx*ts), int(ty*ts), int(tx*ts)+ts, int(ty*ts)+ts)
				draw.Draw(img, dst, src, image.Pt(sx, sy), draw.Over)
			}
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Printf("%s: %dx%d tiles\n", out, w, h)
	return nil
}
