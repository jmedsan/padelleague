package league

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// PenaltyMap returns the current penalty points for a competition.
func PenaltyMap(comp *core.Record) map[string]float64 {
	penalties := make(map[string]float64)
	if err := comp.UnmarshalJSONField("penalty_points", &penalties); err != nil {
		slog.Warn("unmarshal penalty_points", "err", err)
	}
	return penalties
}

// ApplyPenalty sets or accumulates a penalty for a pair in a competition.
// When accumulate is true the amount is added to any existing penalty;
// when false the amount replaces it.
func ApplyPenalty(app core.App, comp *core.Record, pairID string, amount float64, accumulate bool) error {
	penalties := PenaltyMap(comp)
	if accumulate {
		penalties[pairID] += amount
	} else {
		penalties[pairID] = amount
	}
	comp.Set("penalty_points", penalties)
	return app.Save(comp)
}

// RemovePenalty removes the penalty for a pair in a competition.
func RemovePenalty(app core.App, comp *core.Record, pairID string) error {
	penalties := PenaltyMap(comp)
	delete(penalties, pairID)
	comp.Set("penalty_points", penalties)
	return app.Save(comp)
}
