// Package infra places the world's infrastructure: settlements, and the
// networks between them (paths, roads, highways, rail, sea lanes, flights).
//
// Everything is a pure function of the world seed and the terrain's
// elevation, evaluated locally: "which settlements are near here" and "what
// does this route look like" never need a pass over the whole planet, so
// chunks can still be generated in any order. Results are cached.
//
// See "Infrastructure: settlements and networks" in ARCHITECTURE.md.
package infra

import (
	"fmt"
	"sort"
	"sync"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/world"
)

// Tier is the size class of a settlement.
type Tier uint8

const (
	POI        Tier = iota // point of interest: farm, camp, ruin, cave entrance
	Village                // about 20 minutes apart on foot
	Town                   // about 75 minutes apart
	City                   // a few hours apart
	Metropolis             // a handful on the planet, up to 2 km across
	NumTiers
)

var tierNames = [NumTiers]string{"poi", "village", "town", "city", "metropolis"}

func (t Tier) String() string {
	if t < NumTiers {
		return tierNames[t]
	}
	return fmt.Sprintf("tier(%d)", t)
}

// ParseTier parses a tier name ("village", ...).
func ParseTier(s string) (Tier, error) {
	for t, n := range tierNames {
		if n == s {
			return Tier(t), nil
		}
	}
	return 0, fmt.Errorf("unknown settlement tier %q", s)
}

// Each tier has a grid with at most one settlement per cell.
var tierCell = [NumTiers]int64{1024, 4096, 16384, 65536, 131072}

// Footprint radius range per tier, in tiles.
var tierRadius = [NumTiers][2]int64{{8, 24}, {50, 100}, {150, 300}, {400, 600}, {700, 1000}}

// Chance in percent that a cell with a suitable site gets a settlement.
var tierChance = [NumTiers]int64{70, 90, 100, 100, 100}

// Cell returns the grid cell size of a tier in tiles.
func (t Tier) Cell() int64 { return tierCell[t] }

// candidates is the number of sites tried per cell.
const candidates = 6

// ID identifies a settlement by its tier and grid cell.
type ID struct {
	Tier   Tier
	CX, CY int64
}

func (id ID) less(o ID) bool {
	if id.Tier != o.Tier {
		return id.Tier < o.Tier
	}
	if id.CY != o.CY {
		return id.CY < o.CY
	}
	return id.CX < o.CX
}

// Settlement is a place on the main map.
type Settlement struct {
	ID      ID
	X, Y    int64  // centre, normalised
	Radius  int64  // footprint radius in tiles
	Coastal bool   // water close to the footprint: can have a harbour
	Fixed   string // name of the hand-made patch, if not generated
}

// Relief is the terrain the infrastructure adapts to (implemented by the
// earth generator).
type Relief interface {
	// Elevation at a main-map position, on the generator's scale.
	Elevation(seed uint64, x, y int64) int64
	// Height is the smooth large-scale height (no small detail), for
	// slopes.
	Height(seed uint64, x, y int64) int64
	// Levels returns the elevations below which there is water and deep
	// water.
	Levels() (sea, deep int64)
}

// Fixed is a hand-made settlement (a patch with the "settlement" property).
type Fixed struct {
	Name   string
	Tier   Tier
	X, Y   int64 // centre
	Radius int64
	// Ends are the gates where routes start (objects of type "road_end");
	// without any, routes start at the centre.
	Ends []Gate
}

// Gate is where routes enter a hand-made settlement: a tile on the border
// of its patch, and the outward direction.
type Gate struct {
	Point
	DX, DY int64
}

// Rect is a rectangle of tiles on the main map.
type Rect struct{ X, Y, W, H int64 }

// FixedFrom collects the hand-made settlements among the patches: those
// with the property "settlement" (a tier name), centred on the patch, with
// their "road_end" objects as gates.
func FixedFrom(patches []world.PatchInfo) ([]Fixed, error) {
	var out []Fixed
	for _, p := range patches {
		s := p.Props.String("settlement")
		if s == "" {
			continue
		}
		t, err := ParseTier(s)
		if err != nil {
			return nil, fmt.Errorf("patch %s: %w", p.Name, err)
		}
		f := Fixed{Name: string(p.Name), Tier: t, X: p.X + p.W/2, Y: p.Y + p.H/2, Radius: max(p.W, p.H) / 2}
		for _, o := range p.Objects {
			if o.Type != "road_end" {
				continue
			}
			g := Gate{Point: Point{p.X + o.Pos.X, p.Y + o.Pos.Y}}
			switch {
			case o.Pos.Y == 0:
				g.DY = -1
			case o.Pos.Y == p.H-1:
				g.DY = 1
			case o.Pos.X == 0:
				g.DX = -1
			case o.Pos.X == p.W-1:
				g.DX = 1
			default:
				return nil, fmt.Errorf("patch %s: road_end %q must be on the patch border", p.Name, o.Name)
			}
			f.Ends = append(f.Ends, g)
		}
		out = append(out, f)
	}
	return out, nil
}

// Config configures the infrastructure of a world.
type Config struct {
	Seed   uint64
	W, H   int64
	Relief Relief
	Fixed  []Fixed
	Avoid  []Rect // hand-made patches: routes go round them
	// Prefabs are the pieces village layouts place (Building.Prefab
	// indexes this list).
	Prefabs []PrefabSpec
}

// Infra answers questions about settlements and networks. It is safe for
// concurrent use.
type Infra struct {
	seed      uint64
	w, h      int64
	relief    Relief
	sea, deep int64
	fixed     []Settlement
	fixedAt   map[ID]Settlement
	ends      map[ID][]Gate // gates of hand-made settlements
	avoid     []Rect
	prefabs   []PrefabSpec

	mu     sync.Mutex
	cells  map[ID]cellResult
	routes map[edgeKey]*Route
	blocks map[[2]int64][]*Way
	ways   map[edgeKey]*Way
	sites  map[ID]*Site
	piers  map[ID]Pier
	cities map[ID]*CityPlan

	prefetchAt      [2]int64
	prefetchStarted bool
	prefetchQueue   chan [2]int64
}

// avoided reports whether tile (x, y) is within m tiles of a hand-made
// patch.
func (in *Infra) avoided(x, y, m int64) bool {
	for _, r := range in.avoid {
		dx, dy := in.delta(r.X, r.Y, x, y)
		if dx >= -m && dx < r.W+m && dy >= -m && dy < r.H+m {
			return true
		}
	}
	return false
}

type cellResult struct {
	s  Settlement
	ok bool
}

// cacheLimit bounds the caches; they are simply cleared when full.
const cacheLimit = 1 << 18

// New creates the infrastructure of a world.
func New(cfg Config) (*Infra, error) {
	for t := Tier(0); t < NumTiers; t++ {
		if c := tierCell[t]; cfg.W%c != 0 || cfg.H%c != 0 {
			return nil, fmt.Errorf("infra: world size %dx%d must be a multiple of %d", cfg.W, cfg.H, c)
		}
	}
	if cfg.Relief == nil {
		return nil, fmt.Errorf("infra: no relief")
	}
	in := &Infra{
		seed: cfg.Seed, w: cfg.W, h: cfg.H, relief: cfg.Relief,
		fixedAt: map[ID]Settlement{},
		ends:    map[ID][]Gate{},
		avoid:   cfg.Avoid,
		prefabs: cfg.Prefabs,
		sites:   map[ID]*Site{},
		piers:   map[ID]Pier{},
		cities:  map[ID]*CityPlan{},
		ways:    map[edgeKey]*Way{},
		cells:   map[ID]cellResult{},
		routes:  map[edgeKey]*Route{},
		blocks:  map[[2]int64][]*Way{},
	}
	in.sea, in.deep = cfg.Relief.Levels()
	for _, f := range cfg.Fixed {
		x, y := geom.Mod(f.X, cfg.W), geom.Mod(f.Y, cfg.H)
		id := ID{f.Tier, x / tierCell[f.Tier], y / tierCell[f.Tier]}
		if other, dup := in.fixedAt[id]; dup {
			return nil, fmt.Errorf("infra: settlements %s and %s share a %s cell", other.Fixed, f.Name, f.Tier)
		}
		s := Settlement{ID: id, X: x, Y: y, Radius: f.Radius, Fixed: f.Name}
		s.Coastal = in.coastal(x, y, f.Radius)
		in.fixedAt[id] = s
		in.fixed = append(in.fixed, s)
		for _, g := range f.Ends {
			g.X, g.Y = geom.Mod(g.X, cfg.W), geom.Mod(g.Y, cfg.H)
			in.ends[id] = append(in.ends[id], g)
		}
	}
	return in, nil
}

// hash mixes the seed and values (splitmix64, as gen.Hash).
func hash(seed uint64, vals ...int64) uint64 {
	h := seed ^ 0x9e3779b97f4a7c15
	for _, v := range vals {
		h ^= uint64(v)
		h += 0x9e3779b97f4a7c15
		h = (h ^ h>>30) * 0xbf58476d1ce4e5b9
		h = (h ^ h>>27) * 0x94d049bb133111eb
		h ^= h >> 31
	}
	return h
}

// salt separates infra's random numbers from the terrain's.
const salt = 0x696e667261 // "infra"

func (in *Infra) elev(x, y int64) int64 { return in.relief.Elevation(in.seed, x, y) }

// delta returns the shortest offset from a to b on the torus.
func (in *Infra) delta(ax, ay, bx, by int64) (int64, int64) {
	return geom.WrapDelta(ax, bx, in.w), geom.WrapDelta(ay, by, in.h)
}

// dist2 is the squared distance between two settlements.
func (in *Infra) dist2(a, b Settlement) int64 {
	dx, dy := in.delta(a.X, a.Y, b.X, b.Y)
	return dx*dx + dy*dy
}

// Get returns the settlement of a grid cell, if it has one.
func (in *Infra) Get(id ID) (Settlement, bool) {
	id.CX = geom.Mod(id.CX, in.w/tierCell[id.Tier])
	id.CY = geom.Mod(id.CY, in.h/tierCell[id.Tier])
	in.mu.Lock()
	r, ok := in.cells[id]
	in.mu.Unlock()
	if ok {
		return r.s, r.ok
	}
	// Computed without the lock: it looks up bigger tiers recursively, and
	// two goroutines computing the same cell get the same result.
	s, found := in.compute(id)
	in.mu.Lock()
	if len(in.cells) >= cacheLimit {
		clear(in.cells)
	}
	in.cells[id] = cellResult{s, found}
	in.mu.Unlock()
	return s, found
}

// ring are unit directions (x, y in 1/16) for sampling around a site.
var ring = [8][2]int64{{16, 0}, {11, 11}, {0, 16}, {-11, 11}, {-16, 0}, {-11, -11}, {0, -16}, {11, -11}}

func (in *Infra) compute(id ID) (Settlement, bool) {
	if s, ok := in.fixedAt[id]; ok {
		return s, true
	}
	t := id.Tier
	cell := tierCell[t]
	h := hash(in.seed^salt, int64(t), id.CX, id.CY)
	if int64(h%100) >= tierChance[t] {
		return Settlement{}, false
	}
	lo, hi := tierRadius[t][0], tierRadius[t][1]
	r := lo + int64(h>>8%uint64(hi-lo+1))
	best, bestScore := Settlement{}, int64(-1)
	for k := int64(0); k < candidates; k++ {
		hk := hash(in.seed^salt, int64(t), id.CX, id.CY, k)
		// Sites keep away from the cell border, so neighbouring
		// settlements of a tier are at least 30% of a cell apart.
		x := id.CX*cell + cell*15/100 + int64(hk%uint64(cell*70/100))
		y := id.CY*cell + cell*15/100 + int64(hk>>32%uint64(cell*70/100))
		score, ok := in.score(x, y, r)
		if !ok {
			continue
		}
		score += int64(hk >> 56 % 10) // a little randomness between equals
		if score > bestScore {
			best, bestScore = Settlement{ID: id, X: x, Y: y, Radius: r}, score
		}
	}
	if bestScore < 0 {
		return Settlement{}, false
	}
	best.Coastal = in.coastal(best.X, best.Y, best.Radius)
	if in.suppressed(best) {
		return Settlement{}, false
	}
	return best, true
}

// score rates a site: mostly land, flat, by the coast. ok is false where
// a settlement can't be.
func (in *Infra) score(x, y, r int64) (int64, bool) {
	c := in.elev(x, y)
	if c < in.sea+64 {
		return 0, false // the centre must be dry land
	}
	land, lo, hi := 0, c, c
	for _, f := range []int64{1, 2} { // at r/2 and r
		for _, d := range ring {
			e := in.elev(x+d[0]*r*f/32, y+d[1]*r*f/32)
			if e >= in.sea {
				land++
				lo, hi = min(lo, e), max(hi, e)
			}
		}
	}
	if land < 13 { // at least 13 of 16 samples on land
		return 0, false
	}
	score := int64(land)*100 - (hi-lo)/4
	if in.coastal(x, y, r) {
		score += 300
	}
	return max(score, 0), true
}

// coastal reports whether there is water within 1.5 radii (at least 32
// tiles) of a footprint's centre.
func (in *Infra) coastal(x, y, r int64) bool {
	d := max(r*3/2, 32)
	for _, v := range ring {
		if in.elev(x+v[0]*d/16, y+v[1]*d/16) < in.sea {
			return true
		}
	}
	return false
}

// gap is the minimum free distance between the footprint of a settlement
// of tier t and any bigger one.
func gap(t Tier) int64 { return tierCell[t] / 4 }

// suppressed reports whether a bigger settlement, or a hand-made one of the
// same tier or bigger, is too close to s.
func (in *Infra) suppressed(s Settlement) bool {
	t := s.ID.Tier
	tooClose := func(o Settlement) bool {
		d := s.Radius + o.Radius + gap(t)
		return in.dist2(s, o) < d*d
	}
	for _, f := range in.fixed {
		if f.ID.Tier >= t && tooClose(f) {
			return true
		}
	}
	for u := t + 1; u < NumTiers; u++ {
		cell := tierCell[u]
		cx, cy := s.X/cell, s.Y/cell
		for dy := int64(-1); dy <= 1; dy++ {
			for dx := int64(-1); dx <= 1; dx++ {
				if o, ok := in.Get(ID{u, cx + dx, cy + dy}); ok && tooClose(o) {
					return true
				}
			}
		}
	}
	return false
}

// Near returns the settlements of tier min and bigger whose centres are
// within r tiles of (x, y), in a stable order.
func (in *Infra) Near(x, y, r int64, minTier Tier) []Settlement {
	var out []Settlement
	for t := minTier; t < NumTiers; t++ {
		cell := tierCell[t]
		nx, ny := in.w/cell, in.h/cell
		x0, x1 := geom.FloorDiv(x-r, cell), geom.FloorDiv(x+r, cell)
		y0, y1 := geom.FloorDiv(y-r, cell), geom.FloorDiv(y+r, cell)
		if x1-x0+1 > nx {
			x0, x1 = 0, nx-1
		}
		if y1-y0+1 > ny {
			y0, y1 = 0, ny-1
		}
		for cy := y0; cy <= y1; cy++ {
			for cx := x0; cx <= x1; cx++ {
				s, ok := in.Get(ID{t, cx, cy})
				if !ok {
					continue
				}
				dx, dy := in.delta(x, y, s.X, s.Y)
				if dx*dx+dy*dy <= r*r {
					out = append(out, s)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.less(out[j].ID) })
	return out
}

// InRect returns the settlements of tier min and bigger whose centres lie
// in the rectangle (x, y, w, h), which may cross the seams.
func (in *Infra) InRect(x, y, w, h int64, minTier Tier) []Settlement {
	cx, cy := x+w/2, y+h/2
	r := (max(w, h) + 1) / 2 * 3 / 2 // covers the rectangle's corners
	var out []Settlement
	for _, s := range in.Near(cx, cy, r, minTier) {
		dx, dy := in.delta(x, y, s.X, s.Y)
		if dx < 0 {
			dx += in.w
		}
		if dy < 0 {
			dy += in.h
		}
		if dx < w && dy < h {
			out = append(out, s)
		}
	}
	return out
}

// Supports reports whether a world of w x h tiles can have infrastructure:
// its size must be a multiple of every tier's grid.
func Supports(w, h int64) bool {
	c := tierCell[NumTiers-1]
	return w > 0 && h > 0 && w%c == 0 && h%c == 0
}
