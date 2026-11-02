package sim

// Ambient is the colour of the light falling on the world, as factors for
// red, green and blue: white (1,1,1) at day, dark blue at night.
type Ambient struct{ R, G, B float32 }

// Daylight is full light: things look as drawn.
var Daylight = Ambient{1, 1, 1}

// Night is the ambient light at night, about 40% brightness with a blue
// cast: dark, but every path stays readable.
var Night = Ambient{0.36, 0.42, 0.64}

// The day's light at key times (minutes after midnight); in between, it
// blends linearly. Dawn and dusk pass through warm colours.
var daylightKeys = []struct {
	at int64
	a  Ambient
}{
	{4*60 + 30, Night},
	{5*60 + 30, Ambient{0.62, 0.56, 0.72}}, // first light, violet
	{6*60 + 30, Ambient{1, 0.84, 0.72}},    // sunrise, warm
	{7*60 + 30, Daylight},
	{17*60 + 30, Daylight},
	{18*60 + 30, Ambient{1, 0.82, 0.62}},    // golden hour
	{19*60 + 30, Ambient{0.72, 0.56, 0.66}}, // dusk
	{20*60 + 30, Night},
}

// LightAt returns the ambient light outdoors at a time in game minutes.
func LightAt(minutes int64) Ambient {
	t := minutes % MinutesPerDay
	if t < 0 {
		t += MinutesPerDay
	}
	keys := daylightKeys
	for i := range keys {
		next := keys[(i+1)%len(keys)]
		from, to := keys[i].at, next.at
		if to <= from {
			to += MinutesPerDay // the night, across midnight
		}
		tt := t
		if tt < from {
			tt += MinutesPerDay
		}
		if tt >= from && tt < to {
			f := float32(tt-from) / float32(to-from)
			return keys[i].a.lerp(next.a, f)
		}
	}
	return Daylight // unreachable: the keys cover the whole day
}

func (a Ambient) lerp(b Ambient, f float32) Ambient {
	return Ambient{a.R + (b.R-a.R)*f, a.G + (b.G-a.G)*f, a.B + (b.B-a.B)*f}
}

// Brightness is the perceived brightness, 0 (black) to 1 (daylight).
func (a Ambient) Brightness() float32 { return 0.299*a.R + 0.587*a.G + 0.114*a.B }

// Darkness is how strongly light sources show: 0 at day, 1 at night.
func (a Ambient) Darkness() float32 {
	d := (1 - a.Brightness()) / (1 - Night.Brightness())
	return min(max(d, 0), 1)
}
