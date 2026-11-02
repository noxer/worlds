package sim

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/noxer/worlds/internal/nav"
	"github.com/noxer/worlds/internal/world"
)

// Entry is one line of a daily routine: from At (minute of the day) the NPC
// wants to be at Place.
type Entry struct {
	At    int
	Place world.Pos
	Name  string // as written, for messages
}

// Schedule is a daily routine, sorted by time.
type Schedule []Entry

// ParseSchedule reads a routine as written in Tiled, e.g.
//
//	07:00 lina_home; 09:00 well_side; 12:00 smithy/counter
//
// Places are spawn names: on the main map by name, on a detail map as
// "template/name".
func ParseSchedule(w *world.World, s string) (Schedule, error) {
	var sch Schedule
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f := strings.Fields(part)
		if len(f) != 2 {
			return nil, fmt.Errorf("schedule entry %q: want \"HH:MM place\"", part)
		}
		hm := strings.Split(f[0], ":")
		if len(hm) != 2 {
			return nil, fmt.Errorf("schedule entry %q: bad time", part)
		}
		h, err1 := strconv.Atoi(hm[0])
		m, err2 := strconv.Atoi(hm[1])
		if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return nil, fmt.Errorf("schedule entry %q: bad time", part)
		}
		mapID, name := world.MainMap, f[1]
		if i := strings.IndexByte(f[1], '/'); i >= 0 {
			mapID, name = world.MapID(f[1][:i]), f[1][i+1:]
		}
		sp, ok := w.Spawn(mapID, name)
		if !ok {
			return nil, fmt.Errorf("schedule entry %q: no spawn %q on map %q", part, name, mapID)
		}
		sch = append(sch, Entry{At: h*60 + m, Place: sp.Pos, Name: f[1]})
	}
	if len(sch) == 0 {
		return nil, fmt.Errorf("empty schedule")
	}
	sort.SliceStable(sch, func(i, j int) bool { return sch[i].At < sch[j].At })
	for i := 1; i < len(sch); i++ {
		if sch[i].At == sch[i-1].At {
			return nil, fmt.Errorf("schedule has two entries at %02d:%02d", sch[i].At/60, sch[i].At%60)
		}
	}
	return sch, nil
}

// Current returns the entry in effect at an absolute game minute, the one
// before it (where the NPC came from), and the absolute minute it started.
// Before the first entry of a day, the last one of the previous day holds.
func (s Schedule) Current(minutes int64) (cur, prev Entry, since int64) {
	day, tod := minutes/MinutesPerDay, int(minutes%MinutesPerDay)
	i := sort.Search(len(s), func(i int) bool { return s[i].At > tod }) - 1
	if i < 0 {
		i = len(s) - 1
		day--
	}
	cur = s[i]
	prev = s[(i-1+len(s))%len(s)]
	return cur, prev, day*MinutesPerDay + int64(cur.At)
}

// Where returns where an NPC following the schedule is at an absolute game
// minute, without simulating it tick by tick: it set off from the previous
// place when the current entry began and walks the planner's route at
// walking speed for the time since. Used when the NPC comes back into the
// party's reach.
func Where(p *nav.Planner, w *world.World, s Schedule, minutes int64) world.Pos {
	cur, prev, since := s.Current(minutes)
	budget := (minutes - since) * TicksPerMinute
	pos := prev.Place
	for legs := 0; legs < 32 && pos != cur.Place; legs++ {
		moves, ok := p.Plan(pos, cur.Place, nav.Foot, nil)
		if !ok || len(moves) == 0 {
			return cur.Place // no route: be there (doors, teleports later)
		}
		for _, mv := range moves {
			t, ok := nav.StepTicks(w, mv.To, nav.Foot)
			if !ok {
				t = nav.WalkTicks
			}
			if budget < int64(t) {
				return pos
			}
			budget -= int64(t)
			pos = mv.To
			if mv.Warp != nil {
				pos = mv.Warp.To
			}
		}
	}
	return pos
}
