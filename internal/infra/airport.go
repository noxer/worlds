package infra

import "github.com/noxer/worlds/internal/geom"

// Airports: a metropolis with flights gets an airport just outside it: a
// fenced field with a runway along its north side, an apron with a gate
// per flight, a terminal and a departures board, and an opening in the
// fence on the south side. Planes taxi from their gate to the runway's
// west end, take off eastwards, fly to the other airport (along the axes,
// one corner), land eastwards on its runway and taxi to their gate there.

// Airport layout, in tiles relative to the airport's top-left corner.
const (
	AirportW = 60
	AirportH = 22

	runwayY0, runwayY1 = 3, 6  // runway rows (edges at y0 and y1)
	apronY0, apronY1   = 7, 20 // apron rows
	GateY              = 10    // the row planes stand on at their gates
	gateX0, gateStep   = 12, 7 // first gate's west end, and the step between gates
	taxiX              = 4     // the column planes taxi along to the runway
	TerminalX          = 3     // the terminal's top-left (a prefab)
	TerminalY          = 13
	BoardX, BoardY     = 26, 14 // the departures board's post
	entranceX0         = 28     // the gap in the south fence
	entranceX1         = 31
)

// AirportCell is what a tile of an airport is.
type AirportCell uint8

const (
	AOutside AirportCell = iota
	AFence
	AGrass
	ARunwayEdgeN
	ARunway
	ARunwayMark // the centre line
	ARunwayEdgeS
	AApron
	ABoard // the departures board's post (bump "airport")
)

// Airport is a metropolis's airport.
type Airport struct {
	S        Settlement
	X, Y     int64 // top-left corner, normalised
	Gates    int
	Flight   []Edge   // the flights, by gate
	Terminal Building // the terminal (a city prefab); Prefab < 0 if none
}

// terminalPrefab is the prefab the terminal is, by name (or any downtown
// building that fits).
const terminalPrefab = "city_down_red_8x7"

func (in *Infra) terminal(x0, y0 int64) Building {
	pick := -1
	for i, p := range in.prefabs {
		if p.Name == terminalPrefab {
			pick = i
			break
		}
		if pick < 0 && p.Kind == "building" && p.Style == "downtown" && p.W <= 10 && p.H <= apronY1-TerminalY+1 {
			pick = i
		}
	}
	return Building{Prefab: pick, X: geom.Mod(x0+TerminalX, in.w), Y: geom.Mod(y0+TerminalY, in.h)}
}

// Airport returns the airport of settlement s, if it has one.
func (in *Infra) Airport(s Settlement) (Airport, bool) {
	if s.ID.Tier != Metropolis || s.Fixed != "" {
		return Airport{}, false
	}
	in.mu.Lock()
	a, ok := in.airports[s.ID]
	in.mu.Unlock()
	if !ok {
		a = in.airport(s)
		in.mu.Lock()
		if len(in.airports) >= siteCacheLimit {
			clear(in.airports)
		}
		in.airports[s.ID] = a
		in.mu.Unlock()
	}
	return a, a.Gates > 0
}

func (in *Infra) airport(s Settlement) Airport {
	es := in.rng(Flight, s)
	if len(es) == 0 {
		return Airport{}
	}
	var plan *CityPlan
	if len(in.prefabs) > 0 {
		plan = in.city(s)
	}
	// Round the city, nearest first: the first spot that is dry and clear
	// of the city and hand-made places. (The ways to the city keep out of
	// it: see airportRects.)
	for _, r := range []int64{s.Radius + 60, s.Radius + 160, s.Radius + 300} {
		for _, d := range [8][2]int64{{0, 1}, {1, 0}, {-1, 0}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
			cx, cy := s.X+d[0]*r, s.Y+d[1]*r
			x0, y0 := cx-AirportW/2, cy-AirportH/2
			if in.airportFits(plan, x0, y0) {
				n := min(len(es), (AirportW-gateX0-4)/gateStep)
				return Airport{S: s, X: geom.Mod(x0, in.w), Y: geom.Mod(y0, in.h), Gates: n, Flight: es[:n],
					Terminal: in.terminal(x0, y0)}
			}
		}
	}
	return Airport{}
}

func (in *Infra) airportFits(plan *CityPlan, x0, y0 int64) bool {
	const m = 3 // a margin of dry land
	for y := y0 - m; y <= y0+AirportH+m; y += 2 {
		for x := x0 - m; x <= x0+AirportW+m; x += 2 {
			p := in.norm(x, y)
			if in.elev(p.X, p.Y) < in.sea+48 || in.avoided(p.X, p.Y, 4) || (plan != nil && plan.Covers(p.X, p.Y)) {
				return false
			}
		}
	}
	return true
}

// airportRects returns the airports (with a margin) of an edge's ends,
// which its route keeps out of.
func (in *Infra) airportRects(e Edge) []Rect {
	if e.Net == Flight || e.Net == SeaLane {
		return nil
	}
	var out []Rect
	for _, s := range []Settlement{e.A, e.B} {
		if s.ID.Tier != Metropolis {
			continue
		}
		if a, ok := in.Airport(s); ok {
			out = append(out, Rect{X: geom.Mod(a.X-2, in.w), Y: geom.Mod(a.Y-2, in.h), W: AirportW + 4, H: AirportH + 4})
		}
	}
	return out
}

// At returns what tile (dx, dy) relative to the airport's corner is.
func (a Airport) At(dx, dy int64) AirportCell {
	switch {
	case dx < 0 || dy < 0 || dx >= AirportW || dy >= AirportH:
		return AOutside
	case dx == 0 || dx == AirportW-1 || dy == 0:
		return AFence
	case dy == AirportH-1:
		if dx >= entranceX0 && dx < entranceX1 {
			return AApron // the way in
		}
		return AFence
	case dy == runwayY0:
		return ARunwayEdgeN
	case dy == runwayY1:
		return ARunwayEdgeS
	case dy > runwayY0 && dy < runwayY1:
		if dy == (runwayY0+runwayY1)/2 && dx%6 < 3 {
			return ARunwayMark
		}
		return ARunway
	case dy >= apronY0 && dy <= apronY1:
		if dx == BoardX && dy == BoardY {
			return ABoard
		}
		return AApron
	}
	return AGrass
}

// Gate returns the west end of gate k (planes stand on it and the two
// tiles east of it).
func (a Airport) Gate(k int) Point {
	return Point{a.X + gateX0 + int64(k)*gateStep, a.Y + GateY}
}

// GateOf returns the gate of flight e at this airport.
func (a Airport) GateOf(e Edge) (int, bool) {
	for k, f := range a.Flight {
		if f.key() == e.key() {
			return k, true
		}
	}
	return 0, false
}

// Board returns the departures board's post.
func (a Airport) Board() Point { return Point{a.X + BoardX, a.Y + BoardY} }

// FlightRoute returns the route of the plane of flight e, in tiles, each
// point the shortest way round the world from the one before: from its
// gate at A (the tail; it stands facing west) to the runway's west end,
// along the runway, across to B's runway (one corner), along it and back
// to its gate at B. Nil if either end has no airport or no gate for it.
func (in *Infra) FlightRoute(e Edge) []Point {
	if e.Net != Flight {
		return nil
	}
	a, okA := in.Airport(e.A)
	b, okB := in.Airport(e.B)
	if !okA || !okB {
		return nil
	}
	ga, okA := a.GateOf(e)
	gb, okB := b.GateOf(e)
	if !okA || !okB {
		return nil
	}
	ry := int64((runwayY0 + runwayY1) / 2)
	gA, gB := a.Gate(ga), b.Gate(gb)
	pts := []Point{
		{gA.X + 2, gA.Y},               // the tail at the gate
		{a.X + taxiX, gA.Y},            // west along the apron
		{a.X + taxiX, a.Y + ry},        // onto the runway
		{a.X + AirportW - 6, a.Y + ry}, // take-off run
	}
	land := []Point{
		{b.X + taxiX, b.Y + ry},        // touch down at the west end
		{b.X + AirportW - 6, b.Y + ry}, // the landing roll
		{b.X + AirportW - 6, gB.Y},     // off the runway
		{gB.X, gB.Y},                   // west along the apron to the gate
	}
	out := []Point{pts[0]}
	add := func(p Point) { out = append(out, in.unwrap(out[len(out)-1], p)) }
	for _, p := range pts[1:] {
		add(p)
	}
	// Across, along the axes: east-west first, then north-south, then in
	// to the runway's west end from the west.
	last := out[len(out)-1]
	t := in.unwrap(last, land[0])
	approach := Point{t.X - 40, t.Y}
	add(Point{approach.X, last.Y})
	add(approach)
	for _, p := range land {
		add(p)
	}
	return out
}

// AirportsIn returns the airports overlapping the rectangle of tiles (x, y,
// w, h).
func (in *Infra) AirportsIn(x, y, w, h int64) []Airport {
	r := tierRadius[Metropolis][1] + 300 + AirportW
	var out []Airport
	for _, s := range in.InRect(x-r, y-r, w+2*r, h+2*r, Metropolis) {
		a, ok := in.Airport(s)
		if !ok {
			continue
		}
		dx, dy := in.delta(x, y, a.X, a.Y)
		if dx < w && dx+AirportW > 0 && dy < h && dy+AirportH > 0 {
			out = append(out, a)
		}
	}
	return out
}
