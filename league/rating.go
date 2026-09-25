package league

import (
	"math"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	ratingK0       = 200
	ratingKMin     = 40
	ratingHalfLife = 4
)

// Level is one skill-level classification a pair can be assigned, mapping
// its DB key to a display label and hidden-Elo starting offset.
type Level struct {
	Key      string
	Label    string
	EloStart float64
}

// Levels is the ordered list of skill-level classifications, weakest first.
// The DB key matches pairs.level; EloStart seeds a pair's hidden rating
// before any match is played.
var Levels = []Level{
	{"beginner", "Principiante", -300},
	{"beginner_high", "Principiante alto", -200},
	{"intermediate_low", "Intermedio bajo", -100},
	{"intermediate", "Intermedio", 0},
	{"intermediate_high", "Intermedio alto", 100},
	{"advanced", "Avanzado", 200},
	{"advanced_high", "Avanzado alto", 300},
	{"unranked", "Sin clasificar", 0},
}

// LevelLabel returns the Spanish display label for a level key, or the key
// itself when unrecognized.
func LevelLabel(key string) string {
	for _, l := range Levels {
		if l.Key == key {
			return l.Label
		}
	}
	return key
}

// LevelElo returns the hidden-Elo starting offset for a level key, or 0
// (the "Intermedio"/unranked baseline) when unrecognized.
func LevelElo(key string) float64 {
	for _, l := range Levels {
		if l.Key == key {
			return l.EloStart
		}
	}
	return 0
}

// LevelLocked reports whether pairID has ever played a match in a leveled
// competition, in which case its level can no longer be changed (its rating
// history already depends on the starting Elo).
func LevelLocked(app core.App, pairID string) bool {
	matches, err := app.FindRecordsByFilter("matches",
		"pair1 = {:pid} || pair2 = {:pid}", "", 0, 0, map[string]any{"pid": pairID})
	if err != nil {
		return false
	}
	seen := make(map[string]struct{})
	for _, m := range matches {
		compID := m.GetString("competition")
		if _, ok := seen[compID]; ok {
			continue
		}
		seen[compID] = struct{}{}
		comp, err := app.FindRecordById("competitions", compID)
		if err != nil {
			continue
		}
		if IsLeveled(comp) {
			return true
		}
	}
	return false
}

// Ratings replays the competition's final matches and returns the hidden rating
// per pair ID. Never stored; recomputed on every call.
func Ratings(app core.App, comp *core.Record) (map[string]float64, error) {
	ratings := make(map[string]float64)
	pairIDs := comp.GetStringSlice("pairs")
	pairRecs, err := app.FindRecordsByIds("pairs", pairIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range pairRecs {
		ratings[p.Id] = LevelElo(p.GetString("level"))
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
