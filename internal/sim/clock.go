// Package sim holds the world's simulation that runs whether or not the
// party is near: the world clock, and NPC schedules with the cheap
// "where would they be by now" position used while they are out of reach.
package sim

import "fmt"

// Clock is game time. It advances one tick per game tick while the world
// runs (not during dialogs or menus).
type Clock struct{ ticks int64 }

const (
	// TicksPerMinute: one game minute per real second at 60 ticks per
	// second, so a game day lasts 24 real minutes.
	TicksPerMinute = 60
	// MinutesPerDay is the length of a day in game minutes.
	MinutesPerDay = 24 * 60
)

// NewClock returns a clock set to a day and time.
func NewClock(day int64, hour, minute int) Clock {
	return Clock{ticks: (day*MinutesPerDay + int64(hour*60+minute)) * TicksPerMinute}
}

// Tick advances the clock by one game tick.
func (c *Clock) Tick() { c.ticks++ }

// Advance moves the clock forward by a number of game minutes.
func (c *Clock) Advance(minutes int) { c.ticks += int64(minutes) * TicksPerMinute }

// Ticks returns the game ticks since day 0, 00:00.
func (c Clock) Ticks() int64 { return c.ticks }

// Minutes returns the game minutes since day 0, 00:00.
func (c Clock) Minutes() int64 { return c.ticks / TicksPerMinute }

// Day returns the day number.
func (c Clock) Day() int64 { return c.Minutes() / MinutesPerDay }

// TimeOfDay returns the hour and minute.
func (c Clock) TimeOfDay() (hour, minute int) {
	m := int(c.Minutes() % MinutesPerDay)
	return m / 60, m % 60
}

func (c Clock) String() string {
	h, m := c.TimeOfDay()
	return fmt.Sprintf("day %d, %02d:%02d", c.Day(), h, m)
}
