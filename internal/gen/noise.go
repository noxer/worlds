package gen

import "fmt"

// Field is fractal value noise that repeats with the world's size, so the
// wrap seams are invisible. It uses integer arithmetic only, which keeps the
// world identical on every CPU (no floating-point rounding differences).
//
// Each octave places random values on a lattice of Cell x Cell tiles and
// interpolates between them with a smoothstep curve. Coarse octaves shape
// continents (the "world level" of generation), fine ones shape coastlines.
type Field struct {
	W, H    int64
	Salt    uint64 // separates independent fields (elevation, moisture, ...)
	Octaves []Octave
	ampSum  int64
}

// Octave is one layer of a Field.
type Octave struct {
	Cell int64 // lattice spacing in tiles; must divide the world size
	Amp  int64 // weight relative to the other octaves
}

// NewField validates the octaves against the world size.
func NewField(w, h int64, salt uint64, octaves ...Octave) (*Field, error) {
	f := &Field{W: w, H: h, Salt: salt, Octaves: octaves}
	for _, o := range octaves {
		if o.Cell <= 0 || w%o.Cell != 0 || h%o.Cell != 0 {
			return nil, fmt.Errorf("noise cell %d must divide the world size %dx%d", o.Cell, w, h)
		}
		if o.Amp <= 0 {
			return nil, fmt.Errorf("noise amplitude must be positive")
		}
		f.ampSum += o.Amp
	}
	if f.ampSum == 0 {
		return nil, fmt.Errorf("noise needs at least one octave")
	}
	return f, nil
}

// MaxValue is the upper bound (exclusive) of Field.At.
const MaxValue = 1 << 16

// At returns the field's value at a normalised main-map position, in
// [0, MaxValue).
func (f *Field) At(seed uint64, x, y int64) int64 {
	var sum int64
	for i, o := range f.Octaves {
		sum += o.Amp * f.octave(seed, int64(i), o.Cell, x, y)
	}
	return sum / f.ampSum
}

// AtCoarse is At with only the octaves of at least minCell tiles: the
// field's large-scale shape, without small detail.
func (f *Field) AtCoarse(seed uint64, x, y, minCell int64) int64 {
	var sum, amps int64
	for i, o := range f.Octaves {
		if o.Cell >= minCell {
			sum += o.Amp * f.octave(seed, int64(i), o.Cell, x, y)
			amps += o.Amp
		}
	}
	if amps == 0 {
		return MaxValue / 2
	}
	return sum / amps
}

func (f *Field) octave(seed uint64, i, cell, x, y int64) int64 {
	nx, ny := f.W/cell, f.H/cell
	lx, ly := x/cell, y/cell
	tx := smooth((x % cell) * MaxValue / cell)
	ty := smooth((y % cell) * MaxValue / cell)
	v := func(cx, cy int64) int64 {
		return int64(Hash(seed^f.Salt, i, cx%nx, cy%ny) & (MaxValue - 1))
	}
	a := lerp(v(lx, ly), v(lx+1, ly), tx)
	b := lerp(v(lx, ly+1), v(lx+1, ly+1), tx)
	return lerp(a, b, ty)
}

// smooth is the smoothstep curve 3t²-2t³ in 16.16 fixed point.
func smooth(t int64) int64 { return t * t * (3*MaxValue - 2*t) >> 32 }

func lerp(a, b, t int64) int64 { return a + (b-a)*t>>16 }
