package produce

import "slices"

// products lists what each kind of farm sells and the units each is sold in; the page
// words the codes. OTHER is open to every farm.
var products = map[string]map[string][]string{
	"POULTRY": {
		"BROILER": {"KG", "PIECE"},
		"EGG":     {"PIECE", "HALI", "TRAY"},
		"LITTER":  {"SACK"},
		"SACK":    {"ITEM"},
	},
	"CATTLE": {
		"MILK":   {"LITRE"},
		"CATTLE": {"HEAD"},
		"DUNG":   {"SACK", "KG"},
	},
	"FISH": {
		"FISH": {"KG", "MAUND"},
	},
	"CROP": {
		"CROP": {"KG", "MAUND"},
	},
}

var otherUnits = []string{"ITEM", "KG"}

// sellable reports whether a farm of farmType sells product in unit.
func sellable(farmType, product, unit string) bool {
	if product == "OTHER" {
		return slices.Contains(otherUnits, unit)
	}
	units, ok := products[farmType][product]
	return ok && slices.Contains(units, unit)
}
