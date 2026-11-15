package scene

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// Day and night: the world is drawn as usual, then multiplied by a light
// map at game resolution. The light map is the ambient light of the time
// of day (see sim.LightAt), with the glow of light sources (tiles with a
// "light" property) blended in. The UI is drawn afterwards, untouched.

// lightBands is the number of brightness steps of a glow. Hard steps at
// game resolution suit the pixel art better than smooth gradients.
const lightBands = 4

// ambient returns the light on the party's map now.
func (e *Explore) ambient() sim.Ambient {
	switch e.w.Lighting(e.party.Leader().Pos.Map) {
	case world.Indoor:
		return sim.Daylight
	case world.Dark:
		return sim.Night
	}
	return sim.LightAt(e.clock.Minutes())
}

// lightSpot is a light source in view.
type lightSpot struct {
	x, y float64 // centre, in pixels relative to the layer images
	world.Light
}

// lightsInView collects the light sources whose glow reaches into a
// w x h pixel area with its top-left corner at camera position (camX,
// camY) on map m.
func (e *Explore) lightsInView(m world.MapRef, camX, camY, w, h int64) []lightSpot {
	reach := int64(math.Ceil(e.w.Tilesets().MaxLightRadius()))
	if reach == 0 {
		return nil
	}
	sets := e.w.Tilesets()
	x0, y0 := geom.FloorDiv(camX, tile)-reach, geom.FloorDiv(camY, tile)-reach
	x1, y1 := geom.FloorDiv(camX+w-1, tile)+reach, geom.FloorDiv(camY+h-1, tile)+reach
	var spots []lightSpot
	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			p, ok := e.w.Normalize(world.Pos{Map: m, X: tx, Y: ty})
			if !ok {
				continue
			}
			for l := 0; l < world.NumLayers; l++ {
				if li, ok := sets.Light(e.w.Tile(p, l)); ok {
					spots = append(spots, lightSpot{
						x:     float64(tx*tile - camX + tile/2),
						y:     float64(ty*tile - camY + tile/2),
						Light: li,
					})
				}
			}
		}
	}
	return spots
}

// drawLight renders the light map for the current view into e.lightImg,
// or reports false in full daylight (nothing to do).
func (e *Explore) drawLight(m world.MapRef, w, h int) bool {
	amb := e.ambient()
	if amb == sim.Daylight {
		return false
	}
	e.lightImg = ensureImage(e.lightImg, w, h)
	e.lightImg.Fill(color.RGBA{uint8(amb.R*255 + 0.5), uint8(amb.G*255 + 0.5), uint8(amb.B*255 + 0.5), 0xff})
	dark := amb.Darkness()
	if dark <= 0 {
		return true
	}
	for _, s := range e.lightsInView(m, e.camX, e.camY, int64(w), int64(h)) {
		g := e.glow(s.Radius)
		r := float64(g.Bounds().Dx()) / 2
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(math.Round(s.x-r), math.Round(s.y-r))
		// The glow is premultiplied white; scaling it by the light's colour
		// and blending source-over moves the ambient towards that colour.
		op.ColorScale.Scale(s.R*dark, s.G*dark, s.B*dark, dark)
		e.lightImg.DrawImage(g, op)
	}
	return true
}

// glow returns the glow image of a light radius in tiles: full light in
// the middle, falling off in a few hard bands to nothing at the radius.
func (e *Explore) glow(radius float64) *ebiten.Image {
	if g, ok := e.glows[radius]; ok {
		return g
	}
	r := radius * tile
	n := int(math.Ceil(r)) * 2
	pix := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)+0.5-float64(n)/2, float64(y)+0.5-float64(n)/2
			a := glowAt(math.Hypot(dx, dy) / r)
			v := byte(a*255 + 0.5)
			i := (y*n + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, v
		}
	}
	g := ebiten.NewImage(n, n)
	g.WritePixels(pix)
	if e.glows == nil {
		e.glows = map[float64]*ebiten.Image{}
	}
	e.glows[radius] = g
	return g
}

// glowAt is the strength of a glow at a distance d from its centre, in
// radii: 1 in the middle, 0 from the radius on, in lightBands steps.
func glowAt(d float64) float64 {
	if d >= 1 {
		return 0
	}
	f := 1 - d*d
	return math.Ceil(f*lightBands) / lightBands
}

// multiply is the blend mode of the light map: the colour below is
// multiplied by the light map's.
var multiply = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorDestinationColor,
	BlendFactorSourceAlpha:      ebiten.BlendFactorZero,
	BlendFactorDestinationRGB:   ebiten.BlendFactorZero,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}
