package handlers

import "math"

func ComputeELO(winnerELO, loserELO int) (winnerNew, loserNew int) {
	expected := 1.0 / (1.0 + math.Pow(10, float64(loserELO-winnerELO)/400))
	delta := int(math.Round(32 * (1 - expected)))
	return winnerELO + delta, loserELO - delta
}
