// Package save stores saved games: what a save holds (State) and where it
// is kept (a Store: files on desktop, the browser's localStorage on the
// web). Saves are JSON, versioned; the world itself is not saved, since it
// comes from the seed (nothing in it can be changed yet).
package save

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Version is the format of State; Decode refuses others.
const Version = 1

// Slots is the number of save slots.
const Slots = 3

// State is a saved game.
type State struct {
	Version int       `json:"version"`
	Seed    uint64    `json:"seed"`  // the world's seed: a save fits one world
	Saved   time.Time `json:"saved"` // when it was saved (wall clock)
	Place   string    `json:"place"` // where, for the slot list
	Ticks   int64     `json:"ticks"` // the game clock

	// The party: members by name, leader first; where the leader stands.
	Party    []string `json:"party"`
	Map      string   `json:"map"`      // "" for the main map
	Instance uint64   `json:"instance"` // detail-map instance
	X        int64    `json:"x"`
	Y        int64    `json:"y"`
	Facing   int      `json:"facing"`
	Riding   bool     `json:"riding"`

	// Places the party has been to (settlement IDs: tier, cell x, cell y).
	Visited [][3]int64 `json:"visited"`
}

// Encode returns s as JSON.
func Encode(s State) ([]byte, error) {
	s.Version = Version
	return json.MarshalIndent(s, "", " ")
}

// Decode reads a State.
func Decode(data []byte) (State, error) {
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("save: %w", err)
	}
	if s.Version != Version {
		return State{}, fmt.Errorf("save: version %d, want %d", s.Version, Version)
	}
	return s, nil
}

// ErrEmpty is returned for a slot that holds no save.
var ErrEmpty = errors.New("save: empty slot")

// Store keeps saves by slot (0 to Slots-1).
type Store interface {
	Load(slot int) ([]byte, error) // ErrEmpty if nothing is saved there
	Save(slot int, data []byte) error
}

// Memory is a Store in memory (tests).
type Memory map[int][]byte

// Load implements Store.
func (m Memory) Load(slot int) ([]byte, error) {
	if d, ok := m[slot]; ok {
		return d, nil
	}
	return nil, ErrEmpty
}

// Save implements Store.
func (m Memory) Save(slot int, data []byte) error {
	m[slot] = append([]byte(nil), data...)
	return nil
}

// List returns the saves of all slots (nil where a slot is empty or
// unreadable).
func List(st Store) []*State {
	out := make([]*State, Slots)
	for k := range out {
		d, err := st.Load(k)
		if err != nil {
			continue
		}
		if s, err := Decode(d); err == nil {
			out[k] = &s
		}
	}
	return out
}
