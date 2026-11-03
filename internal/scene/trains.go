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

const (
	trainMap      world.MapID = "train" // the carriage's detail map
	trainReach                = 48      // lines this close to the party are drawn
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

// line returns the line of a rail edge, made on first use.
func (e *Explore) line(edge infra.Edge) *transit.Line {
	if l, ok := e.lines[edge]; ok {
		return l
	}
	l := transit.NewLine(e.infra, edge, e.w.W, e.w.H, e.w.Seed)
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
			if w.Net != infra.Rail {
				continue
			}
			if line := e.line(w.Edge); line != nil && !seen[line] {
				seen[line] = true
				e.nearLines = append(e.nearLines, line)
			}
		}
	}
	clear(e.trainCells)
	if l.Pos.Map.IsMain() {
		for _, line := range e.nearLines {
			st := line.At(float64(e.clock.Ticks()))
			for _, c := range line.Cars(st) {
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

// stationAt returns the city whose station door is at p.
func (e *Explore) stationAt(p world.Pos) (infra.Settlement, bool) {
	for _, s := range e.infra.Near(p.X, p.Y, 200, infra.City) {
		if front, ok := e.infra.StationFront(s); ok {
			if dx, dy, _ := e.w.Delta(p, world.Pos{Map: world.Main, X: front.X, Y: front.Y}); geom.Abs(dx) <= 1 && dy == 1 {
				return s, true
			}
		}
	}
	return infra.Settlement{}, false
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
			e.say(e.infra.Name(s) + " Station. Ask at the door for the timetable.")
			return true
		}
	case ok && id == "train_exit":
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
	if id, ok := e.w.Bump(p); ok && id == "station" {
		if s, ok := e.stationAt(p); ok {
			e.timetable(ctx, s)
			return true
		}
	}
	if e.signs[p] == "train_seat" && e.ride != nil && !e.ride.arrived {
		e.restOffer(ctx)
		return true
	}
	return false
}

// timetable shows the trains from a station, to board or wait for.
func (e *Explore) timetable(ctx *Context, s infra.Settlement) {
	now := e.clock.Ticks()
	var choices []dialog.Choice
	var lines []*transit.Line
	for _, edge := range e.infra.Edges(infra.Rail, s) {
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
		text = "No trains run from here."
	}
	choices = append(choices, dialog.Choice{Text: "Not now."})
	d := &dialog.Dialog{ID: "timetable", Start: "main", Nodes: map[string]*dialog.Node{
		"main": {Speaker: e.infra.Name(s) + " Station", Lines: []string{text}, Choices: choices},
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
	sp, ok := e.w.Spawn(trainMap, "aboard")
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
	if e.party.Leader().Pos.Map.Template != trainMap {
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
		e.bump0("train_exit")
		return
	}
	front, ok := e.infra.StationFront(at)
	if !ok {
		return
	}
	e.ride = nil
	e.warp = &world.Warp{To: world.Pos{Map: world.Main, X: front.X, Y: front.Y}, Facing: world.Down, HasFacing: true}
	e.fade = fadeTicks
	if forced {
		e.pendingNotice = "End of the line. Everyone off, please."
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

// trainSprite returns the tile of a car.
func (e *Explore) trainSprite(c transit.Car) (img *ebiten.Image, flip bool) {
	name := "train car h"
	switch {
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
		for _, c := range line.Cars(line.At(t)) {
			x, y := e.screenAt(m, c.X*tile, c.Y*tile, cx, cy)
			if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
				continue
			}
			img, flip := e.trainSprite(c)
			out = append(out, drawable{img: img, flip: flip, x: x, y: y})
		}
	}
	return out
}

// viewPos is where the window looks out: the train's real place near
// stations; at speed, the track passing at a calm pace instead (the real
// place would be a blur), cutting back to the real place on arrival.
func (e *Explore) viewPos(alpha float64) (x, y float64, ok bool) {
	r := e.ride
	if r == nil {
		return 0, 0, false
	}
	t := e.trainTick(alpha)
	st := r.line.At(t)
	s := st.Front
	if st.Moving {
		start := float64(transit.Cars - 1) // where the locomotive leaves from
		dir := 1.0
		if !st.ToB {
			start, dir = r.line.Length-float64(transit.Cars-1), -1
		}
		run := math.Abs(s - start)
		left := r.line.Length - float64(transit.Cars-1) - run
		if run > viewSlowTiles && left > viewSlowTiles {
			since := t - float64(st.Since) - r.line.TimeTo(viewSlowTiles)
			s = start + dir*(viewSlowTiles+viewSpeed*max(since, 0))
			s = min(max(s, 0), r.line.Length)
		}
	}
	x, y, _ = r.line.Pos(s)
	return x, y, true
}

// drawWindowView draws the world outside into the carriage's windows:
// under the window tiles of dst (the ground layer at the camera), whose
// glass is transparent.
func (e *Explore) drawWindowView(dst *ebiten.Image, alpha float64) {
	vx, vy, ok := e.viewPos(alpha)
	if !ok {
		return
	}
	win, err := e.w.Tilesets().Named("worlds", "train window")
	if err != nil {
		return
	}
	mw, mh, _ := e.w.Size(world.Detail(trainMap))
	e.viewImg = ensureImage(e.viewImg, tile, tile)
	saveX, saveY := e.camX, e.camY
	for cy := int64(0); cy < mh; cy++ {
		for cx := int64(0); cx < mw; cx++ {
			p := world.Pos{Map: world.Detail(trainMap), X: cx, Y: cy}
			if e.w.Tile(p, world.LayerGround) != win {
				continue
			}
			// The window shows the world a few tiles north of the track,
			// spread out like the windows along the carriage.
			wx := vx*tile + float64(cx-mw/2)*tile
			wy := vy*tile - 3*tile
			e.viewImg.Clear()
			e.camX, e.camY = int64(math.Floor(wx)), int64(math.Floor(wy))
			e.drawLayers(e.viewImg, world.Main, false)
			e.drawLayers(e.viewImg, world.Main, true)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(cx*tile-saveX), float64(cy*tile-saveY))
			dst.DrawImage(e.viewImg, op)
		}
	}
	e.camX, e.camY = saveX, saveY
}
