package scene

import (
	"fmt"
	"image"
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/entity"
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

	// Click/tap-to-move state.
	path     []nav.Move
	goal     world.Pos
	hasGoal  bool
	interact *world.Pos // interact with this tile once the goal is reached
	waiting  int

	turnWait int
	fade     int // >0 fading out, <0 fading in
	warp     *world.Warp

	camX, camY int
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
	}
	ids := make([]string, 0, len(e.w.Maps))
	for id := range e.w.Maps {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := e.w.Maps[world.MapID(id)]
		for _, o := range m.Objects {
			p := world.Pos{Map: m.ID, X: o.X, Y: o.Y}
			switch o.Type {
			case "npc":
				if o.Tile.Empty() {
					return nil, fmt.Errorf("map %s: npc %q must be a tile object", id, o.Name)
				}
				facing, _ := world.ParseDir(o.Props.String("facing"))
				n := &NPC{Dialog: o.Props.String("dialog")}
				n.Name = o.Name
				n.Sprite = entity.Sprite{Sheet: o.Tile.Set.Name, Index: o.Tile.Index}
				n.Place(p, facing)
				e.npcs = append(e.npcs, n)
			case "sign":
				e.signs[p] = o.Props.String("dialog")
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
	e.updateCamera(ctx)
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
	leader := e.party.Leader()
	r, _, _ := e.w.ToRegion(leader.Pos)
	if target, ok := r.At(floorDiv(p.X+e.camX, tile), floorDiv(p.Y+e.camY, tile)); ok {
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
	ctx.Push(NewDialogBox(dialog.NewRunner(d), nil))
}

// regionPixel returns the top-left pixel of an actor in region coordinates.
func (e *Explore) regionPixel(a *entity.Actor) (x, y int) {
	_, rx, ry := e.w.ToRegion(a.Pos)
	dx, dy := a.Offset()
	return rx*tile + dx, ry*tile + dy + a.Hop()
}

func (e *Explore) updateCamera(ctx *Context) {
	r, _, _ := e.w.ToRegion(e.party.Leader().Pos)
	x, y := e.regionPixel(e.party.Leader())
	e.camX = clampCam(x+tile/2-ctx.W/2, r.MinX*tile, r.W*tile, ctx.W)
	e.camY = clampCam(y+tile/2-ctx.H/2, r.MinY*tile, r.H*tile, ctx.H)
}

// clampCam keeps the view inside the region, or centres the region if it is
// smaller than the screen.
func clampCam(c, lo, size, view int) int {
	if size <= view {
		return lo - (view-size)/2
	}
	return max(lo, min(c, lo+size-view))
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Draw implements Scene.
func (e *Explore) Draw(dst *ebiten.Image) {
	dst.Fill(gfx.ColorVoid)
	r, _, _ := e.w.ToRegion(e.party.Leader().Pos)
	e.drawLayers(dst, r, false)
	e.drawActors(dst, r)
	e.drawLayers(dst, r, true)
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

func (e *Explore) drawLayers(dst *ebiten.Image, r *world.Region, overhead bool) {
	sw, sh := dst.Bounds().Dx(), dst.Bounds().Dy()
	for _, m := range r.Maps {
		ox, oy := m.OriginX*tile-e.camX, m.OriginY*tile-e.camY
		x0, y0 := max(0, floorDiv(-ox, tile)), max(0, floorDiv(-oy, tile))
		x1, y1 := min(m.W, floorDiv(sw-ox, tile)+1), min(m.H, floorDiv(sh-oy, tile)+1)
		for _, l := range m.Layers {
			if l.Overhead != overhead {
				continue
			}
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					t := l.Tiles[y*m.W+x]
					if t.Empty() {
						continue
					}
					e.drawTile(dst, e.sheets.Tile(t.Set.Name, t.Index), ox+x*tile, oy+y*tile, t.FlipH, t.FlipV)
				}
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
	x, y     int
	priority int
}

func (e *Explore) drawActors(dst *ebiten.Image, r *world.Region) {
	var ds []drawable
	for _, n := range e.npcs {
		if e.w.Map(n.Pos.Map).Region != r {
			continue
		}
		x, y := e.regionPixel(&n.Actor)
		ds = append(ds, drawable{&n.Actor, x, y, 0})
	}
	for i, m := range e.party.Members {
		x, y := e.regionPixel(m)
		// Leader on top when the party is stacked after a warp.
		ds = append(ds, drawable{m, x, y, len(e.party.Members) - i})
	}
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].y != ds[j].y {
			return ds[i].y < ds[j].y
		}
		return ds[i].priority < ds[j].priority
	})
	for _, d := range ds {
		img := e.sheets.Tile(d.a.Sprite.Sheet, d.a.Sprite.Index)
		e.drawTile(dst, img, d.x-e.camX, d.y-e.camY, d.a.Facing == world.Left, false)
	}
}

func (e *Explore) drawDebug(dst *ebiten.Image) {
	for _, m := range e.path {
		r, rx, ry := e.w.ToRegion(m.To)
		if r == nil {
			continue
		}
		x, y := rx*tile-e.camX, ry*tile-e.camY
		gfx.FillRect(dst, image.Rect(x+6, y+6, x+10, y+10), gfx.ColorAccent)
	}
	l := e.party.Leader()
	r, rx, ry := e.w.ToRegion(l.Pos)
	s := fmt.Sprintf("%s  region %s (%d,%d)  %.0f fps", l.Pos, r.ID, rx, ry, ebiten.ActualFPS())
	gfx.FillRect(dst, image.Rect(0, 0, gfx.TextWidth(s)+4, gfx.LineHeight+2), color.RGBA{A: 0xa0})
	gfx.Text(dst, s, 2, 0, color.White)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
