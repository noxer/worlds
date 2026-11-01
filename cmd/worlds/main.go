// Command worlds runs the game.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/assets"
	"github.com/noxer/worlds/internal/game"
)

func main() {
	g, err := game.New(assets.FS)
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("Worlds")
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
