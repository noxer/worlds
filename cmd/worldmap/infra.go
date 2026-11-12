package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/world"
)

var tierColors = [infra.NumTiers]color.RGBA{
	infra.POI:        {0x9a, 0x6a, 0x3a, 0xff},
	infra.Village:    {0xe0, 0x8a, 0x2a, 0xff},
	infra.Town:       {0xe0, 0x40, 0x30, 0xff},
	infra.City:       {0xb0, 0x20, 0x60, 0xff},
	infra.Metropolis: {0x50, 0x10, 0x50, 0xff},
}

var netStyle = [infra.NumNetworks]struct {
	c    color.RGBA
	dash int // draw dash pixels, skip dash pixels; 0 = solid
}{
	infra.Path:    {color.RGBA{0xa0, 0x78, 0x40, 0xff}, 0},
	infra.Road:    {color.RGBA{0xf0, 0xe0, 0x90, 0xff}, 0},
	infra.Highway: {color.RGBA{0xff, 0x90, 0x20, 0xff}, 0},
	infra.Rail:    {color.RGBA{0x30, 0x30, 0x30, 0xff}, 3},
	infra.SeaLane: {color.RGBA{0xff, 0xff, 0xff, 0xff}, 2},
	infra.Flight:  {color.RGBA{0xc0, 0x40, 0xc0, 0xff}, 6},
}

// drawInfra draws settlements and the chosen networks of the view onto img
// (one pixel per step x step tiles).
func drawInfra(img *image.RGBA, in *infra.Infra, wd *world.World, x0, y0, w, h, step int64, netNames []string) {
	minTier := infra.POI
	if step > 64 {
		minTier = infra.Village // points of interest are too small to see
	}
	var nets []infra.Network
	for _, name := range netNames {
		found := false
		for n := infra.Network(0); n < infra.NumNetworks; n++ {
			if n.String() == name {
				nets, found = append(nets, n), true
			}
		}
		if !found && name != "" {
			log.Fatalf("unknown network %q", name)
		}
	}
	// toPx converts a position to image pixels, the shortest way from the
	// reference point (rx, ry) given in pixels, so lines cross seams.
	toPx := func(p infra.Point, rx, ry float64) (float64, float64) {
		dx := geom.WrapDelta(x0+int64(rx*float64(step)), p.X, wd.W)
		dy := geom.WrapDelta(y0+int64(ry*float64(step)), p.Y, wd.H)
		return rx + float64(dx)/float64(step), ry + float64(dy)/float64(step)
	}
	start := time.Now()
	for _, n := range nets {
		t0 := time.Now()
		edges := edgesParallel(in, n, x0, y0, w, h)
		st := netStyle[n]
		for _, e := range edges {
			r := in.Route(e)
			ax, ay := toPx(r.Points[0], float64(w/step)/2, float64(h/step)/2)
			k := 0
			for _, p := range r.Points[1:] {
				bx, by := toPx(p, ax, ay)
				k = line(img, ax, ay, bx, by, st.c, st.dash, k)
				ax, ay = bx, by
			}
		}
		fmt.Printf("%-8s %6d edges  %v\n", n, len(edges), time.Since(t0).Round(time.Millisecond))
	}
	counts := map[infra.Tier]int{}
	for _, s := range in.InRect(x0, y0, w, h, minTier) {
		counts[s.ID.Tier]++
		px, py := toPx(infra.Point{X: s.X, Y: s.Y}, float64(w/step)/2, float64(h/step)/2)
		r := max(float64(s.Radius)/float64(step), float64(s.ID.Tier)*0.8+1.2)
		disc(img, px, py, r, tierColors[s.ID.Tier])
	}
	for t := minTier; t < infra.NumTiers; t++ {
		fmt.Printf("%-10s %6d\n", t, counts[t])
	}
	fmt.Printf("infrastructure in %v\n", time.Since(start).Round(time.Millisecond))
}

// edgesParallel is EdgesIn, with the routes computed on all cores.
func edgesParallel(in *infra.Infra, n infra.Network, x, y, w, h int64) []infra.Edge {
	ss := in.InRect(x, y, w, h, n.MinTier())
	work := make(chan infra.Settlement)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range work {
				in.Edges(n, s) // warms the route cache
			}
		}()
	}
	for _, s := range ss {
		work <- s
	}
	close(work)
	wg.Wait()
	return in.EdgesIn(n, x, y, w, h)
}

// line draws a line from (ax, ay) to (bx, by), dashed if dash > 0; k is the
// dash phase, returned for the next segment.
func line(img *image.RGBA, ax, ay, bx, by float64, c color.RGBA, dash, k int) int {
	steps := int(max(abs(bx-ax), abs(by-ay))) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if dash == 0 || (k/dash)%2 == 0 {
			img.SetRGBA(int(ax+(bx-ax)*t), int(ay+(by-ay)*t), c)
		}
		if i < steps {
			k++
		}
	}
	return k
}

func disc(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
