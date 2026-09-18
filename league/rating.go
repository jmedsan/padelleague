package league

import (
	"math"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	ratingSeedStep = 40
	ratingK0       = 200
	ratingKMin     = 40
	ratingHalfLife = 4
)

// Ratings replays the competition's final matches and returns the hidden rating
// per pair ID. Never stored; recomputed on every call.
func Ratings(app core.App, comp *core.Record) (map[string]float64, error) {
	ratings := make(map[string]float64)
	seedIDs := comp.GetStringSlice("seed_pairs")
	n := len(seedIDs)
	for i, id := range seedIDs {
		ratings[id] = (float64(n-1)/2 - float64(i)) * ratingSeedStep
	}

	matches, err := app.FindRecordsByFilter(
		"matches",
		"competition = {:comp} && status = 'final'",
		"finalized_at, id",
		0, 0,
		map[string]any{"comp": comp.Id},
	)
	if err != nil {
		return nil, err
	}

	played := make(map[string]int)
	for _, m := range matches {
		applyMatchRating(m, ratings, played)
	}
	return ratings, nil
}

// applyMatchRating updates ratings and played counts for one final match.
func applyMatchRating(m *core.Record, ratings map[string]float64, played map[string]int) {
	scores := strings.TrimSpace(m.GetString("scores"))
	if strings.EqualFold(scores, "WO") || m.GetString("review_type") == "walkover" {
		return
	}
	sc, err := ParseScore(scores)
	if err != nil {
		return
	}
	totalSets := sc.Sets1 + sc.Sets2
	if totalSets == 0 {
		return
	}
	p1, p2 := m.GetString("pair1"), m.GetString("pair2")
	sA := float64(sc.Sets1) / float64(totalSets)
	eA := 1.0 / (1.0 + math.Pow(10, (ratings[p2]-ratings[p1])/400))
	delta := sA - eA
	ratings[p1] += kFactor(played[p1]) * delta
	ratings[p2] -= kFactor(played[p2]) * delta
	played[p1]++
	played[p2]++
}

// kFactor returns the K value for a pair that has played n rated matches.
func kFactor(n int) float64 {
	k := ratingK0 * float64(ratingHalfLife) / float64(ratingHalfLife+n)
	if k < ratingKMin {
		return ratingKMin
	}
	return k
}
