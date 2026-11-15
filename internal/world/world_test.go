package world_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noxer/worlds/internal/world"
	"github.com/noxer/worlds/internal/world/worldtest"
)

var pos = worldtest.Pos

// A 64x64 world of walls with a camp patch straddling the east-west seam
// (x = 61..63 and 0..2) and a hut reached through a warp.
func spec() worldtest.Spec {
	return worldtest.Spec{
		Patches: []worldtest.Map{{
			Name: "camp", X: -3, Y: 10,
			Rows: []string{
				"######",
				"#....#",
				"#.....",
				"######",
			},
			Spawns: map[string][2]int{"door": {2, 2}},
			Warps:  []worldtest.Warp{{X: 2, Y: 1, ToMap: "hut", ToSpawn: "in"}},
		}},
		Details: []worldtest.Map{{
			Name: "hut",
			Rows: []string{
				"###",
				"#.#",
				"#.#",
			},
			Spawns: map[string][2]int{"in": {1, 1}},
			Warps:  []worldtest.Warp{{X: 1, Y: 2, ToMap: "main", ToSpawn: "door"}},
		}},
	}
}

func TestWrapAndNeighbors(t *testing.T) {
	w := worldtest.Load(t, spec())
	cases := []struct {
		from world.Pos
		dir  world.Dir
		want world.Pos
		ok   bool
	}{
		{pos("main", 63, 11), world.Right, pos("main", 0, 11), true},
		{pos("main", 0, 11), world.Left, pos("main", 63, 11), true},
		{pos("main", 5, 0), world.Up, pos("main", 5, 63), true},
		{pos("hut", 1, 1), world.Up, pos("hut", 1, 0), true},
		{pos("hut", 1, 0), world.Up, world.Pos{}, false},
	}
	for _, c := range cases {
		got, ok := w.Neighbor(c.from, c.dir)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Neighbor(%v, %v) = %v, %v; want %v, %v", c.from, c.dir, got, ok, c.want, c.ok)
		}
	}
	if d, ok := w.DirBetween(pos("main", 63, 11), pos("main", 0, 11)); !ok || d != world.Right {
		t.Errorf("DirBetween across the seam = %v, %v", d, ok)
	}
	if dx, dy, _ := w.Delta(pos("main", 62, 1), pos("main", 1, 62)); dx != 3 || dy != -3 {
		t.Errorf("Delta = %d,%d; want 3,-3", dx, dy)
	}
	if p, _ := w.Normalize(pos("main", -1, 64)); p != pos("main", 63, 0) {
		t.Errorf("Normalize = %v", p)
	}
}

func TestPatchOverridesGenerator(t *testing.T) {
	w := worldtest.Load(t, spec())
	// Patch floor across the seam is walkable, the generated rest is wall.
	for _, p := range []world.Pos{pos("main", 62, 11), pos("main", 63, 11), pos("main", 0, 11), pos("main", 2, 12)} {
		if w.Solid(p) {
			t.Errorf("%v should be walkable", p)
		}
	}
	if !w.Solid(pos("main", 30, 30)) {
		t.Errorf("generated wall expected")
	}
	if o, ok := w.PatchOrigin("camp"); !ok || o != pos("main", 61, 10) {
		t.Errorf("PatchOrigin = %v, %v", o, ok)
	}
}

func TestUnpaintedCellsAreGenerated(t *testing.T) {
	s := spec()
	s.Fill = '.'
	s.Patches[0].Rows[3] = "### ##" // hole in the bottom wall
	w := worldtest.Load(t, s)
	if w.Solid(pos("main", 0, 13)) {
		t.Errorf("unpainted patch cell should come from the generator (floor)")
	}
	if !w.Solid(pos("main", 63, 13)) {
		t.Errorf("painted wall should stay")
	}
}

func TestWarps(t *testing.T) {
	w := worldtest.Load(t, spec())
	wp := w.WarpAt(pos("main", 63, 11))
	if wp == nil || wp.To != pos("hut", 1, 1) {
		t.Fatalf("warp into hut = %+v", wp)
	}
	back := w.WarpAt(pos("hut", 1, 2))
	if back == nil || back.To != pos("main", 63, 12) {
		t.Fatalf("warp out of hut = %+v", back)
	}
}

func TestBlockersSurviveEviction(t *testing.T) {
	fsys, cfg := spec().Config()
	cfg.CacheChunks = 2
	w, err := world.Load(fsys, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := pos("main", 62, 11)
	v := w.Version()
	w.SetBlocked(p, true)
	if !w.Solid(p) || w.Version() == v {
		t.Fatal("SetBlocked did not block or bump the version")
	}
	// Touch every chunk; the cache can only hold two.
	for y := int64(0); y < 64; y += world.ChunkSize {
		for x := int64(0); x < 64; x += world.ChunkSize {
			w.Solid(pos("main", x, y))
		}
	}
	if w.CachedChunks() > 2 {
		t.Fatalf("cache holds %d chunks", w.CachedChunks())
	}
	if !w.Solid(p) {
		t.Fatal("blocker lost after eviction")
	}
	w.SetBlocked(p, false)
	if w.Solid(p) {
		t.Fatal("SetBlocked(false) did not unblock")
	}
}

func TestValidation(t *testing.T) {
	s := spec()
	s.Patches[0].Warps = append(s.Patches[0].Warps, worldtest.Warp{X: 1, Y: 1, ToMap: "hut", ToSpawn: "nope"})
	fsys, cfg := s.Config()
	if _, err := world.Load(fsys, cfg); err == nil || !strings.Contains(err.Error(), "unknown spawn") {
		t.Fatalf("expected unknown spawn error, got %v", err)
	}

	s = spec()
	s.Patches = append(s.Patches, worldtest.Map{Name: "other", X: 2, Y: 12, Rows: []string{".."}})
	fsys, cfg = s.Config()
	if _, err := world.Load(fsys, cfg); err == nil || !strings.Contains(err.Error(), "both paint") {
		t.Fatalf("expected overlap error, got %v", err)
	}

	// Overlap only in unpainted cells is fine.
	s = spec()
	s.Patches = append(s.Patches, worldtest.Map{Name: "other", X: 1, Y: 12, Rows: []string{"  ", "  ", ".."}})
	worldtest.Load(t, s)
}

// TestMultiTileValidation checks the "above" tileset property: the lower
// part of a two-tile object needs its upper part on the overhead layer
// above, and upper parts may not float.
func TestMultiTileValidation(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		err  string // "" means the map is valid
	}{
		{"complete", []string{"..t..", "..T.."}, ""},
		{"missing top", []string{".....", "..T.."}, "needs tile 3 on the overhead layer"},
		{"top row", []string{"..T..", "....."}, "top row"},
		{"floating top", []string{"..t..", "....."}, "lower part is missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys, cfg := worldtest.Spec{Details: []worldtest.Map{{Name: "room", Rows: c.rows}}}.Config()
			w, err := world.Load(fsys, cfg)
			if c.err == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// The top is on the overhead layer and does not block.
				if w.Solid(pos("room", 2, 0)) || !w.Solid(pos("room", 2, 1)) {
					t.Fatal("upper part must be walkable, lower part solid")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.err) || !strings.Contains(err.Error(), "room.tmj") {
				t.Fatalf("error = %v, want one naming room.tmj and containing %q", err, c.err)
			}
		})
	}
}

// animatedConfig loads the default test world whose tileset animates the floor
// tile (0) through tiles 0, 4, 5 with the given phase property.
func animatedConfig(t *testing.T, phase string, frames string) (world.Config, *world.World, error) {
	t.Helper()
	fsys, cfg := worldtest.Spec{Details: []worldtest.Map{{Name: "room", Rows: []string{"...."}}}}.Config()
	var ts map[string]any
	if err := json.Unmarshal(fsys["tilesets/test.tsj"].Data, &ts); err != nil {
		t.Fatal(err)
	}
	var anim []any
	if err := json.Unmarshal([]byte(frames), &anim); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"id": 0, "animation": anim}
	if phase != "" {
		entry["properties"] = []any{map[string]any{"name": "phase", "type": "string", "value": phase}}
	}
	ts["tiles"] = append(ts["tiles"].([]any), entry)
	b, _ := json.Marshal(ts)
	fsys["tilesets/test.tsj"].Data = b
	w, err := world.Load(fsys, cfg)
	return cfg, w, err
}

func TestTileAnimation(t *testing.T) {
	_, w, err := animatedConfig(t, "", `[{"tileid":0,"duration":100},{"tileid":4,"duration":200},{"tileid":5,"duration":100}]`)
	if err != nil {
		t.Fatal(err)
	}
	ts := w.Tilesets()
	floor := w.Tile(pos("room", 0, 0), world.LayerGround)
	cases := []struct {
		ms   int64
		want int
	}{{0, 0}, {99, 0}, {100, 4}, {299, 4}, {300, 5}, {399, 5}, {400, 0}, {1300, 4}}
	for _, c := range cases {
		if got := ts.Frame(floor, c.ms, 0, 0).Index(); got != c.want {
			t.Errorf("frame at %d ms = %d, want %d", c.ms, got, c.want)
		}
	}
	// Synchronised: every cell shows the same frame.
	if ts.Frame(floor, 150, 0, 0) != ts.Frame(floor, 150, 3, 7) {
		t.Error("unphased animation differs between cells")
	}
	// Frames don't change what the cell is: still walkable floor.
	if w.Solid(pos("room", 0, 0)) || w.Terrain(pos("room", 0, 0)) != world.Land {
		t.Error("animation changed collision or terrain")
	}
	// Non-animated tiles are returned unchanged.
	wall, _ := ts.Tile("test", 1)
	if ts.Frame(wall, 150, 0, 0) != wall {
		t.Error("static tile changed")
	}
}

func TestPhasedAnimation(t *testing.T) {
	_, w, err := animatedConfig(t, "random", `[{"tileid":0,"duration":100},{"tileid":4,"duration":100}]`)
	if err != nil {
		t.Fatal(err)
	}
	ts := w.Tilesets()
	floor := w.Tile(pos("room", 0, 0), world.LayerGround)
	differ := false
	for x := int64(0); x < 20; x++ {
		if ts.Frame(floor, 0, x, 0) != ts.Frame(floor, 0, 0, 0) {
			differ = true
		}
	}
	if !differ {
		t.Error("random phase: all cells show the same frame")
	}
	if ts.Frame(floor, 0, 5, 9) != ts.Frame(floor, 0, 5, 9) {
		t.Error("phase must be deterministic")
	}
}

func TestAnimationValidation(t *testing.T) {
	if _, _, err := animatedConfig(t, "", `[{"tileid":0,"duration":100},{"tileid":99,"duration":100}]`); err == nil || !strings.Contains(err.Error(), "not a tile of this set") {
		t.Errorf("frame out of range: err = %v", err)
	}
	if _, _, err := animatedConfig(t, "", `[{"tileid":0,"duration":0}]`); err == nil || !strings.Contains(err.Error(), "no duration") {
		t.Errorf("zero duration: err = %v", err)
	}
	if _, _, err := animatedConfig(t, "sometimes", `[{"tileid":0,"duration":100}]`); err == nil || !strings.Contains(err.Error(), "phase") {
		t.Errorf("bad phase: err = %v", err)
	}
}
