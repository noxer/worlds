package scene

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noxer/worlds/internal/world"
)

// Puzzle pieces are objects on the maps, wired up by name in Tiled:
//
//   - "lever": confirm while facing it to pull it, on or off. Its "target"
//     names the doors it works (comma-separated, on the same map).
//   - "plate": a pressure plate, pressed while a block or a party member
//     stands on it; "target" as for a lever.
//   - "block": a crate the party pushes a tile by walking into it, if the
//     tile beyond is free floor. A crate pushed into a corner can't come
//     out, so when the party leaves a map, its crates go back to where
//     they started, unless they rest on a plate.
//   - "door": shut until every lever and plate wired to it is on (or any
//     one of them, with the property "mode" set to "any"). It doesn't shut
//     on anyone standing in it.
//
// Levers, blocks and shut doors are solid. A lever's position is kept as
// the story flag "lever.<map>/<name>", so dialogs and quests can ask about
// it; blocks moved are kept in the save.

type pieceKind uint8

const (
	pzLever pieceKind = iota
	pzPlate
	pzBlock
	pzDoor
)

var pieceKinds = map[string]pieceKind{"lever": pzLever, "plate": pzPlate, "block": pzBlock, "door": pzDoor}

// piece is a puzzle piece.
type piece struct {
	key       string // "<map>/<name>", unique
	kind      pieceKind
	pos, home world.Pos // where it is, and where it started (blocks move)
	targets   []string  // lever, plate: the keys of its doors
	any       bool      // door: opens when any one of its sources is on
	sources   []*piece  // door: the levers and plates wired to it
	on        bool      // lever pulled, plate pressed, door open
}

// puzzles are all the pieces in the world.
type puzzles struct {
	all   []*piece
	byKey map[string]*piece
}

// addPiece reads a puzzle object; false if o isn't one.
func (e *Explore) addPiece(o world.Object) (bool, error) {
	kind, ok := pieceKinds[o.Type]
	if !ok {
		return false, nil
	}
	if e.puzzle.byKey == nil {
		e.puzzle.byKey = map[string]*piece{}
	}
	key := string(o.Pos.Map.Template) + "/" + o.Name
	if o.Pos.Map.IsMain() {
		key = "main/" + o.Name
	}
	if o.Name == "" {
		return true, fmt.Errorf("%v: a %s has no name", o.Pos, o.Type)
	}
	if _, dup := e.puzzle.byKey[key]; dup {
		return true, fmt.Errorf("%v: two puzzle pieces are called %q", o.Pos, o.Name)
	}
	p := &piece{key: key, kind: kind, pos: o.Pos, home: o.Pos, any: o.Props.String("mode") == "any"}
	prefix := key[:strings.LastIndex(key, "/")+1]
	for _, t := range strings.Split(o.Props.String("target"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			p.targets = append(p.targets, prefix+t)
		}
	}
	if (kind == pzLever || kind == pzPlate) && len(p.targets) == 0 {
		return true, fmt.Errorf("%v: %s %q has no target", o.Pos, o.Type, o.Name)
	}
	e.puzzle.all = append(e.puzzle.all, p)
	e.puzzle.byKey[key] = p
	return true, nil
}

// wirePuzzles connects levers and plates to their doors and blocks the
// solid pieces, once every object has been read.
func (e *Explore) wirePuzzles() error {
	for _, p := range e.puzzle.all {
		for _, t := range p.targets {
			d, ok := e.puzzle.byKey[t]
			if !ok || d.kind != pzDoor {
				return fmt.Errorf("%s: target %q is not a door on its map", p.key, t)
			}
			d.sources = append(d.sources, p)
		}
	}
	for _, p := range e.puzzle.all {
		if p.kind == pzDoor && len(p.sources) == 0 {
			return fmt.Errorf("%s: no lever or plate opens this door", p.key)
		}
		if p.kind != pzPlate {
			e.w.SetBlocked(p.pos, true) // doors start shut
		}
	}
	return nil
}

// pieceAt returns the piece of a kind on tile p, if any.
func (e *Explore) pieceAt(p world.Pos, kind pieceKind) *piece {
	for _, q := range e.puzzle.all {
		if q.kind == kind && q.pos == p {
			return q
		}
	}
	return nil
}

// leverFlag is the story flag of a lever pulled.
func leverFlag(l *piece) string { return "lever." + l.key }

// pullLever turns a lever over.
func (e *Explore) pullLever(l *piece) {
	l.on = !l.on
	e.story.SetFlag(leverFlag(l), l.on)
	e.updatePuzzles()
	e.say("Clunk.")
}

// pushBlock moves a block a tile in direction d, if the tile beyond is free
// floor; it reports whether it did.
func (e *Explore) pushBlock(b *piece, d world.Dir) bool {
	to, ok := e.w.Neighbor(b.pos, d)
	if !ok || e.w.Solid(to) || e.w.Terrain(to) != world.Land || e.npcAt(to) != nil || e.party.Occupies(to) ||
		e.w.WarpAt(to) != nil {
		return false
	}
	e.moveBlock(b, to)
	return true
}

func (e *Explore) moveBlock(b *piece, to world.Pos) {
	e.w.SetBlocked(b.pos, false)
	b.pos = to
	e.w.SetBlocked(b.pos, true)
}

// occupied reports whether someone stands on p (a member of the party, or
// an NPC).
func (e *Explore) occupied(p world.Pos) bool {
	if e.npcAt(p) != nil {
		return true
	}
	for _, m := range e.party.Members {
		if m.Pos == p {
			return true
		}
	}
	return false
}

// updatePuzzles presses the plates that bear something and opens or shuts
// the doors to match.
func (e *Explore) updatePuzzles() {
	for _, p := range e.puzzle.all {
		if p.kind != pzPlate {
			continue
		}
		p.on = e.pieceAt(p.pos, pzBlock) != nil
		for _, m := range e.party.Members {
			p.on = p.on || m.Pos == p.pos
		}
	}
	for _, d := range e.puzzle.all {
		if d.kind != pzDoor {
			continue
		}
		open := !d.any
		for _, s := range d.sources {
			if d.any {
				open = open || s.on
			} else {
				open = open && s.on
			}
		}
		switch {
		case open && !d.on:
			d.on = true
			e.w.SetBlocked(d.pos, false)
		case !open && d.on && !e.occupied(d.pos):
			d.on = false
			e.w.SetBlocked(d.pos, true)
		}
	}
}

// puzzleBump says what a piece is when the party walks into it; it reports
// whether there was one.
func (e *Explore) puzzleBump(p world.Pos) bool {
	switch {
	case e.pieceAt(p, pzLever) != nil:
		e.say("A lever. Press to pull it.")
	case e.pieceAt(p, pzBlock) != nil:
		e.say("It won't budge that way.")
	case e.pieceAt(p, pzDoor) != nil:
		e.say("The door is shut fast. There's no keyhole.")
	default:
		return false
	}
	return true
}

// puzzleDrawables are the pieces on map m to draw, relative to the camera
// at alpha.
func (e *Explore) puzzleDrawables(m world.MapRef, alpha float64) []drawable {
	cx, cy := e.cameraAt(alpha)
	var out []drawable
	for _, p := range e.puzzle.all {
		if p.pos.Map != m {
			continue
		}
		name, priority := "", 0
		switch p.kind {
		case pzLever:
			name = map[bool]string{false: "lever off", true: "lever on"}[p.on]
		case pzPlate:
			name, priority = map[bool]string{false: "plate", true: "plate down"}[p.on], -1000 // under whoever stands on it
		case pzBlock:
			name = "block"
		case pzDoor:
			name, priority = map[bool]string{false: "door closed", true: "door open"}[p.on], -1000
		}
		x, y := e.screenAt(m, float64(p.pos.X*tile), float64(p.pos.Y*tile), cx, cy)
		if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
			continue
		}
		if img, ok := e.vehicleTile(name); ok {
			out = append(out, drawable{img: img, x: x, y: y, priority: priority})
		}
	}
	return out
}

// resetBlocks puts the crates of map m back where they started, except
// those on a plate (the party has left the map: a crate stuck in a corner
// gets another go).
func (e *Explore) resetBlocks(m world.MapRef) {
	for _, p := range e.puzzle.all {
		if p.kind == pzBlock && p.pos.Map == m && p.pos != p.home && e.pieceAt(p.pos, pzPlate) == nil {
			e.moveBlock(p, p.home)
		}
	}
	e.updatePuzzles()
}

// Saving: the blocks moved; levers are story flags.

// puzzleBlocks returns the blocks away from where they started, by key.
func (e *Explore) puzzleBlocks() map[string][2]int64 {
	var out map[string][2]int64
	for _, p := range e.puzzle.all {
		if p.kind == pzBlock && p.pos != p.home {
			if out == nil {
				out = map[string][2]int64{}
			}
			out[p.key] = [2]int64{p.pos.X, p.pos.Y}
		}
	}
	return out
}

// restorePuzzles puts the pieces as a save has them: levers by their
// flags (the story must be restored first), blocks where they were.
func (e *Explore) restorePuzzles(blocks map[string][2]int64) {
	keys := make([]string, 0, len(e.puzzle.byKey))
	for k := range e.puzzle.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := e.puzzle.byKey[k]
		switch p.kind {
		case pzLever:
			p.on = e.story.Flag(leverFlag(p))
		case pzBlock:
			to := p.home
			if xy, ok := blocks[k]; ok {
				to.X, to.Y = xy[0], xy[1]
			}
			e.moveBlock(p, to)
		}
	}
	e.updatePuzzles()
}
