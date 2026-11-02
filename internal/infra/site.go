package infra

import (
	"github.com/noxer/worlds/internal/geom"
)

// Sites are the layouts of generated villages: a square at the centre
// where the paths meet, with a well and lamps, and houses and gardens
// (prefabs) along the paths, each house connected by a footpath. A layout
// is a pure function of the seed, the terrain and the ways, computed once
// per village and cached.

// PrefabSpec describes a prefab for layouts. Houses have a door; the cell
// below it, in the prefab's bottom row, is the front step.
type PrefabSpec struct {
	Name   string
	Kind   string // "house", "garden", "well" (villages) or "building" (cities)
	Style  string // buildings: "downtown", "residential", "suburb" or "station"
	W, H   int64
	Weight int
	DoorX  int64 // column of the door (houses)
}

// Cell kinds of a site.
type SiteCell uint8

const (
	Outside SiteCell = iota // beyond the footprint
	Open                    // free ground in the footprint (few trees)
	Water                   // not dry enough to build on
	Street                  // a way (path or road) through the village
	Lane                    // a village lane from the square (dirt, like paths)
	Track                   // a rail line through the village
	Square                  // the village square (cobbles)
	Foot                    // a footpath from a front step to a street (cobbles)
	Built                   // covered by a prefab or a lamp
	Yard                    // next to a building: no trees
)

// Building is a placed prefab.
type Building struct {
	Prefab int   // index into Config.Prefabs
	X, Y   int64 // top-left tile, normalised
}

// Site is a village's layout.
type Site struct {
	S         Settlement
	X0, Y0    int64 // top-left tile of the grid, normalised
	N         int64 // edge length of the grid
	Cells     []SiteCell
	Buildings []Building
	Lamps     []Point // lamp posts (the bottom tile; the lantern is above)
}

// At returns the kind of tile (x, y), relative to the grid's corner.
func (s *Site) At(i, j int64) SiteCell {
	if i < 0 || j < 0 || i >= s.N || j >= s.N {
		return Outside
	}
	return s.Cells[j*s.N+i]
}

// siteMargin is how far the grid reaches beyond the footprint.
const siteMargin = 8

// maxSiteReach bounds how far from its centre a site has cells.
var maxSiteReach = tierRadius[Village][1] + siteMargin

// SitesIn returns the layouts of the generated villages whose grids
// overlap the rectangle of tiles (x, y, w, h).
func (in *Infra) SitesIn(x, y, w, h int64) []*Site {
	if len(in.prefabs) == 0 {
		return nil
	}
	r := maxSiteReach
	var out []*Site
	for _, s := range in.InRect(x-r, y-r, w+2*r, h+2*r, Village) {
		if s.ID.Tier != Village || s.Fixed != "" {
			continue
		}
		site := in.site(s)
		dx, dy := in.delta(site.X0, site.Y0, x, y)
		if dx < site.N && dx+w > 0 && dy < site.N && dy+h > 0 {
			out = append(out, site)
		}
	}
	return out
}

func (in *Infra) site(s Settlement) *Site {
	in.mu.Lock()
	site, ok := in.sites[s.ID]
	in.mu.Unlock()
	if ok {
		return site
	}
	site = in.layout(s)
	in.mu.Lock()
	if len(in.sites) >= siteCacheLimit {
		clear(in.sites)
	}
	in.sites[s.ID] = site
	in.mu.Unlock()
	return site
}

// siteCacheLimit bounds the site cache (about 50 KB per village).
const siteCacheLimit = 256

// rng is a deterministic random sequence (splitmix64).
type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	return z ^ z>>31
}

// intn returns a number in [0, n).
func (r *rng) intn(n int64) int64 { return int64(r.next() % uint64(n)) }

// layout plans a village.
func (in *Infra) layout(s Settlement) *Site {
	R := s.Radius + siteMargin
	n := 2*R + 1
	site := &Site{S: s, X0: geom.Mod(s.X-R, in.w), Y0: geom.Mod(s.Y-R, in.h), N: n, Cells: make([]SiteCell, n*n)}
	at := func(i, j int64) *SiteCell { return &site.Cells[j*n+i] }
	inside := func(i, j int64) bool { return i >= 0 && j >= 0 && i < n && j < n }
	pos := func(i, j int64) (int64, int64) { return geom.Mod(site.X0+i, in.w), geom.Mod(site.Y0+j, in.h) }

	// 1. The footprint: dry land in the circle, away from hand-made patches.
	for j := int64(0); j < n; j++ {
		for i := int64(0); i < n; i++ {
			di, dj := i-R, j-R
			if di*di+dj*dj > s.Radius*s.Radius {
				continue
			}
			x, y := pos(i, j)
			if in.elev(x, y) < in.sea+48 || in.avoided(x, y, 2) {
				*at(i, j) = Water
			} else {
				*at(i, j) = Open
			}
		}
	}
	// 2. The ways through it are its streets.
	in.Paint(site.X0, site.Y0, int(n), func(k int, net Network) {
		switch {
		case net == Rail && site.Cells[k] != Outside:
			site.Cells[k] = Track
		case site.Cells[k] == Open:
			site.Cells[k] = Street
		}
	})
	r := &rng{s: hash(in.seed^salt, 0x73697465, int64(s.ID.Tier), s.ID.CX, s.ID.CY)} // "site"

	// 3. The square at the centre: cobbles, the well, a lamp on each corner.
	const sq = 3 // half size: 7x7
	for j := R - sq; j <= R+sq; j++ {
		for i := R - sq; i <= R+sq; i++ {
			if c := at(i, j); *c == Open || *c == Street {
				*c = Square
			}
		}
	}
	if well, ok := in.prefabKind("well"); ok {
		pf := in.prefabs[well]
		i0, j0 := R-pf.W/2, R-pf.H/2
		if in.fits(site, i0, j0, pf.W, pf.H, Square) {
			in.mark(site, i0, j0, pf.W, pf.H, Built)
			x, y := pos(i0, j0)
			site.Buildings = append(site.Buildings, Building{Prefab: well, X: x, Y: y})
		}
	}
	for _, c := range [4][2]int64{{-sq, -sq}, {sq, -sq}, {-sq, sq}, {sq, sq}} {
		i, j := R+c[0], R+c[1]
		// The lantern sits on the overhead layer above the post.
		if *at(i, j) == Square && inside(i, j-1) && *at(i, j-1) != Water && *at(i, j-1) != Built {
			*at(i, j) = Built
			x, y := pos(i, j)
			site.Lamps = append(site.Lamps, Point{x, y})
		}
	}

	// 4. Lanes: 2-wide dirt lanes from the square in the directions no
	// way leaves it, so that houses spread round the square. At least two
	// directions have a street.
	type dir struct{ dx, dy int64 }
	dirs := []dir{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for k := len(dirs) - 1; k > 0; k-- { // shuffle
		m := r.intn(int64(k + 1))
		dirs[k], dirs[m] = dirs[m], dirs[k]
	}
	served := func(d dir) bool { // a street within 6 tiles beyond that side
		for t := int64(sq + 1); t <= sq+6; t++ {
			for o := int64(-sq); o <= sq; o++ {
				i, j := R+d.dx*t+d.dy*o, R+d.dy*t+d.dx*o
				if inside(i, j) && *at(i, j) == Street {
					return true
				}
			}
		}
		return false
	}
	streetsOut := 0
	for _, d := range dirs {
		if served(d) {
			streetsOut++
		}
	}
	for _, d := range dirs {
		if served(d) || (streetsOut >= 2 && r.intn(2) == 0) {
			continue
		}
		length := s.Radius*3/10 + r.intn(s.Radius/6+1)
		for t := int64(sq + 1); t <= length; t++ {
			stop := false
			for o := int64(0); o <= 1; o++ {
				i, j := R+d.dx*t+d.dy*o, R+d.dy*t+d.dx*o
				if !inside(i, j) || *at(i, j) != Open {
					stop = true
					break
				}
			}
			if stop {
				break
			}
			for o := int64(0); o <= 1; o++ {
				*at(R+d.dx*t+d.dy*o, R+d.dy*t+d.dx*o) = Lane
			}
		}
		streetsOut++
	}

	// 5. Houses and gardens near the streets.
	var streets [][2]int64
	for j := int64(0); j < n; j++ {
		for i := int64(0); i < n; i++ {
			if c := *at(i, j); c == Street || c == Lane || c == Square {
				// Houses gather near the square.
				if di, dj := i-R, j-R; di*di+dj*dj <= s.Radius*s.Radius*3/10 {
					streets = append(streets, [2]int64{i, j})
				}
			}
		}
	}
	if len(streets) == 0 {
		return site
	}
	target := 8 + r.intn(5) + s.Radius/25 // 10 to 16
	placed := int64(0)
	for try := int64(0); try < 80*target && placed < target; try++ {
		pi := in.pickPrefab(r)
		if pi < 0 {
			break
		}
		pf := in.prefabs[pi]
		st := streets[r.intn(int64(len(streets)))]
		// The front step (or the garden's bottom middle) a few tiles from
		// the street.
		fi, fj := st[0]+r.intn(11)-5, st[1]+r.intn(11)-5
		i0, j0 := fi-pf.DoorX, fj-(pf.H-1)
		if pf.Kind != "house" {
			i0 = fi - pf.W/2
		}
		// The prefab and a ring around it must be open ground.
		if !in.fits(site, i0-1, j0-1, pf.W+2, pf.H+2, Open, Yard) {
			continue
		}
		// Mark it (the footpath must not run through it), then connect
		// the front step; without a footpath, undo.
		saved := make([]SiteCell, 0, (pf.W+2)*(pf.H+2))
		for j := j0 - 1; j <= j0+pf.H; j++ {
			for i := i0 - 1; i <= i0+pf.W; i++ {
				saved = append(saved, *at(i, j))
			}
		}
		in.mark(site, i0-1, j0-1, pf.W+2, pf.H+2, Yard)
		in.mark(site, i0, j0, pf.W, pf.H, Built)
		if pf.Kind == "house" {
			*at(fi, fj) = Open // the front step
			path, ok := connect(site, fi, fj, 14)
			if !ok {
				k := 0
				for j := j0 - 1; j <= j0+pf.H; j++ {
					for i := i0 - 1; i <= i0+pf.W; i++ {
						*at(i, j) = saved[k]
						k++
					}
				}
				continue
			}
			for _, c := range path {
				*at(c[0], c[1]) = Foot
			}
		}
		x, y := pos(i0, j0)
		site.Buildings = append(site.Buildings, Building{Prefab: pi, X: x, Y: y})
		placed++
	}
	return site
}

// fits reports whether all cells of a rectangle of the grid are of the
// given kinds.
func (in *Infra) fits(site *Site, i0, j0, w, h int64, kinds ...SiteCell) bool {
	for j := j0; j < j0+h; j++ {
		for i := i0; i < i0+w; i++ {
			c, ok := site.At(i, j), false
			for _, k := range kinds {
				ok = ok || c == k
			}
			if !ok {
				return false
			}
		}
	}
	return true
}

// mark sets the cells of a rectangle; Yard only marks open ground.
func (in *Infra) mark(site *Site, i0, j0, w, h int64, k SiteCell) {
	for j := max(j0, 0); j < min(j0+h, site.N); j++ {
		for i := max(i0, 0); i < min(i0+w, site.N); i++ {
			c := &site.Cells[j*site.N+i]
			if k != Yard || *c == Open {
				*c = k
			}
		}
	}
}

// connect finds the shortest footpath (4 directions, over open ground and
// yards) from a front step to a street, the square or another footpath,
// at most max steps long. The path includes the step, not the street.
func connect(site *Site, fi, fj, maxLen int64) ([][2]int64, bool) {
	type node struct{ i, j int64 }
	start := node{fi, fj}
	if c := site.At(fi, fj); c != Open && c != Yard {
		return nil, false
	}
	prev := map[node]node{start: start}
	frontier := []node{start}
	for d := int64(0); d < maxLen && len(frontier) > 0; d++ {
		var next []node
		for _, p := range frontier {
			for _, dd := range [4][2]int64{{0, 1}, {-1, 0}, {1, 0}, {0, -1}} {
				q := node{p.i + dd[0], p.j + dd[1]}
				if _, seen := prev[q]; seen {
					continue
				}
				switch site.At(q.i, q.j) {
				case Street, Lane, Square, Foot:
					var path [][2]int64
					for c := p; ; c = prev[c] {
						path = append(path, [2]int64{c.i, c.j})
						if c == start {
							return path, true
						}
					}
				case Open, Yard:
					prev[q] = p
					next = append(next, q)
				}
			}
		}
		frontier = next
	}
	return nil, false
}

// pickPrefab picks a house or garden by weight, or -1 if there is none.
func (in *Infra) pickPrefab(r *rng) int {
	total := 0
	for _, p := range in.prefabs {
		if p.Kind == "house" || p.Kind == "garden" {
			total += p.Weight
		}
	}
	if total == 0 {
		return -1
	}
	k := int(r.intn(int64(total)))
	for i, p := range in.prefabs {
		if p.Kind == "house" || p.Kind == "garden" {
			if k < p.Weight {
				return i
			}
			k -= p.Weight
		}
	}
	return -1
}

func (in *Infra) prefabKind(kind string) (int, bool) {
	for i, p := range in.prefabs {
		if p.Kind == kind {
			return i, true
		}
	}
	return 0, false
}
