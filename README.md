# worlds

A 2D open-world RPG experiment in Go with [Ebitengine](https://ebitengine.org):
exploration, puzzles, dialogs and quests, and turn-based tactical fights.
Art by [Kenney](https://kenney.nl) (CC0).

See [ARCHITECTURE.md](ARCHITECTURE.md) for the design.

## Running

```sh
go run ./cmd/worlds          # desktop
make serve                   # browser build at http://localhost:8080
make test                    # vet + tests
```

On Linux, desktop builds need the X11/GL headers, for example:
`sudo apt install libx11-dev libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev libgl1-mesa-dev libxxf86vm-dev libasound2-dev`

## Controls

| | Keyboard | Gamepad | Mouse / touch |
|---|---|---|---|
| Move | arrows / WASD (tap to turn) | d-pad / left stick | click or tap a tile |
| Talk / confirm | Space, Enter, E, Z | A | click an NPC or sign |
| Cancel | Esc, X | B | |
| Debug overlay | F3 | | |

## Editing the world

Maps are [Tiled](https://www.mapeditor.org) files in `assets/maps`. Open
`assets/maps/overworld.world` in Tiled to edit the town and meadow side by
side. The object conventions (spawns, warps, NPCs, signs) are listed in
ARCHITECTURE.md. `go test ./assets` validates the content.
