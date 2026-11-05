// Package content loads the game's content (world configuration, maps and
// dialogs) from an asset file system. It has no Ebitengine dependency.
package content

import (
	"fmt"
	"io/fs"

	"go.yaml.in/yaml/v3"

	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/gen"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/world"
)

// WorldFile is the format of world.yaml.
type WorldFile struct {
	Seed      uint64   `yaml:"seed"`
	Width     int64    `yaml:"width"`
	Height    int64    `yaml:"height"`
	Generator string   `yaml:"generator"`
	Tilesets  []string `yaml:"tilesets"`
	Patches   string   `yaml:"patches"`
	Details   string   `yaml:"details"`
	Pool      string   `yaml:"pool"`
	Prefabs   string   `yaml:"prefabs"`
	Start     string   `yaml:"start"`
}

// Content is everything loaded from the assets.
type Content struct {
	World   *world.World
	Infra   *infra.Infra // settlements and networks; nil without a relief generator
	Dialogs dialog.Library
	Start   world.Spawn
}

// Options override parts of world.yaml (for tools and tests).
type Options struct {
	Seed *uint64
}

// Load reads world.yaml, assembles the world and loads all dialogs.
func Load(fsys fs.FS) (*Content, error) { return LoadWith(fsys, Options{}) }

// LoadWith is Load with overrides.
func LoadWith(fsys fs.FS, opt Options) (*Content, error) {
	b, err := fs.ReadFile(fsys, "world.yaml")
	if err != nil {
		return nil, err
	}
	var wf WorldFile
	if err := yaml.Unmarshal(b, &wf); err != nil {
		return nil, fmt.Errorf("world.yaml: %w", err)
	}
	if opt.Seed != nil {
		wf.Seed = *opt.Seed
	}
	cfg := world.Config{
		Seed: wf.Seed, Width: wf.Width, Height: wf.Height,
		Tilesets: wf.Tilesets, Patches: wf.Patches, Details: wf.Details, Pool: wf.Pool, Prefabs: wf.Prefabs,
	}
	if wf.Generator != "" {
		if cfg.Generator, err = gen.ByName(wf.Generator); err != nil {
			return nil, fmt.Errorf("world.yaml: %w", err)
		}
	}
	w, err := world.Load(fsys, cfg)
	if err != nil {
		return nil, fmt.Errorf("load world: %w", err)
	}
	start, ok := w.Spawn(world.MainMap, wf.Start)
	if !ok {
		return nil, fmt.Errorf("world.yaml: start spawn %q not found on the main map", wf.Start)
	}
	lib, err := dialog.Load(fsys, "dialog")
	if err != nil {
		return nil, fmt.Errorf("load dialogs: %w", err)
	}
	in, err := loadInfra(w)
	if err != nil {
		return nil, err
	}
	return &Content{World: w, Infra: in, Dialogs: lib, Start: start}, nil
}

// loadInfra returns the infrastructure of the world's generator, if any.
func loadInfra(w *world.World) (*infra.Infra, error) {
	if g, ok := w.Generator().(interface{ Infra() *infra.Infra }); ok {
		in := g.Infra()
		// Ferries keep clear of the islands the world places at sea.
		in.SetObstacles(w.Painted)
		return in, nil
	}
	return nil, nil
}
