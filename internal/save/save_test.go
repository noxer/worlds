package save_test

import (
	"errors"
	"testing"
	"time"

	"github.com/noxer/worlds/internal/save"
)

func TestRoundTrip(t *testing.T) {
	for name, st := range map[string]save.Store{"memory": save.Memory{}, "files": save.Files(t.TempDir())} {
		if _, err := st.Load(0); !errors.Is(err, save.ErrEmpty) {
			t.Fatalf("%s: empty slot gives %v", name, err)
		}
		in := save.State{Seed: 7, Saved: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Place: "Brookhaven",
			Ticks: 12345, Party: []string{"Bryn", "Ari"}, Map: "smithy", Instance: 2, X: -4, Y: 9, Facing: 3,
			Riding: true, Visited: [][3]int64{{0, 1, 2}}}
		data, err := save.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Save(1, data); err != nil {
			t.Fatal(err)
		}
		back, err := st.Load(1)
		if err != nil {
			t.Fatal(err)
		}
		out, err := save.Decode(back)
		if err != nil {
			t.Fatal(err)
		}
		in.Version = save.Version
		if out.Place != in.Place || out.Ticks != in.Ticks || out.X != in.X || out.Y != in.Y || !out.Saved.Equal(in.Saved) ||
			out.Map != in.Map || out.Instance != in.Instance || out.Facing != in.Facing || !out.Riding ||
			len(out.Party) != 2 || out.Party[0] != "Bryn" || len(out.Visited) != 1 || out.Visited[0] != in.Visited[0] {
			t.Errorf("%s: got %+v, want %+v", name, out, in)
		}
		if l := save.List(st); l[0] != nil || l[1] == nil || l[1].Place != "Brookhaven" {
			t.Errorf("%s: list %v", name, l)
		}
	}
}

func TestDecodeRefusesOtherVersions(t *testing.T) {
	if _, err := save.Decode([]byte(`{"version": 99}`)); err == nil {
		t.Error("a save of another version decoded")
	}
	if _, err := save.Decode([]byte(`{nope`)); err == nil {
		t.Error("garbage decoded")
	}
}
