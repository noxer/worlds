package infra

// Names of settlements, made up from the seed: a first part and an ending,
// such as "Kelbridge" or "Marwick". Hand-made settlements take the
// "title" property of their patch.

var nameStarts = []string{"Ash", "Bel", "Cor", "Dun", "El", "Fen", "Gar", "Hal", "Iver", "Kel",
	"Lan", "Mar", "Nor", "Oak", "Pen", "Rav", "Sel", "Thorn", "Wil", "Yar", "Brim", "Calder",
	"Dray", "Esk", "Fair", "Glen", "Hart", "Lind", "Moss", "Red", "Stan", "Wester"}

var nameEnds = []string{"ford", "bridge", "mouth", "ton", "by", "field", "haven", "wick",
	"stead", "bury", "dale", "port", "minster", "moor", "holt", "well", "gate", "hurst"}

// Name returns the name of a settlement.
func (in *Infra) Name(s Settlement) string {
	if s.Fixed != "" {
		if t, ok := in.titles[s.ID]; ok {
			return t
		}
		return s.Fixed
	}
	h := hash(in.seed^salt, 0x6e616d65, int64(s.ID.Tier), s.ID.CX, s.ID.CY) // "name"
	return nameStarts[h%uint64(len(nameStarts))] + nameEnds[h>>16%uint64(len(nameEnds))]
}
