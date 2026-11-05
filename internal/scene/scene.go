// Package scene implements the scene stack and the game's screens.
//
// Scenes are stacked: only the top scene receives updates (and therefore
// input), but every scene is drawn from the bottom up. A dialog box pushed on
// top of the exploration scene thus freezes the world while still showing it.
package scene

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/input"
)

// Scene is one screen or overlay.
type Scene interface {
	Update(ctx *Context)
	Draw(dst *ebiten.Image)
}

// Context gives scenes access to shared services.
type Context struct {
	Input *input.State
	// Size of the logical screen in pixels.
	W, H int

	stack *Stack
}

// Push puts s on top of the stack.
func (c *Context) Push(s Scene) { c.stack.scenes = append(c.stack.scenes, s) }

// Quit asks the game to end (the desktop build closes its window).
func (c *Context) Quit() { c.stack.quit = true }

// Pop removes the top scene.
func (c *Context) Pop() {
	if n := len(c.stack.scenes); n > 0 {
		c.stack.scenes = c.stack.scenes[:n-1]
	}
}

// Stack is the scene stack.
type Stack struct {
	scenes []Scene
	ctx    Context
	quit   bool
}

// Quitting reports whether a scene asked the game to end.
func (s *Stack) Quitting() bool { return s.quit }

// NewStack creates a stack with a root scene.
func NewStack(in *input.State, root Scene) *Stack {
	s := &Stack{scenes: []Scene{root}}
	s.ctx = Context{Input: in, stack: s}
	return s
}

// SetSize updates the logical screen size.
func (s *Stack) SetSize(w, h int) { s.ctx.W, s.ctx.H = w, h }

// Update updates the top scene.
func (s *Stack) Update() {
	if n := len(s.scenes); n > 0 {
		s.scenes[n-1].Update(&s.ctx)
	}
}

// Draw draws all scenes bottom-up.
func (s *Stack) Draw(dst *ebiten.Image) {
	for _, sc := range s.scenes {
		sc.Draw(dst)
	}
}
