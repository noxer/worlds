// Package game wires everything together and implements ebiten.Game.
package game

import (
	"image"
	"io/fs"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/content"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/scene"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// The logical screen is at least MinWidth x MinHeight pixels. It is scaled
// up by the largest integer factor that fits the window; any remaining space
// shows more of the world instead of letterboxing. 320x180 at 1280x720 and
// 1920x1080.
const (
	MinWidth  = 240
	MinHeight = 180
)

// Game implements ebiten.Game.
type Game struct {
	input   input.State
	stack   *scene.Stack
	world   *world.World
	explore *scene.Explore

	// Screen geometry: the logical (game pixel) size, the integer scale to
	// the physical screen, and the letterbox offset.
	w, h, scale int
	offX, offY  int
	uiImg       *ebiten.Image // dialogs, notices, overlays
	lastUpdate  time.Time
}

// New loads all content from fsys and sets up the starting scene.
func New(fsys fs.FS) (*Game, error) {
	c, err := content.Load(fsys)
	if err != nil {
		return nil, err
	}
	sheets := gfx.NewSheets(fsys)
	for _, ts := range c.World.Tilesets().All() {
		if err := sheets.Register(ts); err != nil {
			return nil, err
		}
	}

	party := entity.NewParty(&entity.Actor{Name: "Ari", Sprite: entity.Sprite{Sheet: "tinydungeon", Index: 85}})
	for _, f := range []struct {
		name  string
		index int
	}{{"Bryn", 99}, {"Cato", 112}, {"Dagny", 97}} {
		if err := party.Add(&entity.Actor{Name: f.name, Sprite: entity.Sprite{Sheet: "tinydungeon", Index: f.index}}); err != nil {
			return nil, err
		}
	}

	explore, err := scene.NewExplore(scene.ExploreConfig{
		World: c.World, Sheets: sheets, Dialogs: c.Dialogs, Party: party, Start: c.Start,
		Clock: sim.NewClock(1, 8, 0),
	})
	if err != nil {
		return nil, err
	}
	// Build chunks around the party ahead of time, so fast travel never
	// stalls a frame on generation.
	c.World.StartStreaming(world.DefaultWorkers())
	g := &Game{world: c.World, explore: explore, scale: 1}
	g.stack = scene.NewStack(&g.input, explore)
	g.input.ToLogical = func(p image.Point) image.Point {
		return image.Pt((p.X-g.offX)/g.scale, (p.Y-g.offY)/g.scale)
	}
	return g, nil
}

// Update implements ebiten.Game. It runs at a fixed 60 ticks per second.
func (g *Game) Update() error {
	t0 := time.Now()
	g.world.Pump() // also while dialogs or menus are open
	t1 := time.Now()
	g.input.Update()
	g.stack.Update()
	t2 := time.Now()
	g.lastUpdate = t2
	if scene.Perf.RecordTick(t1.Sub(t0), t2.Sub(t1)) && scene.DebugOverlay() {
		log.Printf("slow tick: chunks %v, update %v", t1.Sub(t0), t2.Sub(t1))
	}
	return nil
}

// Draw implements ebiten.Game. The exploration scene draws the world at
// physical resolution, following real time between ticks (see
// Explore.DrawWorld); the UI is drawn at game resolution and scaled up.
func (g *Game) Draw(screen *ebiten.Image) {
	t0 := time.Now()
	alpha := float64(time.Since(g.lastUpdate)) / float64(time.Second/60)
	g.explore.DrawWorld(screen, g.scale, g.offX, g.offY, alpha)

	g.uiImg = ensure(g.uiImg, g.w, g.h)
	g.uiImg.Clear()
	g.stack.Draw(g.uiImg)
	s := float64(g.scale)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(g.offX), float64(g.offY))
	screen.DrawImage(g.uiImg, op)
	scene.Perf.RecordDraw(time.Since(t0))
}

// ensure returns img if it has the given size, or a new image.
func ensure(img *ebiten.Image, w, h int) *ebiten.Image {
	if img != nil && img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	if img != nil {
		img.Deallocate()
	}
	return ebiten.NewImage(w, h)
}

// Layout implements ebiten.Game.
// The screen is rendered at its physical resolution; the game draws in
// game pixels and scales up itself (see Draw).
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	ds := ebiten.Monitor().DeviceScaleFactor()
	pw, ph := int(float64(outsideWidth)*ds), int(float64(outsideHeight)*ds)
	g.w, g.h = LogicalSize(outsideWidth, outsideHeight, ds)
	g.scale = pixelScale(float64(pw), float64(ph))
	g.offX, g.offY = (pw-g.w*g.scale)/2, (ph-g.h*g.scale)/2
	g.stack.SetSize(g.w, g.h)
	return pw, ph
}

// LogicalSize computes the logical screen size for a window size in device
// independent pixels.
func LogicalSize(outsideWidth, outsideHeight int, deviceScale float64) (int, int) {
	pw, ph := float64(outsideWidth)*deviceScale, float64(outsideHeight)*deviceScale
	scale := float64(pixelScale(pw, ph))
	return int(pw / scale), int(ph / scale)
}

// pixelScale is the integer scale from game pixels to physical pixels.
func pixelScale(pw, ph float64) int {
	return int(math.Max(1, math.Floor(math.Min(pw/MinWidth, ph/MinHeight))))
}
