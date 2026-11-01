package scene

import (
	"fmt"
	"image"
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

const (
	tile = world.TileSize
	// turnDelay is how long a direction must be held after turning on the
	// spot before the leader starts walking. Tapping only turns.
	turnDelay = 6
	fadeTicks = 14
	// blockedWait is how long path following waits for a blocker to move
	// before replanning.
	blockedWait = 20
)

// NPC is a non-player character placed in a map.
type NPC struct {
	entity.Actor
	Dialog string
}

// Explore is the exploration scene: the party walks the world in real time.
// Later, combat runs in this same scene with a turn-based controller instead
// of the real-time one, so fights happen on the map itself.
type Explore struct {
	w       *world.World
	planner *nav.Planner
	sheets  *gfx.Sheets
	dialogs dialog.Library

	party *entity.Party
	npcs  []*NPC
	signs map[world.Pos]string // interactable objects -> dialog id
	bumps map[world.Pos]string // walking into these shows a passive message (dialog id)

	// notice is a passive message (e.g. "The door is locked."). It does not
	// block input and disappears when the leader moves or turns away.
	notice *notice

	// Click/tap-to-move state.
	path     []nav.Move
	goal     world.Pos
	hasGoal  bool
	interact *world.Pos // interact with this tile once the goal is reached
	waiting  int

	turnWait int
	fade     int // >0 fading out, <0 fading in
	warp     *world.Warp

	// Camera: top-left pixel of the view in the coordinates of the leader's
	// map. On the main map it is not wrapped; everything drawn is placed
	// relative to it with wrap-aware deltas.
	camX, camY int64
	debug      bool
}

// ExploreConfig holds everything the exploration scene needs.
type ExploreConfig struct {
	World   *world.World
	Sheets  *gfx.Sheets
	Dialogs dialog.Library
	Party   *entity.Party
	Start   world.Spawn
}

// NewExplore creates the exploration scene and spawns NPCs from all maps.
func NewExplore(cfg ExploreConfig) (*Explore, error) {
	e := &Explore{
		w:       cfg.World,
		planner: nav.NewPlanner(cfg.World),
		sheets:  cfg.Sheets,
		dialogs: cfg.Dialogs,
		party:   cfg.Party,
		signs:   map[world.Pos]string{},
		bumps:   map[world.Pos]string{},
	}
	for _, o := range e.w.Objects() {
		switch o.Type {
		case "npc":
			if o.Tile.Empty() {
				return nil, fmt.Errorf("%v: npc %q must be a tile object", o.Pos, o.Name)
			}
			facing, _ := world.ParseDir(o.Props.String("facing"))
			n := &NPC{Dialog: o.Props.String("dialog")}
			n.Name = o.Name
			n.Sprite = entity.Sprite{Sheet: e.w.Tilesets().Get(o.Tile.Set()).Name, Index: o.Tile.Index()}
			n.Place(o.Pos, facing)
			e.npcs = append(e.npcs, n)
		case "sign":
			e.signs[o.Pos] = o.Props.String("dialog")
		}
		if id := o.Props.String("bump"); id != "" {
			if e.dialogs[id] == nil {
				return nil, fmt.Errorf("%v: object %q: unknown bump dialog %q", o.Pos, o.Name, id)
			}
			for y := int64(0); y < o.H; y++ {
				for x := int64(0); x < o.W; x++ {
					if p, ok := e.w.Offset(o.Pos, x, y); ok {
						e.bumps[p] = id
					}
				}
			}
		}
	}
	facing := world.Down
	if cfg.Start.HasFacing {
		facing = cfg.Start.Facing
	}
	e.party.PlaceAll(cfg.Start.Pos, facing)
	return e, nil
}

func (e *Explore) npcAt(p world.Pos) *NPC {
	for _, n := range e.npcs {
		if n.Pos == p {
			return n
		}
	}
	return nil
}

func (e *Explore) blocked(p world.Pos) bool { return e.npcAt(p) != nil }

func (e *Explore) walkable(p world.Pos) bool { return !e.w.Solid(p) && !e.blocked(p) }

// Update implements Scene.
func (e *Explore) Update(ctx *Context) {
	in := ctx.Input
	if in.Pressed(input.Debug) {
		e.debug = !e.debug
	}
	if e.fade != 0 {
		e.updateFade()
		e.updateCamera(ctx)
		return
	}

	// Taps are accepted while walking; the new goal is picked up as soon as
	// the leader reaches the next tile.
	if p, ok := in.Tap(); ok {
		e.tapAt(p)
	}

	arrived := e.party.Update()
	leader := e.party.Leader()
	if arrived {
		if wp := e.w.WarpAt(leader.Pos); wp != nil {
			e.warp = wp
			e.fade = fadeTicks
			return
		}
	}
	if !leader.Moving() {
		e.control(ctx, arrived)
	}
	if n := e.notice; n != nil && (leader.Moving() || leader.Pos != n.at || leader.Facing != n.facing) {
		e.notice = nil
	}
	e.updateCamera(ctx)
}

// notice is a passive message shown while the leader stays put.
type notice struct {
	speaker, text string
	at            world.Pos
	facing        world.Dir
}

// bump shows the bump message of the object at p, if it has one.
func (e *Explore) bump(p world.Pos) {
	id, ok := e.bumps[p]
	if !ok {
		return
	}
	l := e.party.Leader()
	if e.notice != nil && e.notice.at == l.Pos && e.notice.facing == l.Facing {
		return // already showing
	}
	start := e.dialogs[id].Nodes[e.dialogs[id].Start]
	e.notice = &notice{speaker: start.Speaker, text: start.Lines[0], at: l.Pos, facing: l.Facing}
}

func (e *Explore) updateFade() {
	if e.fade > 0 {
		e.fade--
		if e.fade == 0 {
			// Fully black: move the party, then fade back in.
			facing := e.party.Leader().Facing
			if e.warp.HasFacing {
				facing = e.warp.Facing
			}
			e.party.PlaceAll(e.warp.To, facing)
			e.warp = nil
			e.path = nil // the next leg is planned from the new position
			e.fade = -fadeTicks
		}
		return
	}
	e.fade++
}

// control handles input and path following while the leader stands still.
func (e *Explore) control(ctx *Context, justArrived bool) {
	in := ctx.Input
	leader := e.party.Leader()

	if in.Pressed(input.Confirm) {
		e.cancelPath()
		if front, ok := e.w.Neighbor(leader.Pos, leader.Facing); ok {
			e.talk(ctx, front)
		}
		return
	}
	if d, ok := in.Dir(); ok {
		e.cancelPath()
		switch {
		case d != leader.Facing && !justArrived && in.DirPressed():
			leader.Facing = d
			e.turnWait = turnDelay
		case e.turnWait > 0:
			e.turnWait--
		default:
			e.tryStep(d)
		}
		return
	}
	e.turnWait = 0
	e.followPath(ctx)
}

func (e *Explore) tryStep(d world.Dir) bool {
	leader := e.party.Leader()
	leader.Facing = d
	to, ok := e.w.Neighbor(leader.Pos, d)
	if !ok || !e.walkable(to) {
		if ok {
			e.bump(to)
		}
		return false
	}
	e.party.Step(e.w, d, to)
	return true
}

func (e *Explore) cancelPath() {
	e.path, e.hasGoal, e.interact = nil, false, nil
}

// tapAt converts a tap to a tile on the current region and walks there.
func (e *Explore) tapAt(p image.Point) {
	m := e.party.Leader().Pos.Map
	tx, ty := geom.FloorDiv(e.camX+int64(p.X), tile), geom.FloorDiv(e.camY+int64(p.Y), tile)
	if target, ok := e.w.Normalize(world.Pos{Map: m, X: tx, Y: ty}); ok {
		e.goTo(target)
	}
}

// goTo walks the party to target, which may be anywhere in the world: the
// planner routes across map edges and through warps. If target holds an NPC
// or a sign, the party walks next to it and talks.
func (e *Explore) goTo(target world.Pos) {
	leader := e.party.Leader()
	e.cancelPath()
	_, isSign := e.signs[target]
	if e.npcAt(target) != nil || isSign {
		// Walk to the closest free neighbour, then interact.
		best, bestLen := world.Pos{}, -1
		for _, d := range world.Dirs {
			n, ok := e.w.Neighbor(target, d)
			if !ok || !e.walkable(n) || e.w.WarpAt(n) != nil {
				continue
			}
			if n == leader.Pos {
				best, bestLen = n, 0
				break
			}
			if moves, ok := e.planner.Plan(leader.Pos, n, e.blocked); ok && (bestLen < 0 || len(moves) < bestLen) {
				best, bestLen = n, len(moves)
			}
		}
		if bestLen < 0 {
			return
		}
		e.goal, e.hasGoal, e.interact = best, true, &target
		return
	}
	if !e.walkable(target) {
		return
	}
	e.goal, e.hasGoal = target, true
}

func (e *Explore) followPath(ctx *Context) {
	if !e.hasGoal {
		return
	}
	leader := e.party.Leader()
	if leader.Pos == e.goal {
		if e.interact != nil {
			target := *e.interact
			if d, ok := e.w.DirBetween(leader.Pos, target); ok {
				leader.Facing = d
			}
			e.cancelPath()
			e.talk(ctx, target)
			return
		}
		e.cancelPath()
		return
	}
	if len(e.path) == 0 {
		moves, ok := e.planner.Plan(leader.Pos, e.goal, e.blocked)
		if !ok || len(moves) == 0 {
			e.cancelPath()
			return
		}
		e.path = moves
	}
	m := e.path[0]
	if next, ok := e.w.Neighbor(leader.Pos, m.Dir); !ok || next != m.To {
		e.path = nil // out of sync, replan next tick
		return
	}
	if !e.walkable(m.To) {
		leader.Facing = m.Dir
		e.waiting++
		if e.waiting >= blockedWait {
			e.waiting = 0
			e.path = nil
		}
		return
	}
	e.waiting = 0
	e.path = e.path[1:]
	e.party.Step(e.w, m.Dir, m.To)
}

// talk starts the conversation with whoever or whatever is at p.
func (e *Explore) talk(ctx *Context, p world.Pos) {
	id := ""
	if n := e.npcAt(p); n != nil {
		id = n.Dialog
		if d, ok := e.w.DirBetween(p, e.party.Leader().Pos); ok {
			n.Facing = d
		}
	} else if s, ok := e.signs[p]; ok {
		id = s
	}
	d := e.dialogs[id]
	if d == nil {
		return
	}
	e.notice = nil
	ctx.Push(NewDialogBox(dialog.NewRunner(d), nil))
}

// pixel returns the top-left pixel of an actor in its map's coordinates
// (unwrapped), following its step but without the walk hop.
func (e *Explore) pixel(a *entity.Actor) (x, y int64) {
	dx, dy := a.Offset()
	return a.Pos.X*tile + int64(dx), a.Pos.Y*tile + int64(dy)
}

// screen converts a pixel position on the leader's map to screen
// coordinates. On the main map the shortest way around the world is used, so
// things across the seam appear next to each other.
func (e *Explore) screen(m world.MapRef, px, py int64) (int, int) {
	w, h, wraps := e.w.Size(m)
	if wraps {
		return int(geom.WrapDelta(e.camX, px, w*tile)), int(geom.WrapDelta(e.camY, py, h*tile))
	}
	return int(px - e.camX), int(py - e.camY)
}

func (e *Explore) updateCamera(ctx *Context) {
	l := e.party.Leader()
	x, y := e.pixel(l)
	e.camX = x + tile/2 - int64(ctx.W)/2
	e.camY = y + tile/2 - int64(ctx.H)/2
	if w, h, wraps := e.w.Size(l.Pos.Map); !wraps {
		e.camX = clampCam(e.camX, w*tile, int64(ctx.W))
		e.camY = clampCam(e.camY, h*tile, int64(ctx.H))
	}
}

// clampCam keeps the view inside a detail map, or centres the map if it is
// smaller than the screen.
func clampCam(c, size, view int64) int64 {
	if size <= view {
		return -(view - size) / 2
	}
	return max(0, min(c, size-view))
}

// Draw implements Scene.
func (e *Explore) Draw(dst *ebiten.Image) {
	dst.Fill(gfx.ColorVoid)
	m := e.party.Leader().Pos.Map
	e.drawLayers(dst, m, false)
	e.drawActors(dst, m)
	e.drawLayers(dst, m, true)
	if e.notice != nil {
		e.drawNotice(dst)
	}
	if e.debug {
		e.drawDebug(dst)
	}
	if e.fade != 0 {
		a := float64(fadeTicks-abs(e.fade)) / fadeTicks
		if e.fade < 0 {
			a = float64(-e.fade) / fadeTicks
		}
		gfx.FillRect(dst, dst.Bounds(), color.RGBA{A: uint8(a * 255)})
	}
}

// drawLayers draws the visible tiles. Tile coordinates are iterated in
// camera space and normalised per tile, so the seam needs no special case.
func (e *Explore) drawLayers(dst *ebiten.Image, m world.MapRef, overhead bool) {
	sw, sh := int64(dst.Bounds().Dx()), int64(dst.Bounds().Dy())
	x0, y0 := geom.FloorDiv(e.camX, tile), geom.FloorDiv(e.camY, tile)
	x1, y1 := geom.FloorDiv(e.camX+sw-1, tile), geom.FloorDiv(e.camY+sh-1, tile)
	sets := e.w.Tilesets()
	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			p, ok := e.w.Normalize(world.Pos{Map: m, X: tx, Y: ty})
			if !ok {
				continue
			}
			sx, sy := int(tx*tile-e.camX), int(ty*tile-e.camY)
			for l := 0; l < world.NumLayers; l++ {
				if (l == world.LayerOverhead) != overhead {
					continue
				}
				t := e.w.Tile(p, l)
				if t.Empty() {
					continue
				}
				e.drawTile(dst, e.sheets.Tile(sets.Get(t.Set()).Name, t.Index()), sx, sy, t.FlipH(), t.FlipV())
			}
		}
	}
}

func (e *Explore) drawTile(dst, img *ebiten.Image, x, y int, flipH, flipV bool) {
	if img == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	if flipH {
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(tile, 0)
	}
	if flipV {
		op.GeoM.Scale(1, -1)
		op.GeoM.Translate(0, tile)
	}
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(img, op)
}

type drawable struct {
	a        *entity.Actor
	x, y     int // screen position
	priority int
}

func (e *Explore) drawActors(dst *ebiten.Image, m world.MapRef) {
	sw, sh := dst.Bounds().Dx(), dst.Bounds().Dy()
	var ds []drawable
	add := func(a *entity.Actor, priority int) {
		if a.Pos.Map != m {
			return
		}
		px, py := e.pixel(a)
		x, y := e.screen(m, px, py+int64(a.Hop()))
		if x < -tile || y < -tile || x > sw || y > sh {
			return
		}
		ds = append(ds, drawable{a, x, y, priority})
	}
	for _, n := range e.npcs {
		add(&n.Actor, 0)
	}
	for i, a := range e.party.Members {
		add(a, len(e.party.Members)-i) // leader on top when stacked after a warp
	}
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].y != ds[j].y {
			return ds[i].y < ds[j].y
		}
		return ds[i].priority < ds[j].priority
	})
	for _, d := range ds {
		img := e.sheets.Tile(d.a.Sprite.Sheet, d.a.Sprite.Index)
		e.drawTile(dst, img, d.x, d.y, d.a.Facing == world.Left, false)
	}
}

// drawNotice draws the passive message in the dialog box position, sized to
// its text. It has no "more" marker because it needs no confirmation.
func (e *Explore) drawNotice(dst *ebiten.Image) {
	sw, sh := dst.Bounds().Dx(), dst.Bounds().Dy()
	rows := gfx.Wrap(e.notice.text, sw-2*boxMargin-2*boxPad)
	h := len(rows)*gfx.LineHeight + 2*boxPad
	box := image.Rect(boxMargin, sh-boxMargin-h, sw-boxMargin, sh-boxMargin)
	gfx.Box(dst, box)
	if sp := e.notice.speaker; sp != "" {
		tw := gfx.TextWidth(sp) + 2*boxPad
		tag := image.Rect(box.Min.X+4, box.Min.Y-gfx.LineHeight-4, box.Min.X+4+tw, box.Min.Y+1)
		gfx.Box(dst, tag)
		gfx.Text(dst, sp, tag.Min.X+boxPad, tag.Min.Y+1, gfx.ColorAccent)
	}
	for i, row := range rows {
		gfx.Text(dst, row, box.Min.X+boxPad, box.Min.Y+boxPad-1+i*gfx.LineHeight, gfx.ColorInk)
	}
}

func (e *Explore) drawDebug(dst *ebiten.Image) {
	for _, mv := range e.path {
		x, y := e.screen(mv.To.Map, mv.To.X*tile, mv.To.Y*tile)
		gfx.FillRect(dst, image.Rect(x+6, y+6, x+10, y+10), gfx.ColorAccent)
	}
	l := e.party.Leader()
	s := fmt.Sprintf("%s chunk(%d,%d) cached %d  %.0f fps", l.Pos,
		geom.FloorDiv(l.Pos.X, world.ChunkSize), geom.FloorDiv(l.Pos.Y, world.ChunkSize),
		e.w.CachedChunks(), ebiten.ActualFPS())
	gfx.FillRect(dst, image.Rect(0, 0, gfx.TextWidth(s)+4, gfx.LineHeight+2), color.RGBA{A: 0xa0})
	gfx.Text(dst, s, 2, 0, color.White)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
