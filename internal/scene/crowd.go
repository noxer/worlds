package scene

import (
	"math/rand/v2"

	"github.com/noxer/worlds/internal/entity"
	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/world"
)

// The crowd: ambient pedestrians in towns, cities and villages. They are
// not simulated away from the party: they turn up out of sight around it,
// as many as the place is busy (infra.Footfall), stroll along sidewalks,
// plazas and lanes, crossing roads at crosswalks, and are gone once the
// party has left them behind. They are NPCs like any other (they block the
// way and can be talked to), with a random line to say.

const (
	crowdEvery  = 20 // ticks between attempts to bring someone in
	crowdNear   = 13 // they turn up at least this far away (off screen)
	crowdFar    = 22 // and at most this far
	crowdGone   = 30 // beyond this, they are gone
	crowdMax    = 16
	crowdSample = 24 // tiles sampled to see how busy it is around the party
	strollTicks = 20 // a stroll is a little slower than walking
)

type crowd struct {
	rng  *rand.Rand
	tick int
	busy int // footfall summed over the sampled tiles
}

func newCrowd() crowd { return crowd{rng: rand.New(rand.NewPCG(0x63726f77, 0x64))} } // "crowd"

// Sprites of city people: RPG Urban characters, 4 directions, standing and
// two walking frames, side frames facing left.
func urbanPerson(k int) entity.Sprite {
	const cols = 27
	at := func(row, col int) int { return row*cols + col }
	r := 3 * (k % 6)
	walk := func(col int) []int { return []int{at(r, col), at(r+1, col), at(r+2, col)} }
	frames := [4][]int{}
	frames[world.Down], frames[world.Left], frames[world.Right], frames[world.Up] = walk(24), walk(23), walk(23), walk(25)
	return entity.Sprite{Sheet: "rpgurban", Index: frames[world.Down][0], Walk: &frames, FacesLeft: true}
}

// Villagers are Tiny Dungeon people, like Brookhaven's.
var villagers = []int{86, 88, 98, 111, 87}

const (
	cityLines    = 8 // dialogs crowd_city_1 ...
	villageLines = 5 // dialogs crowd_village_1 ...
)

// updateCrowd brings people in, moves them, and lets them go.
func (e *Explore) updateCrowd() {
	if e.infra == nil {
		return
	}
	l := e.party.Leader()
	c := &e.crowd
	c.tick++
	// Gone: left behind, or the party is indoors.
	kept := e.npcs[:0]
	count := 0
	for _, n := range e.npcs {
		if n.ambient {
			dx, dy, _ := e.w.Delta(l.Pos, n.Pos)
			if !l.Pos.Map.IsMain() || !n.Pos.Map.IsMain() || max(geom.Abs(dx), geom.Abs(dy)) > crowdGone {
				continue
			}
			count++
		}
		kept = append(kept, n)
	}
	e.npcs = kept
	if !l.Pos.Map.IsMain() {
		return
	}
	if c.tick%60 == 1 {
		c.busy = 0
		for range crowdSample {
			x, y := l.Pos.X+int64(c.rng.IntN(41))-20, l.Pos.Y+int64(c.rng.IntN(41))-20
			b, _, _ := e.infra.Footfall(geom.Mod(x, e.w.W), geom.Mod(y, e.w.H))
			c.busy += b
		}
	}
	if c.tick%crowdEvery == 0 && count < min(crowdMax, c.busy/3) {
		e.bringSomeone()
	}
	for _, n := range e.npcs {
		if n.ambient {
			e.stroll(n)
		}
	}
}

// bringSomeone tries a few places out of sight round the party.
func (e *Explore) bringSomeone() {
	l := e.party.Leader()
	c := &e.crowd
	for range 10 {
		d := int64(crowdNear + c.rng.IntN(crowdFar-crowdNear+1))
		var dx, dy int64
		switch c.rng.IntN(4) { // on a square ring at distance d
		case 0:
			dx, dy = d, int64(c.rng.IntN(int(2*d+1)))-d
		case 1:
			dx, dy = -d, int64(c.rng.IntN(int(2*d+1)))-d
		case 2:
			dx, dy = int64(c.rng.IntN(int(2*d+1)))-d, d
		default:
			dx, dy = int64(c.rng.IntN(int(2*d+1)))-d, -d
		}
		p, ok := e.w.Offset(l.Pos, dx, dy)
		if !ok {
			continue
		}
		busy, _, urban := e.infra.Footfall(p.X, p.Y)
		if busy == 0 || c.rng.IntN(4) >= busy || !e.strollable(p) {
			continue
		}
		n := &NPC{active: true, ambient: true}
		if urban {
			n.Sprite = urbanPerson(c.rng.IntN(6))
			n.Dialog = "crowd_city_" + itoa(1+c.rng.IntN(cityLines))
		} else {
			n.Sprite = entity.Sprite{Sheet: "tinydungeon", Index: villagers[c.rng.IntN(len(villagers))]}
			n.Dialog = "crowd_village_" + itoa(1+c.rng.IntN(villageLines))
		}
		if e.dialogs[n.Dialog] == nil {
			n.Dialog = ""
		}
		n.Place(p, world.Dirs[c.rng.IntN(4)])
		e.npcs = append(e.npcs, n)
		return
	}
}

// strollable reports whether a pedestrian may step onto p now.
func (e *Explore) strollable(p world.Pos) bool {
	if e.w.Solid(p) || e.w.Terrain(p) != world.Land || e.npcAt(p) != nil || e.party.Reserves(p) {
		return false
	}
	_, walk, _ := e.infra.Footfall(p.X, p.Y)
	return walk
}

// stroll moves a pedestrian: mostly straight on, sometimes turning or
// stopping for a while, never back the way it came unless it must.
func (e *Explore) stroll(n *NPC) {
	n.Update()
	if n.Moving() {
		return
	}
	c := &e.crowd
	if n.wait > 0 {
		n.wait--
		return
	}
	if c.rng.IntN(40) == 0 {
		n.wait = 60 + c.rng.IntN(180) // a pause
		return
	}
	back := map[world.Dir]world.Dir{world.Up: world.Down, world.Down: world.Up, world.Left: world.Right, world.Right: world.Left}[n.Facing]
	try := []world.Dir{n.Facing}
	if c.rng.IntN(5) == 0 {
		try = nil // turn here
	}
	for _, k := range c.rng.Perm(4) {
		if d := world.Dirs[k]; d != n.Facing && d != back {
			try = append(try, d)
		}
	}
	try = append(try, n.Facing, back)
	for _, d := range try {
		if q, ok := e.w.Neighbor(n.Pos, d); ok && e.strollable(q) {
			n.BeginStep(d, q, strollTicks)
			return
		}
	}
	n.wait = 30 // hemmed in: wait
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// debugTravel jumps (through the usual fade) to the plaza of the nearest
// generated village, then town, then city, then the end of the nearest
// ferry pier, then the nearest bus stop, then back home: a quick way to
// look at places that are hours apart on foot.
func (e *Explore) debugTravel() {
	if e.infra == nil {
		return
	}
	from := e.home
	var dest world.Pos
	found := false
	tiers := []infra.Tier{infra.Village, infra.Town, infra.City}
	for k := e.travel; k < len(tiers) && !found; k++ {
		e.travel = k + 1
		best := int64(-1)
		for _, s := range e.infra.Near(from.X, from.Y, 4*tiers[k].Cell(), tiers[k]) {
			if s.ID.Tier != tiers[k] || s.Fixed != "" {
				continue
			}
			dx, dy, _ := e.w.Delta(from, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
			if d := dx*dx + dy*dy; best < 0 || d < best {
				best = d
				dest, found = e.arrivalAt(s), true
			}
		}
	}
	facing := world.Down
	if !found && e.travel == len(tiers) {
		// Last, the nearest harbour with a ferry: the end of its pier,
		// looking out to sea.
		e.travel++
		if _, pier, ok := e.ferryHarbourNear(from); ok {
			end := pier.Cells[len(pier.Cells)-1]
			dest, found = world.Pos{Map: world.Main, X: end.X, Y: end.Y}, true
			facing, _ = e.w.DirBetween(dest, world.Pos{Map: world.Main, X: pier.Tip.X, Y: pier.Tip.Y})
		}
	}
	if !found && e.travel == len(tiers)+1 {
		// Then the nearest bus stop, beside its post.
		e.travel++
		if st, ok := e.busStopNear(from); ok {
			// Next to the post on the side towards the road, facing it.
			pole := world.Pos{Map: world.Main, X: st.Pole.X, Y: st.Pole.Y}
			dx, dy, _ := e.w.Delta(pole, world.Pos{Map: world.Main, X: st.Stop.X, Y: st.Stop.Y})
			toRoad := world.Down
			switch {
			case geom.Abs(dx) >= geom.Abs(dy) && dx > 0:
				toRoad = world.Right
			case geom.Abs(dx) >= geom.Abs(dy):
				toRoad = world.Left
			case dy < 0:
				toRoad = world.Up
			}
			if q, ok := e.w.Neighbor(pole, toRoad); ok {
				dest, facing, found = q, opposite(toRoad), true
			}
		}
	}
	if !found {
		dest, e.travel = e.home, 0
	}
	// Somewhere free nearby.
	for r := int64(0); r < 8 && (e.w.Solid(dest) || e.w.Terrain(dest) != world.Land); r++ {
		dest, _ = e.w.Offset(dest, 1, 0)
	}
	e.warp = &world.Warp{To: dest, Facing: facing, HasFacing: true}
	e.fade = fadeTicks
}

// busStopNear returns the bus stop nearest p (of the towns within reach).
func (e *Explore) busStopNear(p world.Pos) (infra.BusStop, bool) {
	best := int64(-1)
	var bs infra.BusStop
	for _, s := range e.infra.Near(p.X, p.Y, 4*infra.Town.Cell(), infra.Town) {
		for _, edge := range e.infra.Edges(infra.Road, s) {
			r := e.infra.BusRoute(edge)
			if r == nil {
				continue
			}
			for _, st := range []infra.BusStop{r.A, r.B} {
				dx, dy, _ := e.w.Delta(p, world.Pos{Map: world.Main, X: st.Stop.X, Y: st.Stop.Y})
				if d := dx*dx + dy*dy; best < 0 || d < best {
					best, bs = d, st
				}
			}
		}
	}
	return bs, best >= 0
}

// ferryHarbourNear returns the harbour with a ferry nearest p (as the crow
// flies, within the longest sea lane's reach).
func (e *Explore) ferryHarbourNear(p world.Pos) (infra.Settlement, infra.Pier, bool) {
	best := int64(-1)
	var bs infra.Settlement
	var bp infra.Pier
	for _, s := range e.infra.Near(p.X, p.Y, 400000, infra.Town) {
		dx, dy, _ := e.w.Delta(p, world.Pos{Map: world.Main, X: s.X, Y: s.Y})
		d := dx*dx + dy*dy
		if best >= 0 && d >= best {
			continue
		}
		pier, ok := e.infra.Harbour(s)
		if !ok {
			continue
		}
		for _, edge := range e.infra.Edges(infra.SeaLane, s) {
			if len(e.infra.FerryRoute(edge)) >= 2 {
				best, bs, bp = d, s, pier
				break
			}
		}
	}
	return bs, bp, best >= 0
}
