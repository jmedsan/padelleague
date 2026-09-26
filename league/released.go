package league

import (
	"encoding/json"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// releasedPairing is the JSON wire shape stored in a competition's
// released_pairings field — an unordered pair the admin released without a
// result, so TopUpAssignments must never recreate it.
type releasedPairing struct {
	A string `json:"a"`
	B string `json:"b"`
}

// ReleasedPairings returns the pairings an admin has released for comp
// without a result. TopUpAssignments' avoid list only blocks the run that
// fires immediately after a release (see hooks/hooks.go's delete hook); a
// later run (e.g. the daily cron) has no other record of the release, so
// buildLeveledState must fold these into met on every run.
func ReleasedPairings(comp *core.Record) []Pairing {
	stored := decodeReleasedPairings(comp)
	pairings := make([]Pairing, len(stored))
	for i, p := range stored {
		pairings[i] = Pairing{A: p.A, B: p.B}
	}
	return pairings
}

// AppendReleasedPairing records that pair a and pair b were released
// unplayed for comp, then saves comp. Idempotent: calling it twice with the
// same unordered pair does not duplicate the entry.
func AppendReleasedPairing(app core.App, comp *core.Record, a, b string) error {
	stored := decodeReleasedPairings(comp)
	for _, p := range stored {
		if (p.A == a && p.B == b) || (p.A == b && p.B == a) {
			return nil
		}
	}
	stored = append(stored, releasedPairing{A: a, B: b})
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	comp.Set("released_pairings", string(encoded))
	return app.Save(comp)
}

func decodeReleasedPairings(comp *core.Record) []releasedPairing {
	raw := comp.GetString("released_pairings")
	if raw == "" {
		return nil
	}
	var stored []releasedPairing
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		slog.Error("released pairings: unmarshal", "competition", comp.Id, "err", err)
		return nil
	}
	return stored
}
