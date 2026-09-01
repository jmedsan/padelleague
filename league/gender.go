package league

import "fmt"

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
		if !((g1 == "male" && g2 == "female") || (g1 == "female" && g2 == "male")) {
			return fmt.Errorf("las parejas mixtas deben tener un jugador y una jugadora")
		}
	}

	return nil
}
