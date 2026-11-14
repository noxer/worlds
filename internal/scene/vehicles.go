package scene

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// Vehicles in the exploration scene: trains, ferries, buses, taxis and
// planes. Each runs on a line with a timetable (package transit); the scene
// draws those near the party, lets the party board at a stop or by walking
// into a waiting vehicle, plays the ride inside the vehicle's detail map
// with the world passing outside, and puts the party off at the other end.
//
// What differs between the kinds is in one table (vehicles): the interior,
// the doors and seats, how a line is made, where the party is put off, how
// the vehicle is drawn and what the view is. The files:
//
//   - vehicles.go: the kinds, lines near the party, boarding, the ride,
//     getting off;
//   - stops.go: stations, harbours, bus stops and airports, and their
//     timetables;
//   - vehicledraw.go: the vehicles on the main map;
//   - vehicleview.go: the world passing outside (windows, a deck, the sky);
//   - taxis.go: taxi ranks and trips.

const (
	trainMap      world.MapID = "train" // the carriage's detail map
	ferryMap      world.MapID = "ferry" // the ferry's deck
	busMap        world.MapID = "bus"   // the inside of a bus
	planeMap      world.MapID = "plane" // an airliner's cabin
	airportReach              = 400     // airports this close have their planes drawn
	vehicleReach              = 48      // lines this close to the party are drawn
	ferryReach                = 3000    // harbours this close have their ferries drawn
	viewSlowTiles             = 150     // near stations, the window shows the real place
	viewSpeed                 = 0.6     // tiles per tick the scenery passes at speed
)

// ride is the party's train journey.
type ride struct {
	line    *transit.Line
	toB     bool             // heading from A to B
	from    infra.Settlement // where it boarded
	to      infra.Settlement // where it is going
	arrived bool             // standing at the destination
}

// vehicleCar is a car drawn this frame.
type vehicleCar struct {
	transit.Car
	line *transit.Line
	st   transit.State
}

// vehicleTick is the clock as a fraction of ticks, for smooth trains.
func (e *Explore) vehicleTick(alpha float64) float64 {
	return float64(e.clock.Ticks()) + min(max(alpha, 0), 1)
}

// vehicle is how the scene treats a kind of vehicle.
type vehicle struct {
	mapID  world.MapID // the detail map it is ridden in
	exit   string      // the doors' bump id, and the dialog while they stay shut
	seat   string      // the sign (dialog id) of a seat: confirm to rest until there
	none   string      // the timetable when nothing runs
	forced string      // the notice when everyone is put off at the end

	// net is the network its lines run on, and newLine makes the line of
	// an edge of it (nil: lines are made some other way, like a taxi's).
	net     infra.Network
	newLine func(in *infra.Infra, e infra.Edge, w, h int64, seed uint64) *transit.Line

	// dropOff is where the party is put when it can't get off right
	// beside the vehicle at settlement at.
	dropOff func(e *Explore, r *ride, at infra.Settlement) (infra.Point, bool)

	// sprite names the tile of a car ("worlds" tileset) and whether to
	// mirror it; draw, if set, draws the whole vehicle instead.
	sprite func(c transit.Car) (name string, flip bool)
	draw   func(e *Explore, m world.MapRef, cars []transit.Car, cx, cy float64) []drawable

	deck bool // the view is all round the map (a deck), not through windows
	sky  bool // away from the stops the windows show the sky
}

var vehicles = map[transit.Kind]*vehicle{
	transit.Train: {
		mapID: trainMap, exit: "train_exit", seat: "train_seat",
		none: "No trains run from here.", forced: "End of the line. Everyone off, please.",
		net: infra.Rail, newLine: transit.NewTrain, sprite: trainSprite,
		dropOff: func(e *Explore, _ *ride, at infra.Settlement) (infra.Point, bool) {
			return e.infra.StationFront(at) // out in front of the station
		},
	},
	transit.Ferry: {
		mapID: ferryMap, exit: "ferry_exit", seat: "ferry_bench",
		none: "No ferries sail from here.", forced: "All ashore, please.",
		net: infra.SeaLane, newLine: transit.NewFerry, sprite: twoTileSprite("ferry", "bow", "stern"), deck: true,
		dropOff: func(e *Explore, _ *ride, at infra.Settlement) (infra.Point, bool) {
			pier, ok := e.infra.Harbour(at)
			if !ok {
				return infra.Point{}, false
			}
			return pier.Cells[len(pier.Cells)-1], true // the end of the pier
		},
	},
	transit.Bus: {
		mapID: busMap, exit: "bus_exit", seat: "train_seat",
		none: "No buses stop here.", forced: "Last stop. Everyone off, please.",
		net: infra.Road, newLine: transit.NewBus, sprite: twoTileSprite("bus", "front", "back"),
		dropOff: func(e *Explore, r *ride, at infra.Settlement) (infra.Point, bool) {
			br := e.infra.BusRoute(r.line.Edge)
			if br == nil {
				return infra.Point{}, false
			}
			if br.B.S.ID == at.ID {
				return br.B.Stop, true
			}
			return br.A.Stop, true // on the road at the stop
		},
	},
	transit.Taxi: {
		mapID: taxiMap, exit: "taxi_exit", seat: "train_seat",
		forced: "Here we are. Mind the door.",
		sprite: twoTileSprite("taxi", "front", "back"),
		dropOff: func(e *Explore, _ *ride, at infra.Settlement) (infra.Point, bool) {
			a := e.arrivalAt(at)
			return infra.Point{X: a.X, Y: a.Y}, true
		},
	},
	transit.Plane: {
		mapID: planeMap, exit: "plane_exit", seat: "plane_seat",
		none: "No flights leave from here.", forced: "Thank you for flying with us. Everyone off, please.",
		net: infra.Flight, newLine: transit.NewPlane, draw: (*Explore).planeDrawables, sky: true,
		dropOff: func(e *Explore, r *ride, at infra.Settlement) (infra.Point, bool) {
			ap, ok := e.infra.Airport(at)
			if !ok {
				return infra.Point{}, false
			}
			k, _ := ap.GateOf(r.line.Edge)
			g := ap.Gate(k)
			return infra.Point{X: g.X + 1, Y: g.Y + 3}, true // on the apron below the gate
		},
	},
}

// vehicleMap is the detail map a line's vehicle is ridden in.
func vehicleMap(l *transit.Line) world.MapID { return vehicles[l.Kind].mapID }

// vehicleByMap returns the kind ridden in a detail map.
func vehicleByMap(id world.MapID) (*vehicle, bool) {
	for _, v := range vehicles {
		if v.mapID == id {
			return v, true
		}
	}
	return nil, false
}

// isVehicleExit reports whether a bump id is some vehicle's doors.
func isVehicleExit(id string) bool {
	for _, v := range vehicles {
		if v.exit == id {
			return true
		}
	}
	return false
}

// vehicleState is what the scene keeps about vehicles.
type vehicleState struct {
	lines        map[infra.Edge]*transit.Line // by edge, made on first use
	nearLines    []*transit.Line              // lines whose vehicles may be in view
	vehicleCells map[world.Pos]vehicleCar     // the tiles vehicles stand on this tick
	vehicleTiles map[string]int               // tile indices by name
	ranks        map[world.Pos]infra.BusStop  // parked taxis near the party (see taxis.go)
	lastTaxi     *transit.Line                // the taxi just left, until it drives off
	ride         *ride                        // the party's journey, if it is on one
	viewImg      *ebiten.Image                // scratch for a window's view
}

func newVehicleState() vehicleState {
	return vehicleState{
		lines:        map[infra.Edge]*transit.Line{},
		vehicleCells: map[world.Pos]vehicleCar{},
		vehicleTiles: map[string]int{},
	}
}

// line returns the line of an edge, made on first use (nil if the edge
// has none).
func (e *Explore) line(edge infra.Edge) *transit.Line {
	if l, ok := e.lines[edge]; ok {
		return l
	}
	var l *transit.Line
	for _, v := range vehicles {
		if v.newLine != nil && v.net == edge.Net {
			l = v.newLine(e.infra, edge, e.w.W, e.w.H, e.w.Seed)
		}
	}
	if len(e.lines) > 256 {
		clear(e.lines)
	}
	e.lines[edge] = l
	return l
}

// updateVehicles finds the lines near the party and where their vehicles
// are, and runs the party's ride.
func (e *Explore) updateVehicles() {
	if e.infra == nil {
		return
	}
	l := e.party.Leader()
	if l.Pos.Map.IsMain() && (e.clock.Ticks()%60 == 0 || e.nearLines == nil) {
		e.gatherLines(l.Pos)
	}
	e.updateTaxis()
	if t := e.lastTaxi; t != nil {
		if st := t.At(float64(e.clock.Ticks())); st.Moving && !st.ToB && st.Front < t.Length-float64(vehicleReach) {
			e.lastTaxi = nil // gone back
		}
	}
	clear(e.vehicleCells)
	if l.Pos.Map.IsMain() {
		lines := e.nearLines
		if e.lastTaxi != nil {
			lines = append(lines[:len(lines):len(lines)], e.lastTaxi)
		}
		for _, line := range lines {
			st := line.At(float64(e.clock.Ticks()))
			for _, c := range line.CarsAt(st) {
				p := world.Pos{Map: world.Main, X: geom.Mod(int64(math.Round(c.X)), e.w.W), Y: geom.Mod(int64(math.Round(c.Y)), e.w.H)}
				e.vehicleCells[p] = vehicleCar{Car: c, line: line, st: st}
			}
		}
	}
	e.updateRide()
}

// gatherLines finds the lines whose vehicles may come into view round p:
// trains and buses on the ways close by, planes at the airports and
// ferries from the harbours around.
func (e *Explore) gatherLines(p world.Pos) {
	e.nearLines = e.nearLines[:0]
	seen := map[*transit.Line]bool{}
	for _, w := range e.infra.WaysIn(p.X-vehicleReach, p.Y-vehicleReach, 2*vehicleReach, 2*vehicleReach) {
		if (w.Net != infra.Rail && w.Net != infra.Road) || w.Access {
			continue
		}
		if line := e.line(w.Edge); line != nil && !seen[line] {
			seen[line] = true
			e.nearLines = append(e.nearLines, line)
		}
	}
	// Planes at the airports around.
	for _, a := range e.infra.AirportsIn(p.X-airportReach, p.Y-airportReach, 2*airportReach, 2*airportReach) {
		for _, edge := range a.Flight {
			if line := e.line(edge); line != nil && !seen[line] {
				seen[line] = true
				e.nearLines = append(e.nearLines, line)
			}
		}
	}
	// Ferries from the harbours around, once their routes are ready
	// (computed in the background with the ways).
	for _, s := range e.infra.Near(p.X, p.Y, ferryReach, infra.Town) {
		if _, ok := e.infra.Harbour(s); !ok {
			continue
		}
		for _, edge := range e.infra.Edges(infra.SeaLane, s) {
			if _, ready := e.infra.FerryRouteReady(edge); !ready {
				continue
			}
			if line := e.line(edge); line != nil && !seen[line] {
				seen[line] = true
				e.nearLines = append(e.nearLines, line)
			}
		}
	}
}

// vehicleAt returns the car on tile p, if there is one.
func (e *Explore) vehicleAt(p world.Pos) (vehicleCar, bool) {
	c, ok := e.vehicleCells[p]
	return c, ok
}

// platformBeside returns a free tile next to the train, on the platform.
func (e *Explore) platformBeside(line *transit.Line, st transit.State) (world.Pos, bool) {
	cars := line.CarsAt(st)
	onVehicle := map[world.Pos]bool{}
	pos := make([]world.Pos, len(cars))
	for k, c := range cars {
		pos[k] = world.Pos{Map: world.Main, X: geom.Mod(int64(math.Round(c.X)), e.w.W), Y: geom.Mod(int64(math.Round(c.Y)), e.w.H)}
		onVehicle[pos[k]] = true
	}
	for _, p := range pos {
		for _, d := range []world.Dir{world.Left, world.Right, world.Down, world.Up} {
			q, ok := e.w.Neighbor(p, d)
			if ok && !onVehicle[q] && !e.w.Solid(q) && e.w.Terrain(q) == world.Land {
				return q, true
			}
		}
	}
	return world.Pos{}, false
}

// board takes the party aboard a waiting train that heads for B (toB) or A.
func (e *Explore) board(line *transit.Line, toB bool) {
	r := &ride{line: line, toB: toB, from: line.Edge.A, to: line.Edge.B}
	if !toB {
		r.from, r.to = r.to, r.from
	}
	sp, ok := e.w.Spawn(vehicleMap(line), "aboard")
	if !ok {
		return
	}
	e.ride = r
	e.notice = nil
	e.warp = &world.Warp{To: sp.Pos, Facing: sp.Facing, HasFacing: sp.HasFacing}
	e.fade = fadeTicks
}

// updateRide announces the stop and puts the party off at the end.
func (e *Explore) updateRide() {
	r := e.ride
	if r == nil || e.fade != 0 {
		return
	}
	if e.party.Leader().Pos.Map.Template != vehicleMap(r.line) {
		e.ride = nil // got off
		return
	}
	st := r.line.At(float64(e.clock.Ticks()))
	// The landscape outside the windows streams in like the party's
	// surroundings do on foot.
	if vx, vy, ok := e.viewPos(0); ok {
		x, y := geom.Mod(int64(vx), e.w.W), geom.Mod(int64(vy), e.w.H)
		e.w.Prefetch(world.Pos{Map: world.Main, X: x, Y: y}, world.Right, st.Moving)
	}
	arrived := !st.Moving && st.AtA != r.toB
	if arrived && !r.arrived {
		e.say("Now arriving at " + e.infra.Name(r.to) + ".")
	}
	r.arrived = arrived
	if st.Moving && st.ToB != r.toB {
		// The train is on its way back: everyone off at the end of the line.
		e.leaveVehicle(true)
	}
}

// leaveVehicle gets off at the destination (or, before it leaves, back at
// the start); otherwise the doors stay closed. forced: the train is going
// back, everyone off.
func (e *Explore) leaveVehicle(forced bool) {
	r := e.ride
	if r == nil {
		e.warp = &world.Warp{To: e.home}
		e.fade = fadeTicks
		return
	}
	st := r.line.At(float64(e.clock.Ticks()))
	var at infra.Settlement
	switch {
	case forced || (!st.Moving && st.AtA != r.toB):
		at = r.to
	case !st.Moving: // still at the platform where it boarded
		at = r.from
	default:
		e.bump0(vehicles[r.line.Kind].exit)
		return
	}
	// Out beside the vehicle (onto the platform, the pier, the road) or,
	// failing that, to the kind's drop-off point.
	to, ok := e.platformBeside(r.line, st)
	if forced || !ok {
		front, ok := vehicles[r.line.Kind].dropOff(e, r, at)
		if !ok {
			return
		}
		to = world.Pos{Map: world.Main, X: front.X, Y: front.Y}
	}
	e.ride = nil
	if r.line.Kind == transit.Taxi {
		e.lastTaxi = r.line // it stands there until it drives back
	}
	e.warp = &world.Warp{To: to, Facing: world.Down, HasFacing: true}
	e.fade = fadeTicks
	if forced {
		e.pendingNotice = vehicles[r.line.Kind].forced
	}
}

// bump0 shows the first line of a dialog as a notice.
func (e *Explore) bump0(id string) {
	if d := e.dialogs[id]; d != nil {
		e.say(d.Nodes[d.Start].Lines[0])
	}
}

// restOffer offers to sit until the next station.
func (e *Explore) restOffer(ctx *Context) {
	r := e.ride
	d := &dialog.Dialog{ID: "rest", Start: "main", Nodes: map[string]*dialog.Node{
		"main": {Lines: []string{"Sit down until " + e.infra.Name(r.to) + "?"}, Choices: []dialog.Choice{
			{Text: "Yes, wake me there.", Do: "rest"}, {Text: "No."}}},
	}}
	run := dialog.NewRunner(d)
	ctx.Push(NewDialogBox(run, func() {
		for _, a := range run.Actions() {
			if a != "rest" || e.ride == nil {
				continue
			}
			now := e.clock.Ticks()
			atA := !e.ride.toB // the destination's end
			arrive, _ := e.ride.line.NextStop(now, atA)
			if arrive > now {
				e.clock.AdvanceTicks(arrive + 1 - now)
			}
		}
	}))
}
