// Package geom contains integer maths for big, wrapping coordinates.
//
// The main map is a torus: positions wrap on both axes. All wrap logic goes
// through this package so that seam handling lives in exactly one place.
package geom

import "math"

// FloorDiv divides rounding towards negative infinity.
func FloorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// Mod returns a modulo n in [0, n) for n > 0.
func Mod(a, n int64) int64 {
	m := a % n
	if m < 0 {
		m += n
	}
	return m
}

// WrapDelta returns the shortest signed difference b-a on a ring of size n,
// in [-n/2, n/2). For n <= 0 (no wrapping) it returns b-a.
func WrapDelta(a, b, n int64) int64 {
	d := b - a
	if n <= 0 {
		return d
	}
	d = Mod(d, n)
	if d >= (n+1)/2 {
		d -= n
	}
	return d
}

// Abs returns |v|.
func Abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// WrapF wraps a distance d on a ring of size n into [-n/2, n/2): the
// shortest way round, for fractional positions.
func WrapF(d, n float64) float64 {
	d = math.Mod(d, n)
	if d < -n/2 {
		d += n
	} else if d >= n/2 {
		d -= n
	}
	return d
}

// Isqrt returns the integer square root of v (0 for negative v): the same
// on every platform, unlike rounding math.Sqrt.
func Isqrt(v int64) int64 {
	if v <= 0 {
		return 0
	}
	r := int64(1)
	for r*r <= v {
		r <<= 1
	}
	lo, hi := r>>1, r
	for lo < hi-1 {
		mid := (lo + hi) / 2
		if mid*mid <= v {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}
