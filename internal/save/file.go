//go:build !js

package save

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Files keeps saves as files in a directory.
type Files string

// DefaultStore returns the store for this platform: files in the user's
// config directory (e.g. ~/.config/worlds).
func DefaultStore() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return Files(filepath.Join(dir, "worlds")), nil
}

func (f Files) path(slot int) string {
	return filepath.Join(string(f), fmt.Sprintf("save%d.json", slot+1))
}

// Load implements Store.
func (f Files) Load(slot int) ([]byte, error) {
	d, err := os.ReadFile(f.path(slot))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrEmpty
	}
	return d, err
}

// Save implements Store. It writes a temporary file and renames it, so a
// crash never leaves half a save.
func (f Files) Save(slot int, data []byte) error {
	if err := os.MkdirAll(string(f), 0o755); err != nil {
		return err
	}
	tmp := f.path(slot) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path(slot))
}
