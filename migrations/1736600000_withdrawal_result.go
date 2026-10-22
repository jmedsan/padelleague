package migrations

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// setScoreRe matches one set of a stored score, e.g. "6-2" or "7-6(5)".
var setScoreRe = regexp.MustCompile(`^(\d+)-(\d+)(\(\d+\))?$`)

// adminResultPrefixes are the timeline details the four admin result writers
// produce: a score set or corrected, a walkover approved, a dispute resolved.
var adminResultPrefixes = []string{
	"Resultado establecido",
	"Resultado corregido",
	"aprobó incomparecencia",
	"resolvió la disputa",
}

// Adds matches.rating_scores (the real score of a match whose result was
// overwritten by a withdrawal) and matches.admin_result (the final result was
// set by an admin, not produced by the players). Backfills admin_result from the
// existing rows, and re-orients legacy walkover rows the old code stored from
// pair1's side.
func init() {
	m.Register(func(app core.App) error {
		if err := addWithdrawalResultFields(app); err != nil {
			return err
		}
		if err := backfillAdminResult(app); err != nil {
			return err
		}
		return reorientLegacyWalkovers(app)
	}, func(app core.App) error {
		return removeWithdrawalResultFields(app)
	})
}

func addWithdrawalResultFields(app core.App) error {
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	matches.Fields.Add(
		&core.TextField{Name: "rating_scores"},
		&core.BoolField{Name: "admin_result"},
	)
	return app.Save(matches)
}

func removeWithdrawalResultFields(app core.App) error {
	matches, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	matches.Fields.RemoveByName("rating_scores")
	matches.Fields.RemoveByName("admin_result")
	return app.Save(matches)
}

// backfillAdminResult marks each final match whose result an admin set: every
// approved walkover, and every match whose latest admin result entry is not
// followed by a player's submission or response.
func backfillAdminResult(app core.App) error {
	matches, err := app.FindRecordsByFilter("matches", "status = 'final'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, match := range matches {
		adminSet := match.GetString("review_type") == "walkover"
		if !adminSet {
			if adminSet, err = adminSetResult(app, match.Id); err != nil {
				return err
			}
		}
		if !adminSet {
			continue
		}
		match.Set("admin_result", true)
		if err := app.SaveNoValidate(match); err != nil {
			return err
		}
	}
	return nil
}

// adminSetResult walks the match's result entries newest first. A player entry
// reached before any admin entry means the result was superseded by the players.
func adminSetResult(app core.App, matchID string) (bool, error) {
	entries, err := app.FindRecordsByFilter("match_messages", "match = {:m}", "-created", 0, 0,
		dbx.Params{"m": matchID})
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		switch e.GetString("type") {
		case "result_submission", "result_response":
			return false, nil
		case "admin_action", "result_event":
			if hasAdminResultPrefix(e.GetString("content")) {
				return true, nil
			}
		}
	}
	return false, nil
}

func hasAdminResultPrefix(content string) bool {
	for _, prefix := range adminResultPrefixes {
		if strings.HasPrefix(content, prefix) {
			return true
		}
	}
	return false
}

// reorientLegacyWalkovers flips the score of an approved walkover won by pair2
// when it is stored from pair1's side. A WO score has no sets and is skipped.
// Running it again changes nothing.
func reorientLegacyWalkovers(app core.App) error {
	matches, err := app.FindRecordsByFilter("matches",
		"status = 'final' && review_type = 'walkover'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, match := range matches {
		score := match.GetString("scores")
		if match.GetString("winner") != match.GetString("pair2") || !pair1WinsMoreSets(score) {
			continue
		}
		match.Set("scores", flipSides(score))
		if err := app.SaveNoValidate(match); err != nil {
			return err
		}
	}
	return nil
}

// pair1WinsMoreSets reports whether pair1 has won more sets of the score than pair2.
func pair1WinsMoreSets(score string) bool {
	sets1, sets2 := 0, 0
	for _, set := range strings.Fields(score) {
		g := setScoreRe.FindStringSubmatch(set)
		if g == nil {
			continue
		}
		g1, _ := strconv.Atoi(g[1])
		g2, _ := strconv.Atoi(g[2])
		switch {
		case g1 > g2:
			sets1++
		case g2 > g1:
			sets2++
		}
	}
	return sets1 > sets2
}

// flipSides swaps the two sides of every set, keeping any tiebreak annotation.
func flipSides(score string) string {
	sets := strings.Fields(score)
	for i, set := range sets {
		if g := setScoreRe.FindStringSubmatch(set); g != nil {
			sets[i] = g[2] + "-" + g[1] + g[3]
		}
	}
	return strings.Join(sets, " ")
}
