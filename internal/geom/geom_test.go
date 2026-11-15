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

func TestWrapF(t *testing.T) {
	for _, c := range []struct{ d, n, want float64 }{
		{0, 10, 0}, {3, 10, 3}, {-3, 10, -3}, {6, 10, -4}, {-6, 10, 4}, {5, 10, -5}, {-5, 10, -5},
		{13, 10, 3}, {-13, 10, -3}, {2.5, 10, 2.5}, {7.5, 10, -2.5},
	} {
		if got := WrapF(c.d, c.n); got != c.want {
			t.Errorf("WrapF(%v, %v) = %v, want %v", c.d, c.n, got, c.want)
		}
	}
}

func TestIsqrt(t *testing.T) {
	for _, c := range [][2]int64{{-4, 0}, {0, 0}, {1, 1}, {2, 1}, {3, 1}, {4, 2}, {15, 3}, {16, 4}, {17, 4},
		{99, 9}, {100, 10}, {1 << 40, 1 << 20}, {1<<40 - 1, 1<<20 - 1}} {
		if got := Isqrt(c[0]); got != c[1] {
			t.Errorf("Isqrt(%d) = %d, want %d", c[0], got, c[1])
		}
	}
}
