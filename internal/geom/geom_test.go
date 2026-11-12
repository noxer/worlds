package geom

import "testing"

func TestFloorDivMod(t *testing.T) {
	cases := []struct{ a, b, q, m int64 }{
		{7, 4, 1, 3}, {-1, 4, -1, 3}, {-4, 4, -1, 0}, {-5, 4, -2, 3}, {0, 4, 0, 0},
	}
	for _, c := range cases {
		if q := FloorDiv(c.a, c.b); q != c.q {
			t.Errorf("FloorDiv(%d,%d) = %d, want %d", c.a, c.b, q, c.q)
		}
		if m := Mod(c.a, c.b); m != c.m {
			t.Errorf("Mod(%d,%d) = %d, want %d", c.a, c.b, m, c.m)
		}
	}
}

func TestWrapDelta(t *testing.T) {
	cases := []struct{ a, b, n, d int64 }{
		{0, 3, 10, 3},
		{9, 0, 10, 1},  // across the seam forwards
		{0, 9, 10, -1}, // across the seam backwards
		{0, 5, 10, -5}, // halfway: canonical negative
		{2, 7, 0, 5},   // no wrapping
		{1 << 40, 3, 1 << 41, 3 - 1<<40},
	}
	for _, c := range cases {
		if d := WrapDelta(c.a, c.b, c.n); d != c.d {
			t.Errorf("WrapDelta(%d,%d,%d) = %d, want %d", c.a, c.b, c.n, d, c.d)
		}
	}
}
