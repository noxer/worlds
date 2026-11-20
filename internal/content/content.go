// Package content loads the game's content (world configuration, maps and
// dialogs) from an asset file system. It has no Ebitengine dependency.
package content

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/noxer/worlds/internal/combat"
	"github.com/noxer/worlds/internal/dialog"
	"github.com/noxer/worlds/internal/gen"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/quest"
	"github.com/noxer/worlds/internal/story"
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
	Infra   *infra.Infra          // settlements and networks; nil without a relief generator
	Combat  *combat.Rules         // the rules of fights; nil if there is no combat.yaml
	Items   map[string]story.Item // the kinds of items, by id
	Shops   map[string]story.Shop // the shops, by id
	Quests  quest.Book            // the quests
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
	// The items, and the wording of every dialog's conditions and effects.
	items := map[string]story.Item{}
	if data, err := fs.ReadFile(fsys, "items.yaml"); err == nil {
		if err := yaml.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("items.yaml: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("items.yaml: %w", err)
	}
	for id, it := range items {
		if it.Name == "" {
			return nil, fmt.Errorf("items.yaml: item %q has no name", id)
		}
		if u := it.Use; u != nil && (u.Heal < 0 || (u.Heal == 0 && !u.Revive)) {
			return nil, fmt.Errorf("items.yaml: using %q does nothing", id)
		}
		if q := it.Equip; q != nil && q.Slot != story.SlotWeapon && q.Slot != story.SlotArmour {
			return nil, fmt.Errorf("items.yaml: %q is worn in slot %q, not %s or %s", id, q.Slot, story.SlotWeapon, story.SlotArmour)
		}
	}
	if err := checkStory(lib, items); err != nil {
		return nil, err
	}
	// The shops (opened by the dialog action "shop <id>").
	shops := map[string]story.Shop{}
	if data, err := fs.ReadFile(fsys, "shops.yaml"); err == nil {
		if err := yaml.Unmarshal(data, &shops); err != nil {
			return nil, fmt.Errorf("shops.yaml: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("shops.yaml: %w", err)
	}
	for id, sh := range shops {
		if sh.Name == "" {
			return nil, fmt.Errorf("shops.yaml: shop %q has no name", id)
		}
		for _, it := range sh.Sells {
			if items[it].Price <= 0 {
				return nil, fmt.Errorf("shops.yaml: %s sells %q, which isn't an item with a price", id, it)
			}
		}
	}
	if err := checkShops(lib, shops); err != nil {
		return nil, err
	}
	// The quests.
	var quests quest.Book
	if data, err := fs.ReadFile(fsys, "quests.yaml"); err == nil {
		known := func(item string) bool { _, ok := items[item]; return ok }
		if quests, err = quest.Load(data, known); err != nil {
			return nil, fmt.Errorf("quests.yaml: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("quests.yaml: %w", err)
	}
	// The rules of fights, if the game has any.
	var rules *combat.Rules
	if data, err := fs.ReadFile(fsys, "combat.yaml"); err == nil {
		if rules, err = combat.LoadRules(data); err != nil {
			return nil, fmt.Errorf("combat.yaml: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("combat.yaml: %w", err)
	}
	return &Content{World: w, Infra: in, Dialogs: lib, Start: start, Combat: rules, Items: items, Quests: quests, Shops: shops}, nil
}

// checkStory checks the wording of the dialogs' conditions and effects,
// and that the items they mention exist.
// checkShops checks that the shops dialogs open ("shop <id>") exist.
func checkShops(lib dialog.Library, shops map[string]story.Shop) error {
	var errs []error
	for id, d := range lib {
		for _, a := range d.Actions() {
			if name, ok := strings.CutPrefix(a, "shop "); ok {
				if _, known := shops[name]; !known {
					errs = append(errs, fmt.Errorf("dialog %q: unknown shop %q", id, name))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func checkStory(lib dialog.Library, items map[string]story.Item) error {
	ids := make([]string, 0, len(lib))
	for id := range lib {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var errs []error
	known := func(id, item string) {
		if _, ok := items[item]; item != "" && !ok {
			errs = append(errs, fmt.Errorf("dialog %q: unknown item %q", id, item))
		}
	}
	for _, id := range ids {
		for _, c := range lib[id].Conditions() {
			item, err := story.CheckCondition(c)
			if err != nil {
				errs = append(errs, fmt.Errorf("dialog %q: condition %w", id, err))
			}
			known(id, item)
		}
		for _, e := range lib[id].Effects() {
			item, err := story.CheckEffect(e)
			if err != nil {
				errs = append(errs, fmt.Errorf("dialog %q: effect %w", id, err))
			}
			known(id, item)
		}
	}
	return errors.Join(errs...)
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
