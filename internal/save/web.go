//go:build js

package save

import (
	"errors"
	"fmt"
	"syscall/js"
)

// Browser keeps saves in the browser's localStorage, per site.
type Browser struct{}

// DefaultStore returns the store for this platform: localStorage.
func DefaultStore() (Store, error) {
	if ls := js.Global().Get("localStorage"); ls.IsUndefined() || ls.IsNull() {
		return nil, errors.New("save: no localStorage")
	}
	return Browser{}, nil
}

func key(slot int) string { return fmt.Sprintf("worlds.save%d", slot+1) }

// Load implements Store.
func (Browser) Load(slot int) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil { // storage blocked (private mode, sandbox)
			data, err = nil, fmt.Errorf("save: %v", r)
		}
	}()
	v := js.Global().Get("localStorage").Call("getItem", key(slot))
	if v.IsNull() || v.IsUndefined() {
		return nil, ErrEmpty
	}
	return []byte(v.String()), nil
}

// Save implements Store.
func (Browser) Save(slot int, data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil { // full, or blocked
			err = fmt.Errorf("save: %v", r)
		}
	}()
	js.Global().Get("localStorage").Call("setItem", key(slot), string(data))
	return nil
}
