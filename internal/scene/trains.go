package scene

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/transit"
	"github.com/noxer/worlds/internal/world"
)

// Trains in the exploration scene: drawn on the main map from their
// timetables (package transit), boarded at a station's door or by walking
// into a waiting train, ridden inside the "train" detail map with the world
// passing by its windows, and left at the next station.

// Ferries work the same way on the sea lanes: drawn from their timetables
// off the pier tips, boarded from the end of the pier, ridden on the
// "ferry" deck with the sea all round.

const (
	trainMap      world.MapID = "train" // the carriage's detail map
	ferryMap      world.MapID = "ferry" // the ferry's deck
	busMap        world.MapID = "bus"   // the inside of a bus
	trainReach                = 48      // lines this close to the party are drawn
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

// trainCar is a car drawn this frame.
type trainCar struct {
	transit.Car
	line *transit.Line
	st   transit.State
}

// trainTick is the clock as a fraction of ticks, for smooth trains.
func (e *Explore) trainTick(alpha float64) float64 {
	return float64(e.clock.Ticks()) + min(max(alpha, 0), 1)
}

// vehicle is how the scene treats a kind of vehicle.
type vehicle struct {
	mapID  world.MapID // the detail map it is ridden in
	exit   string      // the doors' bump id, and the dialog while they stay shut
	none   string      // the timetable when nothing runs
	forced string      // the notice when everyone is put off at the end
}

var vehicles = map[transit.Kind]vehicle{
	transit.Train: {trainMap, "train_exit", "No trains run from here.", "End of the line. Everyone off, please."},
	transit.Ferry: {ferryMap, "ferry_exit", "No ferries sail from here.", "All ashore, please."},
	transit.Bus:   {busMap, "bus_exit", "No buses stop here.", "Last stop. Everyone off, please."},
}

// vehicleMap is the detail map a line's vehicle is ridden in.
func vehicleMap(l *transit.Line) world.MapID { return vehicles[l.Kind].mapID }

// line returns the line of a rail edge or sea lane, made on first use.
func (e *Explore) line(edge infra.Edge) *transit.Line {
	if l, ok := e.lines[edge]; ok {
		return l
	}
	var l *transit.Line
	switch edge.Net {
	case infra.SeaLane:
		l = transit.NewFerry(e.infra, edge, e.w.W, e.w.H, e.w.Seed)
	case infra.Road:
		l = transit.NewBus(e.infra, edge, e.w.W, e.w.H, e.w.Seed)
	default:
		l = transit.NewLine(e.infra, edge, e.w.W, e.w.H, e.w.Seed)
	}
	if len(e.lines) > 256 {
		clear(e.lines)
	}
	e.lines[edge] = l
	return l
}

// updateTrains finds the lines near the party, where their trains are,
// and runs the party's ride.
func (e *Explore) updateTrains() {
	if e.infra == nil {
		return
	}
	l := e.party.Leader()
	if l.Pos.Map.IsMain() && (e.clock.Ticks()%60 == 0 || e.nearLines == nil) {
		e.nearLines = e.nearLines[:0]
		seen := map[*transit.Line]bool{}
		for _, w := range e.infra.WaysIn(l.Pos.X-trainReach, l.Pos.Y-trainReach, 2*trainReach, 2*trainReach) {
			if w.Net != infra.Rail && w.Net != infra.Road {
				continue
			}
			if line := e.line(w.Edge); line != nil && !seen[line] {
				seen[line] = true
				e.nearLines = append(e.nearLines, line)
			}
		}
		// Ferries from the harbours around, once their routes are ready
		// (computed in the background with the ways).
		for _, s := range e.infra.Near(l.Pos.X, l.Pos.Y, ferryReach, infra.Town) {
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
	clear(e.trainCells)
	if l.Pos.Map.IsMain() {
		for _, line := range e.nearLines {
			st := line.At(float64(e.clock.Ticks()))
			for _, c := range line.CarsAt(st) {
				p := world.Pos{Map: world.Main, X: geom.Mod(int64(math.Round(c.X)), e.w.W), Y: geom.Mod(int64(math.Round(c.Y)), e.w.H)}
				e.trainCells[p] = trainCar{Car: c, line: line, st: st}
			}
		}
	}
	e.updateRide()
}

// trainAt returns the car on tile p, if there is one.
func (e *Explore) trainAt(p world.Pos) (trainCar, bool) {
	c, ok := e.trainCells[p]
	return c, ok
}

// stationAt returns the city whose station tile p (a timetable board) is.
func (e *Explore) stationAt(p world.Pos) (infra.Settlement, bool) {
	for _, s := range e.infra.Near(p.X, p.Y, 200, infra.City) {
		if e.infra.InStation(s, p.X, p.Y) {
			return s, true
		}
	}
	return infra.Settlement{}, false
}

// harbourAt returns the settlement whose pier tip (the water off the end
// of the pier, where the ferry moors) tile p is.
func (e *Explore) harbourAt(p world.Pos) (infra.Settlement, bool) {
	for _, s := range e.infra.Near(p.X, p.Y, ferryReach, infra.Town) {
		if pier, ok := e.infra.Harbour(s); ok && pier.Tip.X == p.X && pier.Tip.Y == p.Y {
			return s, true
		}
	}
	return infra.Settlement{}, false
}

// platformBeside returns a free tile next to the train, on the platform.
func (e *Explore) platformBeside(line *transit.Line, st transit.State) (world.Pos, bool) {
	cars := line.CarsAt(st)
	onTrain := map[world.Pos]bool{}
	pos := make([]world.Pos, len(cars))
	for k, c := range cars {
		pos[k] = world.Pos{Map: world.Main, X: geom.Mod(int64(math.Round(c.X)), e.w.W), Y: geom.Mod(int64(math.Round(c.Y)), e.w.H)}
		onTrain[pos[k]] = true
	}
	for _, p := range pos {
		for _, d := range []world.Dir{world.Left, world.Right, world.Down, world.Up} {
			q, ok := e.w.Neighbor(p, d)
			if ok && !onTrain[q] && !e.w.Solid(q) && e.w.Terrain(q) == world.Land {
				return q, true
			}
		}
	}
	return world.Pos{}, false
}

// trainBump handles walking into a train, a station door or the train's
// doors; it reports whether it did.
func (e *Explore) trainBump(p world.Pos) bool {
	if e.infra == nil {
		return false
	}
	if c, ok := e.trainAt(p); ok {
		if !c.st.Moving {
			e.board(c.line, c.st.ToB)
		}
		return true
	}
	id, ok := e.bumps[p]
	if !ok {
		id, ok = e.w.Bump(p)
	}
	switch {
	case ok && id == "station":
		if s, ok := e.stationAt(p); ok {
			e.say(e.infra.Name(s) + " Station: departures. Press to read the timetable.")
			return true
		}
	case ok && id == "bus_stop":
		if s, _, ok := e.busStopAt(p); ok {
			e.say(e.infra.Name(s) + " bus stop. Press to read the timetable.")
			return true
		}
	case ok && (id == "train_exit" || id == "ferry_exit" || id == "bus_exit"):
		e.leaveTrain(false)
		return true
	}
	return false
}

// trainTalk handles confirm at a station door, a waiting train or a seat
// on the train; it reports whether it did.
func (e *Explore) trainTalk(ctx *Context, p world.Pos) bool {
	if e.infra == nil {
		return false
	}
	if c, ok := e.trainAt(p); ok {
		if !c.st.Moving {
			e.board(c.line, c.st.ToB)
		}
		return true
	}
	id, _ := e.w.Bump(p)
	if id == "station" {
		if s, ok := e.stationAt(p); ok {
			e.timetable(ctx, s, transit.Train, e.infra.Edges(infra.Rail, s), e.infra.Name(s)+" Station")
			return true
		}
	}
	if id == "bus_stop" {
		if s, edges, ok := e.busStopAt(p); ok {
			e.timetable(ctx, s, transit.Bus, edges, e.infra.Name(s)+" bus stop")
			return true
		}
	}
	if p.Map.IsMain() {
		if s, ok := e.harbourAt(p); ok {
			e.timetable(ctx, s, transit.Ferry, e.infra.Edges(infra.SeaLane, s), e.infra.Name(s)+" Harbour")
			return true
		}
	}
	if sg := e.signs[p]; (sg == "train_seat" || sg == "ferry_bench") && e.ride != nil && !e.ride.arrived {
		e.restOffer(ctx)
		return true
	}
	return false
}

// busStopAt returns the town whose bus stop post is tile p, and the roads
// whose buses stop there.
func (e *Explore) busStopAt(p world.Pos) (infra.Settlement, []infra.Edge, bool) {
	for _, st := range e.infra.BusStopsIn(p.X, p.Y, 1, 1) {
		var edges []infra.Edge
		for _, edge := range e.infra.Edges(infra.Road, st.S) {
			if r := e.infra.BusRoute(edge); r != nil && (r.A.Pole == st.Pole || r.B.Pole == st.Pole) {
				edges = append(edges, edge)
			}
		}
		return st.S, edges, true
	}
	return infra.Settlement{}, nil, false
}

// timetable shows the vehicles of a kind leaving s on edges (from a
// station, a harbour or a bus stop), to board or wait for.
func (e *Explore) timetable(ctx *Context, s infra.Settlement, kind transit.Kind, edges []infra.Edge, speaker string) {
	now := e.clock.Ticks()
	var choices []dialog.Choice
	var lines []*transit.Line
	for _, edge := range edges {
		line := e.line(edge)
		if line == nil {
			continue
		}
		atA := edge.A.ID == s.ID
		dest := edge.B
		if !atA {
			dest = edge.A
		}
		k := len(lines)
		lines = append(lines, line)
		st := line.At(float64(now))
		if !st.Moving && st.AtA == atA {
			choices = append(choices, dialog.Choice{Text: "To " + e.infra.Name(dest) + ": boarding now", Do: fmt.Sprintf("board %d", k)})
		} else {
			arrive, _ := line.NextStop(now, atA)
			choices = append(choices, dialog.Choice{Text: "To " + e.infra.Name(dest) + ": wait for the " + clockTime(arrive),
				Do: fmt.Sprintf("wait %d", k)})
		}
	}
	text := "Where would you like to go?"
	if len(lines) == 0 {
		text = vehicles[kind].none
	}
	choices = append(choices, dialog.Choice{Text: "Not now."})
	d := &dialog.Dialog{ID: "timetable", Start: "main", Nodes: map[string]*dialog.Node{
		"main": {Speaker: speaker, Lines: []string{text}, Choices: choices},
	}}
	r := dialog.NewRunner(d)
	ctx.Push(NewDialogBox(r, func() {
		for _, a := range r.Actions() {
			var k int
			var verb string
			if _, err := fmt.Sscanf(a, "%s %d", &verb, &k); err != nil || k >= len(lines) {
				continue
			}
			line := lines[k]
			atA := line.Edge.A.ID == s.ID
			if verb == "wait" {
				arrive, _ := line.NextStop(e.clock.Ticks(), atA)
				e.clock.AdvanceTicks(arrive + 1 - e.clock.Ticks())
				e.refreshReach()
			}
			e.board(line, atA)
		}
	}))
}

// clockTime formats clock ticks as the time of day.
func clockTime(ticks int64) string {
	m := geom.Mod(ticks/60, 24*60)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
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
		e.leaveTrain(true)
	}
}

// leaveTrain gets off at the destination (or, before it leaves, back at
// the start); otherwise the doors stay closed. forced: the train is going
// back, everyone off.
func (e *Explore) leaveTrain(forced bool) {
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
	// Onto the platform beside the train (or, failing that, out in front
	// of the station).
	to, ok := e.platformBeside(r.line, st)
	if forced || !ok {
		front, ok := e.infra.StationFront(at)
		switch r.line.Kind {
		case transit.Ferry:
			var pier infra.Pier
			if pier, ok = e.infra.Harbour(at); ok { // the end of the pier
				front = pier.Cells[len(pier.Cells)-1]
			}
		case transit.Bus:
			br := e.infra.BusRoute(r.line.Edge)
			if ok = br != nil; ok {
				front = br.A.Stop // on the road at the stop
				if br.B.S.ID == at.ID {
					front = br.B.Stop
				}
			}
		}
		if !ok {
			return
		}
		to = world.Pos{Map: world.Main, X: front.X, Y: front.Y}
	}
	e.ride = nil
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

// trainSprite returns the tile of a car (or half of a ferry).
func (e *Explore) trainSprite(l *transit.Line, c transit.Car) (img *ebiten.Image, flip bool) {
	name := "train car h"
	part := "stern"
	if c.Loco {
		part = "bow"
	}
	if l.Kind == transit.Bus {
		part = "back"
		if c.Loco {
			part = "front"
		}
	}
	switch {
	case l.Kind == transit.Bus && c.Facing == world.Right:
		name = "bus " + part + " e"
	case l.Kind == transit.Bus && c.Facing == world.Left:
		name, flip = "bus "+part+" e", true
	case l.Kind == transit.Bus && c.Facing == world.Down:
		name = "bus " + part + " s"
	case l.Kind == transit.Bus:
		name = "bus " + part + " n"
	case l.Kind == transit.Ferry && c.Facing == world.Right:
		name = "ferry " + part + " e"
	case l.Kind == transit.Ferry && c.Facing == world.Left:
		name, flip = "ferry "+part+" e", true
	case l.Kind == transit.Ferry && c.Facing == world.Down:
		name = "ferry " + part + " s"
	case l.Kind == transit.Ferry:
		name = "ferry " + part + " n"
	case c.Loco && c.Facing == world.Right:
		name = "train loco e"
	case c.Loco && c.Facing == world.Left:
		name, flip = "train loco e", true
	case c.Loco && c.Facing == world.Down:
		name = "train loco s"
	case c.Loco:
		name = "train loco n"
	case c.Facing == world.Up || c.Facing == world.Down:
		name = "train car v"
	}
	t, ok := e.trainTiles[name]
	if !ok {
		tt, err := e.w.Tilesets().Named("worlds", name)
		if err != nil {
			return nil, false
		}
		t = tt.Index()
		e.trainTiles[name] = t
	}
	return e.sheets.Tile(sheetWorlds, t), flip
}

// trainDrawables are the cars to draw, relative to the camera at alpha.
func (e *Explore) trainDrawables(m world.MapRef, alpha float64) []drawable {
	if e.infra == nil || !m.IsMain() {
		return nil
	}
	cx, cy := e.cameraAt(alpha)
	t := e.trainTick(alpha)
	var out []drawable
	for _, line := range e.nearLines {
		for _, c := range line.CarsAt(line.At(t)) {
			x, y := e.screenAt(m, c.X*tile, c.Y*tile, cx, cy)
			if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
				continue
			}
			img, flip := e.trainSprite(line, c)
			out = append(out, drawable{img: img, flip: flip, x: x, y: y})
		}
	}
	return out
}

// viewTrack is where along the track the window looks out (track distance
// from A's buffer stop) and which way the train heads (+1 for B, -1 for A):
// the train's real place near stations; at speed, the real place would be
// a blur, so the track passes at a calm pace instead, cutting back to the
// real place as the train slows into the station.
func (e *Explore) viewTrack(alpha float64) (s, heading float64, ok bool) {
	r := e.ride
	if r == nil {
		return 0, 0, false
	}
	t := e.trainTick(alpha)
	st := r.line.At(t)
	s, heading = st.Front, 1 // the ride's direction, not the timetable's next one
	if !r.toB {
		heading = -1
	}
	if !st.Moving && st.AtA != r.toB {
		// Standing at the destination: where it stopped, not where the
		// locomotive will be for the way back.
		s = r.line.Length
		if !r.toB {
			s = 0
		}
	}
	if st.Moving && st.ToB == r.toB {
		start := float64(r.line.Cars - 1) // where the locomotive leaves from
		if !st.ToB {
			start = r.line.Length - float64(r.line.Cars-1)
		}
		run := math.Abs(s - start)
		left := r.line.Length - float64(r.line.Cars-1) - run
		if run > viewSlowTiles && left > viewSlowTiles {
			since := t - float64(st.Since) - r.line.TimeTo(viewSlowTiles)
			s = start + heading*(viewSlowTiles+viewSpeed*max(since, 0))
			s = min(max(s, 0), r.line.Length)
		}
	}
	return s, heading, true
}

// viewPos is the world position the window looks out at (for streaming).
func (e *Explore) viewPos(alpha float64) (x, y float64, ok bool) {
	s, _, ok := e.viewTrack(alpha)
	if !ok {
		return 0, 0, false
	}
	x, y, _ = e.ride.line.Pos(s)
	return x, y, true
}

// viewSide is how many tiles beside the track the windows look.
const viewSide = 2

// drawWindowView draws the world outside into the carriage's windows,
// under the window tiles of dst (whose glass is transparent). The view is
// the land beside the track, unrolled: whichever way the track runs in the
// world, the carriage moves to the right through it (the scenery slides
// left), ahead to the right, on the train's left-hand side.
func (e *Explore) drawWindowView(dst *ebiten.Image, alpha float64) {
	s, heading, ok := e.viewTrack(alpha)
	if !ok {
		return
	}
	win, err := e.w.Tilesets().Named("worlds", "train window")
	if err != nil {
		return
	}
	line := e.ride.line
	vm := vehicleMap(e.ride.line)
	mw, mh, _ := e.w.Size(world.Detail(vm))
	e.viewImg = ensureImage(e.viewImg, tile, tile)
	u := heading * s // grows as the train goes, either way
	for cy := int64(0); cy < mh; cy++ {
		for cx := int64(0); cx < mw; cx++ {
			p := world.Pos{Map: world.Detail(vm), X: cx, Y: cy}
			if e.w.Tile(p, world.LayerGround) != win {
				continue
			}
			// The windows side by side show the strip from left to right;
			// those along the top row look out to the left, others (a
			// windscreen) by their row.
			side := int64(viewSide)
			if cy > 0 {
				side = mh/2 - cy
			}
			v := u + float64(cx-mw/2)
			t0 := math.Floor(v)
			shift := int((v - t0) * tile)
			e.viewImg.Clear()
			for k := 0; k < 2; k++ {
				sd := heading * (t0 + float64(k)) // back to track distance
				x, y, d := line.Pos(sd)
				if heading < 0 {
					d = opposite(d)
				}
				nx, ny := leftOf(d)
				wp, ok := e.w.Normalize(world.Pos{Map: world.Main,
					X: int64(math.Round(x)) + nx*side, Y: int64(math.Round(y)) + ny*side})
				if ok {
					e.drawWorldTile(e.viewImg, wp, k*tile-shift, 0)
				}
			}
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(cx*tile-e.camX), float64(cy*tile-e.camY))
			dst.DrawImage(e.viewImg, op)
		}
	}
}

// drawSeaView draws the world round the ferry's deck, over the whole of
// dst: the strip of world along the route, unrolled like the train's
// window view. The ferry sails to the right through it; each row above the
// deck's middle row is a tile further to the left of the route (port),
// each row below one to the right.
func (e *Explore) drawSeaView(dst *ebiten.Image, alpha float64) {
	s, heading, ok := e.viewTrack(alpha)
	if !ok {
		return
	}
	line := e.ride.line
	mw, mh, _ := e.w.Size(world.Detail(ferryMap))
	u := heading * s
	t0 := math.Floor(u)
	shift := int((u - t0) * tile)
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx0, cy0 := geom.FloorDiv(e.camX, tile), geom.FloorDiv(e.camY, tile)
	for cy := cy0; cy*tile-e.camY < int64(h); cy++ {
		lat := mh/2 - cy
		for cx := cx0; cx*tile-e.camX-int64(shift) < int64(w); cx++ {
			sd := heading * (t0 + float64(cx-mw/2)) // back to route distance
			sc := min(max(sd, 0), line.Length)
			x, y, d := line.Pos(sc)
			// Beyond the ends (at the piers), straight on.
			ax, ay := dirVec(d)
			x, y = x+ax*(sd-sc), y+ay*(sd-sc)
			if heading < 0 {
				d = opposite(d)
			}
			nx, ny := leftOf(d)
			wp, ok := e.w.Normalize(world.Pos{Map: world.Main,
				X: int64(math.Round(x)) + nx*lat, Y: int64(math.Round(y)) + ny*lat})
			if ok {
				e.drawWorldTile(dst, wp, int(cx*tile-e.camX)-shift, int(cy*tile-e.camY))
			}
		}
	}
}

// dirVec returns the unit step of a direction.
func dirVec(d world.Dir) (dx, dy float64) {
	switch d {
	case world.Right:
		return 1, 0
	case world.Left:
		return -1, 0
	case world.Down:
		return 0, 1
	}
	return 0, -1
}

// leftOf returns the offset to the left of a direction of travel.
func leftOf(d world.Dir) (dx, dy int64) {
	switch d {
	case world.Right:
		return 0, -1
	case world.Left:
		return 0, 1
	case world.Down:
		return 1, 0
	}
	return -1, 0
}

func opposite(d world.Dir) world.Dir {
	switch d {
	case world.Up:
		return world.Down
	case world.Down:
		return world.Up
	case world.Left:
		return world.Right
	}
	return world.Left
}

// drawWorldTile draws all layers of main-map tile p at (x, y) (pixels) in
// dst.
func (e *Explore) drawWorldTile(dst *ebiten.Image, p world.Pos, x, y int) {
	sets := e.w.Tilesets()
	ms := animationClock()
	for l := 0; l < world.NumLayers; l++ {
		t := sets.Frame(e.w.Tile(p, l), ms, p.X, p.Y)
		if t.Empty() {
			continue
		}
		e.drawTile(dst, e.sheets.Tile(sets.Get(t.Set()).Name, t.Index()), x, y, t.FlipH(), t.FlipV())
	}
}
