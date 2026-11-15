package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// Vehicles on the main map: a tile per car, by name from the "worlds"
// tileset, or a kind's own drawing (the airliner's 3x3 tiles).

// trainSprite names a train's locomotive or carriage by its heading.
func trainSprite(c transit.Car) (name string, flip bool) {
	switch {
	case c.Loco && c.Facing == world.Right:
		return "train loco e", false
	case c.Loco && c.Facing == world.Left:
		return "train loco e", true
	case c.Loco && c.Facing == world.Down:
		return "train loco s", false
	case c.Loco:
		return "train loco n", false
	case c.Facing == world.Up || c.Facing == world.Down:
		return "train car v", false
	}
	return "train car h", false
}

// twoTileSprite names the halves of a two-tile vehicle: "<what> <front or
// back> <heading>", the west-facing ones mirrored from the east-facing.
func twoTileSprite(what, front, back string) func(transit.Car) (string, bool) {
	heading := map[world.Dir]string{world.Right: "e", world.Left: "e", world.Down: "s", world.Up: "n"}
	return func(c transit.Car) (string, bool) {
		part := back
		if c.Loco {
			part = front
		}
		return what + " " + part + " " + heading[c.Facing], c.Facing == world.Left
	}
}

// vehicleTile returns the image of a named tile of the "worlds" tileset;
// false if there is no such tile.
func (e *Explore) vehicleTile(name string) (*ebiten.Image, bool) {
	t, ok := e.vehicleTiles[name]
	if !ok {
		tt, err := e.w.Tilesets().Named("worlds", name)
		if err != nil {
			return nil, false
		}
		t = tt.Index()
		e.vehicleTiles[name] = t
	}
	return e.sheets.Tile(sheetWorlds, t), true
}

// carDrawables draws a vehicle of a kind: a sprite per car, or the kind's
// own drawing.
func (e *Explore) carDrawables(kind transit.Kind, m world.MapRef, cars []transit.Car, cx, cy float64) []drawable {
	v := vehicles[kind]
	if v.draw != nil {
		return v.draw(e, m, cars, cx, cy)
	}
	var out []drawable
	for _, c := range cars {
		x, y := e.screenAt(m, c.X*tile, c.Y*tile, cx, cy)
		if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
			continue
		}
		name, flip := v.sprite(c)
		img, _ := e.vehicleTile(name)
		out = append(out, drawable{img: img, flip: flip, x: x, y: y})
	}
	return out
}

// vehicleDrawables are the vehicles to draw, relative to the camera at
// alpha: those of the lines near the party, the taxi just left, and the
// taxis waiting at their ranks.
func (e *Explore) vehicleDrawables(m world.MapRef, alpha float64) []drawable {
	if e.infra == nil || !m.IsMain() {
		return nil
	}
	cx, cy := e.cameraAt(alpha)
	t := e.vehicleTick(alpha)
	var out []drawable
	for _, line := range e.nearLines {
		out = append(out, e.carDrawables(line.Kind, m, line.CarsAt(line.At(t)), cx, cy)...)
	}
	if e.lastTaxi != nil {
		out = append(out, e.carDrawables(transit.Taxi, m, e.lastTaxi.CarsAt(e.lastTaxi.At(t)), cx, cy)...)
	}
	seen := map[infra.Point]bool{}
	for _, st := range e.ranks {
		if !seen[st.Rank] {
			seen[st.Rank] = true
			out = append(out, e.carDrawables(transit.Taxi, m, rankTaxi(st, e.w), cx, cy)...)
		}
	}
	return out
}

// planeDrawables are the 3x3 tiles of a plane, around its middle car.
func (e *Explore) planeDrawables(m world.MapRef, cars []transit.Car, cx, cy float64) []drawable {
	mid := cars[len(cars)/2]
	name, flip := "plane e", false
	switch mid.Facing {
	case world.Left:
		flip = true
	case world.Down:
		name = "plane s"
	case world.Up:
		name = "plane n"
	}
	var out []drawable
	for i := 0; i < 9; i++ {
		r, c := i/3, i%3
		dx := c - 1
		if flip {
			dx = 1 - c
		}
		x, y := e.screenAt(m, (mid.X+float64(dx))*tile, (mid.Y+float64(r-1))*tile, cx, cy)
		if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
			continue
		}
		img, ok := e.vehicleTile(fmt.Sprintf("%s %d", name, i))
		if !ok {
			return nil
		}
		out = append(out, drawable{img: img, flip: flip, x: x, y: y})
	}
	return out
}
