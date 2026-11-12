package scene

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/save"
	"github.com/noxer/worlds/internal/sim"
	"github.com/noxer/worlds/internal/world"
)

// Saving and loading the game (see package save). A save holds the clock,
// the party and where it stands, and the places visited; everything else
// follows from the world's seed and the clock (NPCs, trains, the crowd).

// ErrCantSave is returned by Snapshot when the game can't be saved now.
var ErrCantSave = errors.New("you can't save while travelling")

// Snapshot returns the game as a save.
func (e *Explore) Snapshot() (save.State, error) {
	if e.ride != nil || e.fade != 0 || e.warp != nil {
		return save.State{}, ErrCantSave
	}
	l := e.party.Leader()
	st := save.State{
		Seed: e.w.Seed, Saved: time.Now().UTC().Truncate(time.Second), Place: e.placeName(),
		Ticks: e.clock.Ticks(),
		Map:   string(l.Pos.Map.Template), Instance: l.Pos.Map.Instance, X: l.Pos.X, Y: l.Pos.Y,
		Facing: int(l.Facing), Riding: e.riding,
	}
	for _, m := range e.party.Members {
		st.Party = append(st.Party, m.Name)
	}
	ids := make([]infra.ID, 0, len(e.visited))
	for id := range e.visited {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b infra.ID) int { return cmpID(a, b) })
	for _, id := range ids {
		st.Visited = append(st.Visited, [3]int64{int64(id.Tier), id.CX, id.CY})
	}
	return st, nil
}

func cmpID(a, b infra.ID) int {
	switch {
	case a.Tier != b.Tier:
		return int(a.Tier) - int(b.Tier)
	case a.CX != b.CX:
		if a.CX < b.CX {
			return -1
		}
		return 1
	case a.CY < b.CY:
		return -1
	case a.CY > b.CY:
		return 1
	}
	return 0
}

// Restore puts the game into the state of a save: the clock is set at
// once, the party fades over to where it was saved.
func (e *Explore) Restore(st save.State) error {
	if st.Seed != e.w.Seed {
		return fmt.Errorf("that save belongs to another world (seed %d)", st.Seed)
	}
	pos := world.Pos{Map: world.MapRef{Template: world.MapID(st.Map), Instance: st.Instance}, X: st.X, Y: st.Y}
	if st.Map == "" {
		pos.Map = world.Main
	}
	p, ok := e.w.Normalize(pos)
	if !ok {
		return fmt.Errorf("the save's place %v is not in this world", pos)
	}
	if st.Facing < 0 || st.Facing > 3 {
		return fmt.Errorf("bad facing %d", st.Facing)
	}
	// The party in the saved order (members not in the save keep their
	// place behind).
	order := map[string]int{}
	for k, n := range st.Party {
		order[n] = k
	}
	slices.SortStableFunc(e.party.Members, func(a, b *entity.Actor) int { return rank(order, a.Name) - rank(order, b.Name) })
	e.clock = sim.ClockAt(st.Ticks)
	clear(e.visited)
	if e.infra != nil {
		for _, v := range st.Visited {
			if s, ok := e.infra.Get(infra.ID{Tier: infra.Tier(v[0]), CX: v[1], CY: v[2]}); ok {
				e.visited[s.ID] = s
			}
		}
	}
	e.ride, e.lastTaxi, e.notice, e.pendingNotice = nil, nil, nil, ""
	e.nearLines, e.ranks = nil, nil
	e.path = nil
	e.restoreRiding = st.Riding
	e.warp = &world.Warp{To: p, Facing: world.Dir(st.Facing), HasFacing: true}
	e.fade = fadeTicks
	return nil
}

func rank(order map[string]int, name string) int {
	if k, ok := order[name]; ok {
		return k
	}
	return len(order)
}

// placeName says where the party is, for the list of saves.
func (e *Explore) placeName() string {
	l := e.party.Leader()
	if !l.Pos.Map.IsMain() {
		return string(l.Pos.Map.Template)
	}
	if e.infra != nil {
		best := int64(-1)
		name := ""
		for _, s := range e.infra.Near(l.Pos.X, l.Pos.Y, 3000, infra.Village) {
			dx, dy, _ := e.w.Delta(l.Pos, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
			if d := dx*dx + dy*dy; best < 0 || d < best {
				best, name = d, e.infra.Name(s)
			}
		}
		switch {
		case best < 0:
			return "the wilds"
		case best <= 600*600:
			return name
		default:
			return "near " + name
		}
	}
	return "the world"
}
