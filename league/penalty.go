package league

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// PenaltyInput is the data recorded for one penalty.
type PenaltyInput struct {
	CompetitionID string
	PairID        string
	Reason        string
	AdminID       string
	Amount        float64
}

// PenaltyTotals returns the active (non-voided) penalty sum per pair for a
// competition.
func PenaltyTotals(app core.App, competitionID string) (map[string]float64, error) {
	rows, err := app.FindRecordsByFilter("penalties",
		"competition = {:c} && voided = false", "", 0, 0,
		map[string]any{"c": competitionID})
	if err != nil {
		return nil, err
	}
	totals := make(map[string]float64, len(rows))
	for _, r := range rows {
		totals[r.GetString("pair")] += r.GetFloat("amount")
	}
	return totals, nil
}

// ApplyPenalty creates one penalty row.
func ApplyPenalty(app core.App, input PenaltyInput) error {
	col, err := app.FindCollectionByNameOrId("penalties")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("competition", input.CompetitionID)
	rec.Set("pair", input.PairID)
	rec.Set("amount", input.Amount)
	rec.Set("reason", input.Reason)
	rec.Set("applied_by", input.AdminID)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("apply penalty: %w", err)
	}
	return nil
}

// VoidPenalty marks a penalty row voided, retaining its history.
func VoidPenalty(app core.App, penaltyID string) error {
	rec, err := app.FindRecordById("penalties", penaltyID)
	if err != nil {
		return err
	}
	rec.Set("voided", true)
	return app.Save(rec)
}
