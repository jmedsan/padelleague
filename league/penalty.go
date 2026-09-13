package league

import (
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// pendingMatchPenaltyPrefix is the reason prefix used by auto-applied pending
// match penalties. It is also used for idempotency: if any non-voided penalty
// with this prefix already exists for a pair in a competition, no new
// auto-penalty is created.
const pendingMatchPenaltyPrefix = "Partidos pendientes"

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

// ApplyPendingMatchPenalties auto-applies -1 point per unplayed match above
// the competition's max_pending_matches threshold. Unplayed = status in
// (pending, scheduled, confirmed). Idempotent: pairs that already have a
// non-voided auto-penalty (reason prefix "Partidos pendientes") are skipped.
// A threshold of 0 disables auto-penalties for this competition.
// Returns the number of penalty records created.
func ApplyPendingMatchPenalties(app core.App, comp *core.Record) (int, error) {
	threshold := comp.GetInt("max_pending_matches")
	if threshold == 0 {
		return 0, nil // disabled
	}

	pairIDs := comp.GetStringSlice("pairs")

	// Count unplayed matches per pair.
	unplayed, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status != 'final'",
		"", 0, 0, map[string]any{"c": comp.Id})
	if err != nil {
		return 0, fmt.Errorf("pending match penalties: list matches: %w", err)
	}

	counts := make(map[string]int, len(pairIDs))
	for _, m := range unplayed {
		p1, p2 := m.GetString("pair1"), m.GetString("pair2")
		counts[p1]++
		counts[p2]++
	}

	// Find existing auto-penalties to enforce idempotency.
	existing, err := app.FindRecordsByFilter("penalties",
		"competition = {:c} && voided = false",
		"", 0, 0, map[string]any{"c": comp.Id})
	if err != nil {
		return 0, fmt.Errorf("pending match penalties: list existing: %w", err)
	}
	alreadyPenalized := make(map[string]bool, len(pairIDs))
	for _, p := range existing {
		if strings.HasPrefix(p.GetString("reason"), pendingMatchPenaltyPrefix) {
			alreadyPenalized[p.GetString("pair")] = true
		}
	}

	applied := 0
	for _, pairID := range pairIDs {
		excess := counts[pairID] - threshold
		if excess <= 0 || alreadyPenalized[pairID] {
			continue
		}
		reason := fmt.Sprintf("%s por encima del límite (%d pendientes, máximo %d)",
			pendingMatchPenaltyPrefix, counts[pairID], threshold)
		if _, err := ApplyPenalty(app, PenaltyInput{
			CompetitionID: comp.Id,
			PairID:        pairID,
			Amount:        float64(excess),
			Reason:        reason,
		}); err != nil {
			return applied, fmt.Errorf("pending match penalties: pair %s: %w", pairID, err)
		}
		applied++
	}
	return applied, nil
}
