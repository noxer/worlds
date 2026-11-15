package combat

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// AbilityID names an ability in the rules.
type AbilityID string

// Ability is something a combatant can do on its turn, as data.
type Ability struct {
	Name   string `yaml:"name"`
	AP     int    `yaml:"ap"`     // action points it costs
	Range  [2]int `yaml:"range"`  // nearest and farthest target, in steps
	Sight  bool   `yaml:"sight"`  // needs a line of sight
	Damage [2]int `yaml:"damage"` // least and most
	// Push moves the target that many tiles straight away; the damage
	// then only applies if it can't move at all.
	Push int `yaml:"push"`
}

// Stats are a combatant's numbers.
type Stats struct {
	HP        int         `yaml:"hp"`
	Move      int         `yaml:"move"`
	AP        int         `yaml:"ap"`
	Init      int         `yaml:"init"`
	Abilities []AbilityID `yaml:"abilities"`
}

// Rules are the abilities and the stats of everyone who can fight, by kind
// (a party member's name, an enemy's kind).
type Rules struct {
	Abilities  map[AbilityID]Ability `yaml:"abilities"`
	Combatants map[string]Stats      `yaml:"combatants"`
	Default    Stats                 `yaml:"default"` // for kinds not listed
}

// LoadRules reads and checks rules from YAML.
func LoadRules(data []byte) (*Rules, error) {
	var r Rules
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("combat rules: %w", err)
	}
	for id, a := range r.Abilities {
		switch {
		case a.Name == "":
			return nil, fmt.Errorf("combat rules: ability %q has no name", id)
		case a.AP < 1:
			return nil, fmt.Errorf("combat rules: ability %q costs %d action points", id, a.AP)
		case a.Range[0] < 1 || a.Range[1] < a.Range[0]:
			return nil, fmt.Errorf("combat rules: ability %q has range %v", id, a.Range)
		case a.Damage[0] < 0 || a.Damage[1] < a.Damage[0]:
			return nil, fmt.Errorf("combat rules: ability %q has damage %v", id, a.Damage)
		}
	}
	check := func(name string, s Stats) error {
		if s.HP < 1 || s.Move < 0 || s.AP < 1 {
			return fmt.Errorf("combat rules: %s has hp %d, move %d, ap %d", name, s.HP, s.Move, s.AP)
		}
		for _, id := range s.Abilities {
			if _, ok := r.Abilities[id]; !ok {
				return fmt.Errorf("combat rules: %s has unknown ability %q", name, id)
			}
		}
		return nil
	}
	if err := check("the default", r.Default); err != nil {
		return nil, err
	}
	for name, s := range r.Combatants {
		if err := check(name, s); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// Combatant makes a combatant of a kind (the default stats if the kind
// isn't listed), at full health.
func (r *Rules) Combatant(kind, name string, team Team) Combatant {
	s, ok := r.Combatants[kind]
	if !ok {
		s = r.Default
	}
	return Combatant{Name: name, Kind: kind, Team: team, HP: s.HP, MaxHP: s.HP, Move: s.Move, AP: s.AP,
		Init: s.Init, Abilities: append([]AbilityID(nil), s.Abilities...)}
}
