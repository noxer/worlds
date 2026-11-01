// Package game wires everything together and implements ebiten.Game.
package game

import (
	"fmt"
	"io/fs"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/scene"
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
	input input.State
	stack *scene.Stack
}

// New loads all content from fsys and sets up the starting scene.
func New(fsys fs.FS) (*Game, error) {
	w, err := world.Load(fsys, "maps")
	if err != nil {
		return nil, fmt.Errorf("load world: %w", err)
	}
	dialogs, err := dialog.Load(fsys, "dialog")
	if err != nil {
		return nil, fmt.Errorf("load dialogs: %w", err)
	}
	sheets := gfx.NewSheets(fsys)
	for _, m := range w.Maps {
		for _, l := range m.Layers {
			for _, t := range l.Tiles {
				if !t.Empty() {
					if err := sheets.Register(t.Set); err != nil {
						return nil, err
					}
				}
			}
		}
		for _, o := range m.Objects {
			if !o.Tile.Empty() {
				if err := sheets.Register(o.Tile.Set); err != nil {
					return nil, err
				}
			}
		}
	}

	start, ok := w.Map("town").Spawn("start")
	if !ok {
		return nil, fmt.Errorf("map town has no spawn named start")
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
		World: w, Sheets: sheets, Dialogs: dialogs, Party: party, Start: start,
	})
	if err != nil {
		return nil, err
	}
	g := &Game{}
	g.stack = scene.NewStack(&g.input, explore)
	return g, nil
}

// Update implements ebiten.Game. It runs at a fixed 60 ticks per second.
func (g *Game) Update() error {
	g.input.Update()
	g.stack.Update()
	return nil
}

// Draw implements ebiten.Game.
func (g *Game) Draw(screen *ebiten.Image) { g.stack.Draw(screen) }

// Layout implements ebiten.Game.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	w, h := LogicalSize(outsideWidth, outsideHeight, ebiten.Monitor().DeviceScaleFactor())
	g.stack.SetSize(w, h)
	return w, h
}

// LogicalSize computes the logical screen size for a window size in device
// independent pixels.
func LogicalSize(outsideWidth, outsideHeight int, deviceScale float64) (int, int) {
	pw, ph := float64(outsideWidth)*deviceScale, float64(outsideHeight)*deviceScale
	scale := math.Max(1, math.Floor(math.Min(pw/MinWidth, ph/MinHeight)))
	return int(pw / scale), int(ph / scale)
}
