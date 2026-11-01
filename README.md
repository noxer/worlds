# worlds

A 2D open-world RPG experiment in Go with [Ebitengine](https://ebitengine.org):
exploration, puzzles, dialogs and quests, and turn-based tactical fights.
Art by [Kenney](https://kenney.nl) (Tiny Town, Tiny Dungeon, Tiny Battle; CC0),
plus placeholder tiles drawn for this project (`assets/tiles/worlds.png`).

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
| Ride bike / get off | R | Y | click your leader |
| Cancel | Esc, X | B | |
| Debug overlay | F3 | | |

## Editing the world

The world is configured in `assets/world.yaml` (seed, size, generator).
Hand-made areas are [Tiled](https://www.mapeditor.org) patches placed on the
main map: open `assets/maps/patches/main.world` in Tiled to edit them in
context. Interiors and caves are detail maps in `assets/maps/detail`. The
object conventions (spawns, warps, NPCs, signs) are listed in
ARCHITECTURE.md. `go test ./assets` validates the content.
