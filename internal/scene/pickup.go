package scene

import (
	"fmt"

	"github.com/noxer/worlds/internal/story"
	"github.com/noxer/worlds/internal/world"
)

// Pickups are things to take: a tile object of type "pickup" with the
// properties "item" (an id from items.yaml), "count" (1 if not set) and
// "text" (what the party sees as it takes it). Confirm while facing it (or
// tap it) to take it; it is solid until then. Taken, it is gone for good:
// the story flag "taken.<map>/<name>" says so, so it is saved and quests
// and dialogs can ask about it.

// pickup is a thing to take.
type pickup struct {
	key   string // "<map>/<name>"
	pos   world.Pos
	item  string
	count int
	text  string
	img   world.Tile // its sprite
}

// addPickup reads a pickup object.
func (e *Explore) addPickup(o world.Object) error {
	if o.Name == "" || o.Props.String("item") == "" || o.Tile.Empty() {
		return fmt.Errorf("%v: a pickup needs a name, an item and a tile", o.Pos)
	}
	if _, ok := e.items[o.Props.String("item")]; !ok && e.items != nil {
		return fmt.Errorf("%v: pickup %q: unknown item %q", o.Pos, o.Name, o.Props.String("item"))
	}
	key := string(o.Pos.Map.Template) + "/" + o.Name
	if o.Pos.Map.IsMain() {
		key = "main/" + o.Name
	}
	for _, p := range e.pickups {
		if p.key == key {
			return fmt.Errorf("%v: two pickups are called %q", o.Pos, o.Name)
		}
	}
	p := &pickup{key: key, pos: o.Pos, item: o.Props.String("item"), count: max(o.Props.Int("count", 1), 1),
		text: o.Props.String("text"), img: o.Tile}
	e.pickups = append(e.pickups, p)
	e.w.SetBlocked(p.pos, true)
	return nil
}

func takenFlag(p *pickup) string { return "taken." + p.key }

// pickupAt returns the pickup still lying on p, if any.
func (e *Explore) pickupAt(pos world.Pos) *pickup {
	for _, p := range e.pickups {
		if p.pos == pos && !e.story.Flag(takenFlag(p)) {
			return p
		}
	}
	return nil
}

// take picks a pickup up.
func (e *Explore) take(p *pickup) {
	e.story.SetFlag(takenFlag(p), true)
	e.story.Give(p.item, p.count)
	e.w.SetBlocked(p.pos, false)
	text := e.changesText([]story.Change{{Item: p.item, N: p.count}})
	if p.text != "" {
		text = p.text + " " + text
	}
	e.say(text)
	e.updateQuests()
}

// restorePickups lays the pickups out as the story says (a loaded game):
// those taken are gone, the others back.
func (e *Explore) restorePickups() {
	for _, p := range e.pickups {
		e.w.SetBlocked(p.pos, false)
		if !e.story.Flag(takenFlag(p)) {
			e.w.SetBlocked(p.pos, true)
		}
	}
}

// pickupDrawables are the pickups lying on map m, relative to the camera.
func (e *Explore) pickupDrawables(m world.MapRef, alpha float64) []drawable {
	cx, cy := e.cameraAt(alpha)
	sets := e.w.Tilesets()
	var out []drawable
	for _, p := range e.pickups {
		if p.pos.Map != m || e.story.Flag(takenFlag(p)) {
			continue
		}
		x, y := e.screenAt(m, float64(p.pos.X*tile), float64(p.pos.Y*tile), cx, cy)
		if x < -tile || y < -tile || x > float64(e.viewW) || y > float64(e.viewH) {
			continue
		}
		img := e.sheets.Tile(sets.Get(p.img.Set()).Name, p.img.Index())
		out = append(out, drawable{img: img, x: x, y: y})
	}
	return out
}
