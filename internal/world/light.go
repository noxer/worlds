package world

import (
	"fmt"
	"strconv"
	"strings"
)

// Light is a light source: a tile with the property "light" (radius in
// tiles, a float) and optionally "light_color" (a Tiled color). Lights show
// when it is dark; they don't change the game rules.
type Light struct {
	Radius  float64
	R, G, B float32
}

// defaultLightColor is warm lamp light.
var defaultLightColor = [3]float32{1, 0.9, 0.68}

func parseLight(p Props) (Light, bool, error) {
	v, ok := p["light"]
	if !ok {
		return Light{}, false, nil
	}
	r, ok := v.(float64)
	if !ok || r <= 0 {
		return Light{}, false, fmt.Errorf("light must be a positive radius in tiles, not %v", v)
	}
	l := Light{Radius: r, R: defaultLightColor[0], G: defaultLightColor[1], B: defaultLightColor[2]}
	if s := p.String("light_color"); s != "" {
		c, err := parseColor(s)
		if err != nil {
			return Light{}, false, fmt.Errorf("light_color: %w", err)
		}
		l.R, l.G, l.B = c[0], c[1], c[2]
	}
	return l, true, nil
}

// parseColor parses a Tiled color, "#rrggbb" or "#aarrggbb" (alpha ignored).
func parseColor(s string) ([3]float32, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) == 8 {
		h = h[2:]
	}
	if len(h) != 6 {
		return [3]float32{}, fmt.Errorf("bad color %q", s)
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return [3]float32{}, fmt.Errorf("bad color %q", s)
	}
	return [3]float32{float32(n>>16&0xff) / 255, float32(n>>8&0xff) / 255, float32(n&0xff) / 255}, nil
}

// Light returns the light tile i gives off, if any.
func (t *Tileset) Light(i int) (Light, bool) {
	l, ok := t.lights[i]
	return l, ok
}

// HasLights reports whether any tile of the set gives off light.
func (t *Tileset) HasLights() bool { return len(t.lights) > 0 }

// Light returns the light tile t gives off, if any.
func (ts *Tilesets) Light(t Tile) (Light, bool) {
	if t.Empty() {
		return Light{}, false
	}
	return ts.list[t.Set()].Light(t.Index())
}

// MaxLightRadius is the largest light radius of all tilesets, in tiles.
func (ts *Tilesets) MaxLightRadius() float64 {
	r := 0.0
	for _, s := range ts.list {
		for _, l := range s.lights {
			r = max(r, l.Radius)
		}
	}
	return r
}

// Lighting says how the time of day lights a map.
type Lighting uint8

const (
	// Outdoor maps follow the time of day: the main map, and detail maps
	// with the map property lighting=outdoor (a courtyard).
	Outdoor Lighting = iota
	// Indoor maps are lit at all times. The default for detail maps.
	Indoor
	// Dark maps (caves) are always as dark as the night.
	Dark
)

// ParseLighting parses the map property "lighting".
func ParseLighting(s string) (Lighting, error) {
	switch s {
	case "outdoor":
		return Outdoor, nil
	case "", "indoor":
		return Indoor, nil
	case "dark":
		return Dark, nil
	}
	return 0, fmt.Errorf("lighting must be outdoor, indoor or dark, not %q", s)
}

// Lighting returns how the time of day lights map m.
func (w *World) Lighting(m MapRef) Lighting {
	if m.IsMain() {
		return Outdoor
	}
	if t := w.templates[m.Template]; t != nil {
		l, _ := ParseLighting(t.src.props.String("lighting")) // validated on load
		return l
	}
	return Indoor
}
