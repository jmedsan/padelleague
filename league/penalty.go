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

// ApplyPendingMatchPenalties reconciles auto-penalties for unplayed matches.
//
// Target per pair is derived from the competition phase:
//   - Recovery: max(0, pendingCount - threshold)
//   - Finished: pendingCount (every unplayed match)
//
// Auto-penalties are identified by applied_by = "" (cron has no author).
// Existing auto-penalties (active + voided) are counted per pair; new ones
// are created only when target exceeds that total. Voided auto-penalties are
// never re-created, so an admin void permanently forgives that slot.
//
// A threshold of 0 disables auto-penalties for this competition.
// Returns the newly created penalty records.
func ApplyPendingMatchPenalties(app core.App, comp *core.Record) ([]*core.Record, error) {
	threshold := comp.GetInt("max_pending_matches")
	if threshold == 0 {
		return nil, nil // disabled
	}

	phase := CompetitionPhase(comp, time.Now())

	pairIDs := comp.GetStringSlice("pairs")

	// Count unplayed matches per pair (status != 'final' = pending/scheduled/confirmed/disputed).
	unplayed, err := app.FindRecordsByFilter("matches",
		"competition = {:c} && status != 'final'",
		"", 0, 0, map[string]any{"c": comp.Id})
	if err != nil {
		return nil, fmt.Errorf("pending match penalties: list matches: %w", err)
	}
	counts := make(map[string]int, len(pairIDs))
	for _, m := range unplayed {
		counts[m.GetString("pair1")]++
		counts[m.GetString("pair2")]++
	}

	// Count total auto-penalties (active + voided) per pair.
	// Cron-created penalties have applied_by = "" (no user author).
	allPenalties, err := app.FindRecordsByFilter("penalties",
		"competition = {:c}",
		"", 0, 0, map[string]any{"c": comp.Id})
	if err != nil {
		return nil, fmt.Errorf("pending match penalties: list existing: %w", err)
	}
	autoTotal := make(map[string]int, len(pairIDs))
	for _, p := range allPenalties {
		if p.GetString("applied_by") == "" {
			autoTotal[p.GetString("pair")]++
		}
	}

	var applied []*core.Record
	for _, pairID := range pairIDs {
		target := targetPenaltyCount(counts[pairID], threshold, phase)
		toCreate := target - autoTotal[pairID]
		if toCreate <= 0 {
			continue
		}
		reason := fmt.Sprintf("Partidos pendientes por encima del límite (%d pendientes, máximo %d)",
			counts[pairID], threshold)
		if phase == PhaseFinished {
			reason = fmt.Sprintf("Partido pendiente al cierre de la competición (%d pendientes)",
				counts[pairID])
		}
		for range toCreate {
			rec, err := ApplyPenalty(app, PenaltyInput{
				CompetitionID: comp.Id,
				PairID:        pairID,
				Amount:        1,
				Reason:        reason,
			})
			if err != nil {
				return applied, fmt.Errorf("pending match penalties: pair %s: %w", pairID, err)
			}
			applied = append(applied, rec)
		}
	}
	return applied, nil
}

// targetPenaltyCount returns how many auto-penalties a pair should have
// given their unplayed count, threshold, and competition phase.
func targetPenaltyCount(pendingCount, threshold int, phase Phase) int {
	switch phase {
	case PhaseFinished:
		return pendingCount
	case PhaseRecovery:
		excess := pendingCount - threshold
		if excess < 0 {
			return 0
		}
		return excess
	default:
		return 0
	}
}
