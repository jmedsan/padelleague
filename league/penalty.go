package league

import (
	"fmt"
	"time"

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

// VoidPenaltyInput is the data recorded when voiding a penalty.
type VoidPenaltyInput struct {
	PenaltyID string
	AdminID   string
	Reason    string
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

// ApplyPenalty creates one penalty row and returns it.
func ApplyPenalty(app core.App, input PenaltyInput) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("penalties")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("competition", input.CompetitionID)
	rec.Set("pair", input.PairID)
	rec.Set("amount", input.Amount)
	rec.Set("reason", input.Reason)
	rec.Set("applied_by", input.AdminID)
	if err := app.Save(rec); err != nil {
		return nil, fmt.Errorf("apply penalty: %w", err)
	}
	return rec, nil
}

// VoidPenalty marks a penalty row voided, retaining its history, and
// returns the voided record.
func VoidPenalty(app core.App, input VoidPenaltyInput) (*core.Record, error) {
	rec, err := app.FindRecordById("penalties", input.PenaltyID)
	if err != nil {
		return nil, err
	}
	rec.Set("voided", true)
	rec.Set("voided_by", input.AdminID)
	rec.Set("voided_at", time.Now())
	rec.Set("void_reason", input.Reason)
	if err := app.Save(rec); err != nil {
		return nil, fmt.Errorf("void penalty: %w", err)
	}
	return rec, nil
}
