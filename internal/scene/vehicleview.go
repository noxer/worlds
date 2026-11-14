package scene

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/world"
)

// The world passing outside a vehicle: through the windows of a carriage,
// bus, cab or cabin; all round a ferry's deck; the sky above the clouds
// from a plane at cruise. The view is the strip of world along the line,
// unrolled, so the vehicle always moves to the right through it.

// drawVehicleView draws what is outside the vehicle the party rides in map
// id (nothing if id is no vehicle's map).
func (e *Explore) drawVehicleView(dst *ebiten.Image, id world.MapID, alpha float64) {
	v, ok := vehicleByMap(id)
	switch {
	case !ok:
	case v.deck:
		e.drawDeckView(dst, alpha)
	default:
		e.drawWindowView(dst, alpha)
	}
}

// viewTrack is where along the track the window looks out (track distance
// from A's buffer stop) and which way the train heads (+1 for B, -1 for A):
// the train's real place near stations; at speed, the real place would be
// a blur, so the track passes at a calm pace instead, cutting back to the
// real place as the train slows into the station.
func (e *Explore) viewTrack(alpha float64) (s, heading float64, ok bool) {
	r := e.ride
	if r == nil {
		return 0, 0, false
	}
	t := e.vehicleTick(alpha)
	st := r.line.At(t)
	s, heading = st.Front, 1 // the ride's direction, not the timetable's next one
	if !r.toB {
		heading = -1
	}
	if !st.Moving && st.AtA != r.toB {
		// Standing at the destination: where it stopped, not where the
		// locomotive will be for the way back.
		s = r.line.Length
		if !r.toB {
			s = 0
		}
	}
	if st.Moving && st.ToB == r.toB {
		start := float64(r.line.Cars - 1) // where the locomotive leaves from
		if !st.ToB {
			start = r.line.Length - float64(r.line.Cars-1)
		}
		run := math.Abs(s - start)
		left := r.line.Length - float64(r.line.Cars-1) - run
		if run > viewSlowTiles && left > viewSlowTiles {
			since := t - float64(st.Since) - r.line.TimeTo(viewSlowTiles)
			s = start + heading*(viewSlowTiles+viewSpeed*max(since, 0))
			s = min(max(s, 0), r.line.Length)
		}
	}
	return s, heading, true
}

// viewPos is the world position the window looks out at (for streaming).
func (e *Explore) viewPos(alpha float64) (x, y float64, ok bool) {
	s, _, ok := e.viewTrack(alpha)
	if !ok {
		return 0, 0, false
	}
	x, y, _ = e.ride.line.Pos(s)
	return x, y, true
}

// viewSide is how many tiles beside the track the windows look.
const viewSide = 2

// drawWindowView draws the world outside into the carriage's windows,
// under the window tiles of dst (whose glass is transparent). The view is
// the land beside the track, unrolled: whichever way the track runs in the
// world, the carriage moves to the right through it (the scenery slides
// left), ahead to the right, on the train's left-hand side.
func (e *Explore) drawWindowView(dst *ebiten.Image, alpha float64) {
	s, heading, ok := e.viewTrack(alpha)
	if !ok {
		return
	}
	win, err := e.w.Tilesets().Named("worlds", "train window")
	if err != nil {
		return
	}
	line := e.ride.line
	vm := vehicleMap(e.ride.line)
	mw, mh, _ := e.w.Size(world.Detail(vm))
	e.viewImg = ensureImage(e.viewImg, tile, tile)
	u := heading * s // grows as the train goes, either way
	// A plane at cruise (the view no longer at its real place) is above
	// the clouds.
	sky := false
	if vehicles[line.Kind].sky {
		st := line.At(e.vehicleTick(alpha))
		sky = st.Moving && math.Abs(st.Front-s) > 2
	}
	for cy := int64(0); cy < mh; cy++ {
		for cx := int64(0); cx < mw; cx++ {
			p := world.Pos{Map: world.Detail(vm), X: cx, Y: cy}
			if e.w.Tile(p, world.LayerGround) != win {
				continue
			}
			// The windows side by side show the strip from left to right;
			// those along the top row look out to the left, others (a
			// windscreen) by their row.
			side := int64(viewSide)
			if cy > 0 {
				side = mh/2 - cy
			}
			v := u + float64(cx-mw/2)
			t0 := math.Floor(v)
			shift := int((v - t0) * tile)
			e.viewImg.Clear()
			if sky {
				drawSky(e.viewImg, t0, shift, cy)
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(float64(cx*tile-e.camX), float64(cy*tile-e.camY))
				dst.DrawImage(e.viewImg, op)
				continue
			}
			for k := 0; k < 2; k++ {
				sd := heading * (t0 + float64(k)) // back to track distance
				x, y, d := line.Pos(sd)
				if heading < 0 {
					d = d.Opposite()
				}
				nx, ny := leftOf(d)
				wp, ok := e.w.Normalize(world.Pos{Map: world.Main,
					X: int64(math.Round(x)) + nx*side, Y: int64(math.Round(y)) + ny*side})
				if ok {
					e.drawWorldTile(e.viewImg, wp, k*tile-shift, 0)
				}
			}
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(cx*tile-e.camX), float64(cy*tile-e.camY))
			dst.DrawImage(e.viewImg, op)
		}
	}
}

// drawDeckView draws the world round a deck (the ferry's), over the whole
// of dst: the strip of world along the route, unrolled like the train's
// window view. The ferry sails to the right through it; each row above the
// deck's middle row is a tile further to the left of the route (port),
// each row below one to the right.
func (e *Explore) drawDeckView(dst *ebiten.Image, alpha float64) {
	s, heading, ok := e.viewTrack(alpha)
	if !ok {
		return
	}
	line := e.ride.line
	mw, mh, _ := e.w.Size(world.Detail(vehicleMap(line)))
	u := heading * s
	t0 := math.Floor(u)
	shift := int((u - t0) * tile)
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx0, cy0 := geom.FloorDiv(e.camX, tile), geom.FloorDiv(e.camY, tile)
	for cy := cy0; cy*tile-e.camY < int64(h); cy++ {
		lat := mh/2 - cy
		for cx := cx0; cx*tile-e.camX-int64(shift) < int64(w); cx++ {
			sd := heading * (t0 + float64(cx-mw/2)) // back to route distance
			sc := min(max(sd, 0), line.Length)
			x, y, d := line.Pos(sc)
			// Beyond the ends (at the piers), straight on.
			ax, ay := dirVec(d)
			x, y = x+ax*(sd-sc), y+ay*(sd-sc)
			if heading < 0 {
				d = d.Opposite()
			}
			nx, ny := leftOf(d)
			wp, ok := e.w.Normalize(world.Pos{Map: world.Main,
				X: int64(math.Round(x)) + nx*lat, Y: int64(math.Round(y)) + ny*lat})
			if ok {
				e.drawWorldTile(dst, wp, int(cx*tile-e.camX)-shift, int(cy*tile-e.camY))
			}
		}
	}
}

// dirVec returns the unit step of a direction.
func dirVec(d world.Dir) (dx, dy float64) {
	switch d {
	case world.Right:
		return 1, 0
	case world.Left:
		return -1, 0
	case world.Down:
		return 0, 1
	}
	return 0, -1
}

// drawSky draws the sky over the clouds into a window tile: blue, with
// white clouds drifting by (by view distance t0, shifted by shift pixels).
func drawSky(dst *ebiten.Image, t0 float64, shift int, row int64) {
	dst.Fill(color.RGBA{136, 190, 236, 255})
	for k := 0; k < 2; k++ {
		h := uint64(int64(t0)+int64(k))*0x9e3779b97f4a7c15 ^ uint64(row)*0xbf58476d1ce4e5b9
		h ^= h >> 29
		if h%3 != 0 {
			continue
		}
		x := k*tile - shift + int(h>>8)%6
		y := 3 + int(h>>16)%6
		w := 7 + int(h>>24)%6
		vector.DrawFilledRect(dst, float32(x), float32(y), float32(w), 4, color.RGBA{250, 250, 252, 255}, false)
		vector.DrawFilledRect(dst, float32(x+2), float32(y-2), float32(w-4), 2, color.RGBA{250, 250, 252, 255}, false)
	}
}

// leftOf returns the offset to the left of a direction of travel.
func leftOf(d world.Dir) (dx, dy int64) {
	switch d {
	case world.Right:
		return 0, -1
	case world.Left:
		return 0, 1
	case world.Down:
		return 1, 0
	}
	return -1, 0
}

// drawWorldTile draws all layers of main-map tile p at (x, y) (pixels) in
// dst.
func (e *Explore) drawWorldTile(dst *ebiten.Image, p world.Pos, x, y int) {
	sets := e.w.Tilesets()
	ms := animationClock()
	for l := 0; l < world.NumLayers; l++ {
		t := sets.Frame(e.w.Tile(p, l), ms, p.X, p.Y)
		if t.Empty() {
			continue
		}
		e.drawTile(dst, e.sheets.Tile(sets.Get(t.Set()).Name, t.Index()), x, y, t.FlipH(), t.FlipV())
	}
}
