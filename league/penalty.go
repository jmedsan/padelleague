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

// SetPenalty replaces the penalty for a pair in a competition.
func SetPenalty(app core.App, comp *core.Record, pairID string, amount float64) error {
	penalties := PenaltyMap(comp)
	penalties[pairID] = amount
	comp.Set("penalty_points", penalties)
	return app.Save(comp)
}

// AccumulatePenalty adds to any existing penalty for a pair in a competition.
func AccumulatePenalty(app core.App, comp *core.Record, pairID string, amount float64) error {
	penalties := PenaltyMap(comp)
	penalties[pairID] += amount
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
