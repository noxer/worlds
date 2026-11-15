package scene

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// A fight happens where the party stands, on the map it is exploring: the
// Battle scene goes on top of the exploration scene (which stands still
// and keeps drawing the world), takes the party's members and the enemies
// near by as combatants (package combat holds the rules), and plays turns
// until one side is down.
//
// Enemies are NPCs with an "enemy" property (their kind in combat.yaml).
// They are in plain sight on the map; a fight starts when one sees the
// party (sightTiles, with a line of sight) or the party walks into one.
//
// On the party's turns: arrows move a cursor over the tiles, Menu cycles
// the action (move, the member's abilities, end the turn), Confirm does
// it on the cursor's tile; a tap on a tile does the same there, a tap on
// the action bar picks an action. Moving onto an enemy with the cursor
// while "Move" is picked uses the first ability that can reach it.

const (
	sightTiles    = 5  // an enemy this near (in steps) with a line of sight starts a fight
	battleRadius  = 10 // enemies this near the one that started it join the fight
	battleStep    = 8  // ticks per tile walked in a fight
	battlePause   = 24 // ticks after an ability, to read what happened
	popupTicks    = 50
	cursorRepeat  = 8 // ticks between cursor steps while a direction is held
	battleBarH    = 2*gfx.LineHeight + 8
	battleButtonW = 8
)

// lookOut starts a fight if an enemy sees the party.
func (e *Explore) lookOut() {
	if e.combat == nil || e.pendingBattle != nil || e.ride != nil {
		return
	}
	l := e.party.Leader()
	board := battleBoard{e: e}
	for _, n := range e.npcs {
		if !n.active || n.enemy == "" || n.Pos.Map != l.Pos.Map {
			continue
		}
		dx, dy, ok := e.w.Delta(n.Pos, l.Pos)
		if !ok || geom.Abs(dx)+geom.Abs(dy) > sightTiles || !combat.Sight(board, n.Pos, l.Pos) {
			continue
		}
		e.pendingBattle = n
		return
	}
}

// battleBoard is the map a fight stands on, for the rules.
type battleBoard struct {
	e      *Explore
	others map[world.Pos]bool // bystanders' tiles: nobody stands there
}

func (b battleBoard) Offset(p world.Pos, dx, dy int64) (world.Pos, bool) {
	return b.e.w.Offset(p, dx, dy)
}

func (b battleBoard) Delta(p, q world.Pos) (int64, int64) {
	dx, dy, _ := b.e.w.Delta(p, q)
	return dx, dy
}

func (b battleBoard) Walkable(p world.Pos) bool {
	return !b.e.w.Solid(p) && b.e.w.Terrain(p) == world.Land && !b.others[p]
}

func (b battleBoard) Opaque(p world.Pos) bool { return b.e.w.Solid(p) }

// Battle is a fight on the map.
type Battle struct {
	e      *Explore
	f      *combat.Fight
	rules  *combat.Rules
	actors []*entity.Actor // by combatant ID
	foes   []*NPC          // by combatant ID; nil for the party's members

	cursor world.Pos
	action int // index into actions(): what Confirm does
	hold   int // ticks a direction has been held, for the cursor's repeat

	walker *entity.Actor // someone walking a path
	path   []world.Pos
	pause  int // ticks to wait before going on
	popups []battlePopup
	log    string
	done   bool // the outcome has been shown and acknowledged
	w, h   int
}

type battlePopup struct {
	text  string
	at    world.Pos
	ticks int
	col   color.Color
}

// battleAction is an entry of the action bar.
type battleAction struct {
	label   string
	ability combat.AbilityID // "" for move and end
	end     bool
}

// newBattle starts a fight with the enemy n and its neighbours; nil if
// there are no rules or nowhere to stand.
func newBattle(e *Explore, n *NPC) *Battle {
	if e.combat == nil || n.enemy == "" || !n.active {
		return nil
	}
	b := &Battle{e: e, rules: e.combat}
	board := battleBoard{e: e, others: map[world.Pos]bool{}}
	var cs []combat.Combatant
	taken := map[world.Pos]bool{}
	// The enemies: the one that started it and those near it.
	for _, m := range e.npcs {
		if !m.active || m.Pos.Map != n.Pos.Map {
			continue
		}
		dx, dy, _ := e.w.Delta(n.Pos, m.Pos)
		if m.enemy == "" || max(geom.Abs(dx), geom.Abs(dy)) > battleRadius {
			if max(geom.Abs(dx), geom.Abs(dy)) <= 2*battleRadius {
				board.others[m.Pos] = true // a bystander
			}
			continue
		}
		c := e.combat.Combatant(m.enemy, strings.ToUpper(m.enemy[:1])+m.enemy[1:], combat.Enemy)
		c.Pos = m.Pos
		taken[m.Pos] = true
		cs = append(cs, c)
		b.actors, b.foes = append(b.actors, &m.Actor), append(b.foes, m)
	}
	// The party: each member where it stands, or on the nearest free tile
	// if that one is taken (after a door they all stand on one).
	e.riding = false
	for _, m := range e.party.Members {
		p, ok := b.freeNear(board, taken, m.Pos)
		if !ok {
			return nil
		}
		taken[p] = true
		m.Place(p, m.Facing)
		c := e.combat.Combatant(m.Name, m.Name, combat.Party)
		c.Pos = p
		cs = append(cs, c)
		b.actors, b.foes = append(b.actors, m), append(b.foes, nil)
	}
	b.f = combat.New(e.combat, board, e.w.Seed^uint64(e.clock.Ticks()), cs)
	// The camera stays on the middle of the fight.
	var sx, sy int64
	for _, c := range cs {
		dx, dy, _ := e.w.Delta(n.Pos, c.Pos)
		sx, sy = sx+dx, sy+dy
	}
	if mid, ok := e.w.Offset(n.Pos, sx/int64(len(cs)), sy/int64(len(cs))); ok {
		e.camLock = &mid
	}
	e.notice = nil
	b.log = "A fight!"
	b.startTurn()
	return b
}

// freeNear returns p, or the nearest tile to it someone can stand on that
// isn't taken.
func (b *Battle) freeNear(board battleBoard, taken map[world.Pos]bool, p world.Pos) (world.Pos, bool) {
	queue, seen := []world.Pos{p}, map[world.Pos]bool{p: true}
	for i := 0; i < len(queue) && i < 200; i++ {
		q := queue[i]
		if board.Walkable(q) && !taken[q] {
			return q, true
		}
		for _, d := range world.Dirs {
			if r, ok := b.e.w.Neighbor(q, d); ok && !seen[r] && !b.e.w.Solid(r) {
				seen[r] = true
				queue = append(queue, r)
			}
		}
	}
	return p, false
}

// actions are the entries of the action bar for the active combatant.
func (b *Battle) actions() []battleAction {
	out := []battleAction{{label: "Move"}}
	for _, id := range b.f.Active().Abilities {
		ab := b.rules.Abilities[id]
		out = append(out, battleAction{label: fmt.Sprintf("%s %d", ab.Name, ab.AP), ability: id})
	}
	return append(out, battleAction{label: "End", end: true})
}

// startTurn sets the cursor for whoever's turn it is.
func (b *Battle) startTurn() {
	c := b.f.Active()
	b.action = 0
	if c.Team == combat.Party {
		b.cursor = c.Pos
	}
}

// busy reports whether something is still being shown.
func (b *Battle) busy() bool { return b.walker != nil || b.pause > 0 }

// walk sends a combatant's actor along a path.
func (b *Battle) walk(a *entity.Actor, path []world.Pos) {
	if len(path) == 0 {
		return
	}
	b.walker, b.path = a, path
	b.stepOn()
}

func (b *Battle) stepOn() {
	a, to := b.walker, b.path[0]
	b.path = b.path[1:]
	d, ok := b.e.w.DirBetween(a.Pos, to)
	if !ok {
		a.Place(to, a.Facing)
		return
	}
	a.BeginStep(d, to, battleStep)
}

// show reports a result: the damage over the target, the log line, and
// the target's push or fall.
func (b *Battle) show(r combat.Result) {
	t := b.f.Combatants()[r.Target]
	actor, target := b.actors[r.Actor], b.actors[r.Target]
	if d, ok := b.faceTo(actor.Pos, t.Pos); ok {
		actor.Facing = d
	}
	b.log = b.f.String(r)
	switch {
	case r.Pushed:
		b.walk(target, []world.Pos{r.To})
		b.popups = append(b.popups, battlePopup{"pushed", r.To, popupTicks, gfx.ColorPaper})
	case r.Damage > 0:
		b.popups = append(b.popups, battlePopup{fmt.Sprintf("-%d", r.Damage), t.Pos, popupTicks, gfx.ColorAccent})
	}
	if r.Downed {
		if n := b.foes[r.Target]; n != nil {
			n.active, n.defeated = false, true
		}
	}
	b.pause = battlePause
}

// faceTo returns the direction from p towards q along the longer axis.
func (b *Battle) faceTo(p, q world.Pos) (world.Dir, bool) {
	dx, dy, ok := b.e.w.Delta(p, q)
	switch {
	case !ok || (dx == 0 && dy == 0):
		return 0, false
	case geom.Abs(dx) >= geom.Abs(dy) && dx > 0:
		return world.Right, true
	case geom.Abs(dx) >= geom.Abs(dy):
		return world.Left, true
	case dy > 0:
		return world.Down, true
	}
	return world.Up, true
}

// act does the picked action on tile p for the party member whose turn
// it is.
func (b *Battle) act(p world.Pos) {
	acts := b.actions()
	a := acts[min(b.action, len(acts)-1)]
	c := b.f.Active()
	switch {
	case a.end:
		b.endTurn()
	case a.ability != "":
		b.use(a.ability, p)
	default:
		// Move; onto an enemy: the first ability that reaches it.
		if t := b.f.At(p); t != nil && t.Team != c.Team {
			for _, id := range c.Abilities {
				if b.f.CanUse(id, p) == nil {
					b.use(id, p)
					return
				}
			}
			b.log = "Out of reach."
			return
		}
		if p == c.Pos {
			return
		}
		path, err := b.f.Move(p)
		if err != nil {
			b.log = capitalize(err.Error()) + "."
			return
		}
		b.walk(b.actors[c.ID], path)
	}
}

func (b *Battle) use(id combat.AbilityID, p world.Pos) {
	r, err := b.f.Use(id, p)
	if err != nil {
		b.log = capitalize(err.Error()) + "."
		return
	}
	b.show(r)
}

func (b *Battle) endTurn() {
	b.f.EndTurn()
	b.startTurn()
}

// barRects returns the action bar's panel and its buttons.
func (b *Battle) barRects() (bar image.Rectangle, buttons []image.Rectangle) {
	bar = image.Rect(0, b.h-battleBarH, b.w, b.h)
	x := boxMargin
	for _, a := range b.actions() {
		w := gfx.TextWidth(a.label) + 2*battleButtonW
		buttons = append(buttons, image.Rect(x, bar.Max.Y-gfx.LineHeight-5, x+w, bar.Max.Y-2))
		x += w + 3
	}
	return bar, buttons
}

// tileAt returns the tile under a screen pixel.
func (b *Battle) tileAt(p image.Point) (world.Pos, bool) {
	m := b.f.Active().Pos.Map
	return b.e.w.Normalize(world.Pos{Map: m,
		X: geom.FloorDiv(b.e.camX+int64(p.X), tile), Y: geom.FloorDiv(b.e.camY+int64(p.Y), tile)})
}

// Update implements Scene.
func (b *Battle) Update(ctx *Context) {
	in := ctx.Input
	b.w, b.h = ctx.W, ctx.H
	b.e.updateCamera(ctx)
	// Show what is going on: walkers walk, damage floats up, pauses pass.
	for _, a := range b.actors {
		a.Update()
	}
	if b.walker != nil && !b.walker.Moving() {
		if len(b.path) > 0 {
			b.stepOn()
		} else {
			b.walker = nil
		}
	}
	for i := 0; i < len(b.popups); {
		if b.popups[i].ticks--; b.popups[i].ticks <= 0 {
			b.popups = append(b.popups[:i], b.popups[i+1:]...)
		} else {
			i++
		}
	}
	if b.pause > 0 {
		b.pause--
	}
	if b.busy() {
		return
	}
	if o := b.f.Outcome(); o != combat.Ongoing {
		if !b.done {
			b.done = true
			b.log = map[combat.Outcome]string{combat.Victory: "Victory!", combat.Defeat: "Defeated..."}[o]
			b.pause = battlePause
			return
		}
		if in.Pressed(input.Confirm) || in.Pressed(input.Cancel) || tapped(in) {
			b.finish(ctx, o)
		}
		return
	}
	c := b.f.Active()
	if c.Team == combat.Enemy {
		b.enemyTurn()
		return
	}
	if b.f.Spent() {
		b.endTurn()
		return
	}
	b.playerInput(in)
}

// enemyTurn carries out the AI's next action.
func (b *Battle) enemyTurn() {
	c := b.f.Active()
	switch a := b.f.Next(); a.Kind {
	case combat.ActMove:
		if path, err := b.f.Move(a.To); err == nil {
			b.walk(b.actors[c.ID], path)
			return
		}
		b.endTurn()
	case combat.ActUse:
		if r, err := b.f.Use(a.Ability, a.To); err == nil {
			b.show(r)
			return
		}
		b.endTurn()
	default:
		b.endTurn()
		b.pause = battlePause / 3
	}
}

func (b *Battle) playerInput(in *input.State) {
	acts := b.actions()
	if in.Pressed(input.Menu) {
		b.action = (b.action + 1) % len(acts)
	}
	if in.Pressed(input.Cancel) {
		b.action = 0
	}
	if d, ok := in.Dir(); ok {
		if in.DirPressed() || b.hold >= cursorRepeat {
			b.hold = 0
			if p, ok := b.e.w.Neighbor(b.cursor, d); ok && b.onScreen(p) {
				b.cursor = p
			}
		}
		b.hold++
	} else {
		b.hold = 0
	}
	if p, ok := in.Tap(); ok {
		_, buttons := b.barRects()
		for i, r := range buttons {
			if p.In(r) {
				b.action = i
				if acts[i].end {
					b.endTurn()
				}
				return
			}
		}
		if p.Y < b.h-battleBarH {
			if t, ok := b.tileAt(p); ok {
				b.cursor = t
				b.act(t)
			}
		}
		return
	}
	if in.Pressed(input.Confirm) {
		b.act(b.cursor)
	}
}

// onScreen reports whether a tile is in view (the cursor stays there).
func (b *Battle) onScreen(p world.Pos) bool {
	x, y := b.e.screen(p.Map, p.X*tile, p.Y*tile)
	return x >= 0 && y >= 0 && x+tile <= b.w && y+tile <= b.h-battleBarH
}

// finish ends the fight: after a victory the party gathers and walks on;
// after a defeat it wakes up where the game started, and the enemies are
// back.
func (b *Battle) finish(ctx *Context, o combat.Outcome) {
	e := b.e
	e.camLock = nil
	ctx.Pop()
	l := e.party.Leader()
	if o == combat.Victory {
		e.party.PlaceAll(l.Pos, l.Facing)
		e.say("The fight is won.")
		return
	}
	for _, n := range b.foes {
		if n != nil {
			n.active, n.defeated = true, false
		}
	}
	e.warp = &world.Warp{To: e.home, Facing: world.Down, HasFacing: true}
	e.fade = fadeTicks
	e.pendingNotice = "You come to, back where you started."
}

var (
	battleReach  = color.NRGBA{0x60, 0xa0, 0xff, 0x48}
	battleTarget = color.NRGBA{0xff, 0x50, 0x40, 0x60}
	battleHPBack = color.NRGBA{0x2b, 0x1d, 0x23, 0xff}
	battleHPFoe  = color.NRGBA{0xd0, 0x48, 0x40, 0xff}
	battleHPOwn  = color.NRGBA{0x58, 0xb0, 0x58, 0xff}
)

// Draw implements Scene.
func (b *Battle) Draw(dst *ebiten.Image) {
	e := b.e
	c := b.f.Active()
	tileRect := func(p world.Pos) image.Rectangle {
		x, y := e.screen(p.Map, p.X*tile, p.Y*tile)
		return image.Rect(x, y, x+tile, y+tile)
	}
	playing := c.Team == combat.Party && !b.busy() && b.f.Outcome() == combat.Ongoing
	if playing {
		acts := b.actions()
		a := acts[min(b.action, len(acts)-1)]
		switch {
		case a.ability != "":
			for _, p := range b.f.Targets(a.ability) {
				gfx.FillRect(dst, tileRect(p), battleTarget)
			}
		case !a.end:
			for _, r := range b.f.Reachable() {
				gfx.FillRect(dst, tileRect(r.Pos).Inset(1), battleReach)
			}
		}
		// The cursor: a frame round its tile.
		r := tileRect(b.cursor)
		for _, edge := range []image.Rectangle{
			image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
			image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y),
		} {
			gfx.FillRect(dst, edge, gfx.ColorPaper)
		}
	}
	// Health bars over everyone standing, where their sprites are; a
	// marker over whoever's turn it is.
	for _, m := range b.f.Combatants() {
		if m.Down {
			continue
		}
		a := b.actors[m.ID]
		fx, fy, _ := e.placement(a, true, 0)
		x, y := int(fx), int(fy)
		bar := image.Rect(x+1, y-5, x+tile-1, y-1)
		gfx.FillRect(dst, bar, battleHPBack)
		col := battleHPFoe
		if m.Team == combat.Party {
			col = battleHPOwn
		}
		w := (bar.Dx() - 2) * m.HP / m.MaxHP
		gfx.FillRect(dst, image.Rect(bar.Min.X+1, bar.Min.Y+1, bar.Min.X+1+w, bar.Max.Y-1), col)
		if m == c && (ebiten.Tick()/15)%2 == 0 {
			gfx.FillRect(dst, image.Rect(x+6, y-10, x+10, y-7), gfx.ColorPaper)
		}
	}
	for _, p := range b.popups {
		r := tileRect(p.at)
		rise := (popupTicks - p.ticks) / 5
		w := gfx.TextWidth(p.text)
		x, y := r.Min.X+tile/2-w/2, r.Min.Y-14-rise
		gfx.FillRect(dst, image.Rect(x-1, y+1, x+w+1, y+gfx.LineHeight), color.NRGBA{0x1b, 0x1b, 0x28, 0xb0})
		gfx.Text(dst, p.text, x, y, p.col)
	}
	// What happened, at the top.
	if b.log != "" {
		w := gfx.TextWidth(b.log) + 2*boxPad
		r := image.Rect(boxMargin, boxMargin, boxMargin+w, boxMargin+gfx.LineHeight+6)
		gfx.Box(dst, r)
		gfx.Text(dst, b.log, r.Min.X+boxPad, r.Min.Y+2, gfx.ColorInk)
	}
	// The bar: whose turn, with their numbers; on the party's turns, the
	// actions.
	bar, buttons := b.barRects()
	gfx.FillRect(dst, bar, color.NRGBA{0x1b, 0x1b, 0x28, 0xd8})
	gfx.Text(dst, fmt.Sprintf("%s  HP %d/%d  Move %d  AP %d   Round %d", c.Name, c.HP, c.MaxHP, c.MP, c.APLeft, b.f.Round),
		boxMargin, bar.Min.Y+1, gfx.ColorPaper)
	if c.Team != combat.Party || b.f.Outcome() != combat.Ongoing {
		return
	}
	for i, a := range b.actions() {
		r := buttons[i]
		fill, ink := color.Color(buttonFill), color.Color(gfx.ColorInk)
		if i == b.action {
			fill, ink = gfx.ColorPaper, gfx.ColorAccent
		}
		gfx.FillRect(dst, r, gfx.ColorInk)
		gfx.FillRect(dst, r.Inset(1), fill)
		gfx.Text(dst, a.label, r.Min.X+battleButtonW, r.Min.Y+1, ink)
	}
}
