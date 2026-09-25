package league

import "fmt"

// genderTypeLabels is the single source of truth for a competition's
// gender_type Spanish label — shared by the create/edit form <option>s and
// the admin activity log.
var genderTypeLabels = map[string]string{
	"free":   "Libre",
	"male":   "Masculina",
	"female": "Femenina",
	"mixed":  "Mixta",
}

// GenderTypeLabel returns the Spanish label for a competition's gender_type
// value, or the raw value if unrecognized.
func GenderTypeLabel(genderType string) string {
	if label, ok := genderTypeLabels[genderType]; ok {
		return label
	}
	return genderType
}

// ValidatePairComposition reports whether a pair of the given genders may
// enter a competition of the given gender-type.
func ValidatePairComposition(genderType, g1, g2 string) error {
	if genderType == "free" {
		return nil
	}

	if g1 == "" || g2 == "" {
		return fmt.Errorf("los jugadores deben tener género asignado")
	}

	switch genderType {
	case "male":
		if g1 != "male" || g2 != "male" {
			return fmt.Errorf("esta competición es solo masculina")
		}
	case "female":
		if g1 != "female" || g2 != "female" {
			return fmt.Errorf("esta competición es solo femenina")
		}
	case "mixed":
		if (g1 != "male" || g2 != "female") && (g1 != "female" || g2 != "male") {
			return fmt.Errorf("las parejas mixtas deben tener un jugador y una jugadora")
		}
	}

	return nil
}
