package scene

import "time"

// Perf holds the worst frame timings of the last second, for the debug
// overlay. The game records them; scenes only read them.
var Perf perfStats

type perfStats struct {
	Pump, Update, Draw          time.Duration // worst in the last second
	curPump, curUpdate, curDraw time.Duration
	ticks                       int
}

// RecordTick records the time a tick spent building chunks and updating.
// It reports whether the tick was slow (over 8 ms, half a frame).
func (p *perfStats) RecordTick(pump, update time.Duration) bool {
	p.curPump = max(p.curPump, pump)
	p.curUpdate = max(p.curUpdate, update)
	if p.ticks++; p.ticks >= 60 {
		p.Pump, p.Update, p.Draw = p.curPump, p.curUpdate, p.curDraw
		p.curPump, p.curUpdate, p.curDraw, p.ticks = 0, 0, 0, 0
	}
	return pump+update > 8*time.Millisecond
}

// RecordDraw records the time a frame took to draw.
func (p *perfStats) RecordDraw(d time.Duration) { p.curDraw = max(p.curDraw, d) }
