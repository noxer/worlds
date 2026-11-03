// Package input maps keyboard, gamepad, mouse and touch to game actions.
// Game code never reads raw keys, which keeps desktop, browser and (later)
// mobile controls in one place.
package input

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/noxer/worlds/internal/world"
)

// Action is a logical button.
type Action int

const (
	Up Action = iota
	Down
	Left
	Right
	Confirm
	Cancel
	Menu
	Ride
	Debug
	// DebugTime skips an hour while the debug overlay is on.
	DebugTime
	// DebugTravel jumps to the nearest village, town, city and home in
	// turn while the debug overlay is on.
	DebugTravel
	// NumActions is the number of actions.
	NumActions
)

var keys = [NumActions][]ebiten.Key{
	Up:          {ebiten.KeyArrowUp, ebiten.KeyW},
	Down:        {ebiten.KeyArrowDown, ebiten.KeyS},
	Left:        {ebiten.KeyArrowLeft, ebiten.KeyA},
	Right:       {ebiten.KeyArrowRight, ebiten.KeyD},
	Confirm:     {ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyE, ebiten.KeyZ},
	Cancel:      {ebiten.KeyEscape, ebiten.KeyBackspace, ebiten.KeyX},
	Menu:        {ebiten.KeyTab, ebiten.KeyM},
	Ride:        {ebiten.KeyR},
	Debug:       {ebiten.KeyF3, ebiten.KeyI, ebiten.KeyBackquote}, // I: browsers take F3, and ` is a dead key on some layouts
	DebugTime:   {ebiten.KeyN},
	DebugTravel: {ebiten.KeyT},
}

var pads = [NumActions][]ebiten.StandardGamepadButton{
	Up:      {ebiten.StandardGamepadButtonLeftTop},
	Down:    {ebiten.StandardGamepadButtonLeftBottom},
	Left:    {ebiten.StandardGamepadButtonLeftLeft},
	Right:   {ebiten.StandardGamepadButtonLeftRight},
	Confirm: {ebiten.StandardGamepadButtonRightBottom},
	Cancel:  {ebiten.StandardGamepadButtonRightRight},
	Menu:    {ebiten.StandardGamepadButtonCenterRight},
	Ride:    {ebiten.StandardGamepadButtonRightTop},
}

var dirActions = [4]struct {
	a Action
	d world.Dir
}{{Up, world.Up}, {Down, world.Down}, {Left, world.Left}, {Right, world.Right}}

const stickThreshold = 0.5

// Frame is the raw device state sampled for one tick. Tests, replays and
// automation can feed frames directly via Apply.
type Frame struct {
	Held   [NumActions]bool
	Tap    image.Point
	Tapped bool
}

// State is the input state of the current tick.
type State struct {
	tick      int
	held      [NumActions]bool
	just      [NumActions]bool
	pressedAt [NumActions]int

	pads  []ebiten.GamepadID
	touch []ebiten.TouchID

	tap    image.Point
	tapped bool

	// ToLogical converts a pointer position on the screen to game pixels.
	// Nil means they are the same.
	ToLogical func(image.Point) image.Point
}

// Update samples all devices. Call once per tick before scenes update.
func (s *State) Update() { s.Apply(s.sample()) }

// Apply advances the state by one tick using the given raw frame.
func (s *State) Apply(f Frame) {
	s.tick++
	for a := Action(0); a < NumActions; a++ {
		h := f.Held[a]
		s.just[a] = h && !s.held[a]
		if s.just[a] {
			s.pressedAt[a] = s.tick
		}
		s.held[a] = h
	}
	s.tap, s.tapped = f.Tap, f.Tapped
}

func (s *State) sample() Frame {
	var f Frame
	s.pads = ebiten.AppendGamepadIDs(s.pads[:0])
	for a := Action(0); a < NumActions; a++ {
		for _, k := range keys[a] {
			f.Held[a] = f.Held[a] || ebiten.IsKeyPressed(k)
		}
		for _, id := range s.pads {
			if !ebiten.IsStandardGamepadLayoutAvailable(id) {
				continue
			}
			for _, b := range pads[a] {
				f.Held[a] = f.Held[a] || ebiten.IsStandardGamepadButtonPressed(id, b)
			}
			f.Held[a] = f.Held[a] || stick(id, a)
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		f.Tap, f.Tapped = image.Pt(ebiten.CursorPosition()), true
	}
	s.touch = inpututil.AppendJustPressedTouchIDs(s.touch[:0])
	if len(s.touch) > 0 {
		f.Tap, f.Tapped = image.Pt(ebiten.TouchPosition(s.touch[0])), true
	}
	if f.Tapped && s.ToLogical != nil {
		f.Tap = s.ToLogical(f.Tap)
	}
	return f
}

func stick(id ebiten.GamepadID, a Action) bool {
	x := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
	y := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
	switch a {
	case Up:
		return y < -stickThreshold
	case Down:
		return y > stickThreshold
	case Left:
		return x < -stickThreshold
	case Right:
		return x > stickThreshold
	}
	return false
}

// Held reports whether the action is down.
func (s *State) Held(a Action) bool { return s.held[a] }

// Pressed reports whether the action went down this tick.
func (s *State) Pressed(a Action) bool { return s.just[a] }

// Dir returns the held direction. If several are held, the most recently
// pressed one wins, which makes cornering with the keyboard feel right.
func (s *State) Dir() (d world.Dir, ok bool) {
	best := -1
	for _, da := range dirActions {
		if s.held[da.a] && s.pressedAt[da.a] > best {
			best, d, ok = s.pressedAt[da.a], da.d, true
		}
	}
	return d, ok
}

// DirPressed reports whether the direction returned by Dir went down this tick.
func (s *State) DirPressed() bool {
	return s.just[Up] || s.just[Down] || s.just[Left] || s.just[Right]
}

// Tap returns the screen position of a mouse click or touch this tick.
func (s *State) Tap() (image.Point, bool) { return s.tap, s.tapped }

// ConsumeTap marks this tick's tap as handled.
func (s *State) ConsumeTap() { s.tapped = false }
