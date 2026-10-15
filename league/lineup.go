package league

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// A match's lineup is the two players of each pair who stood behind the
// result. It is frozen once, when a result is first proposed or a walkover is
// recorded, and never overwritten: a later pair swap changes the pair, not
// the match history of the players who played. Pair stats stay with the pair;
// a player's own stats read the lineup.

// lineupFields are the relation fields holding the frozen players of pair1
// and pair2, in team order.
var lineupFields = [2]string{"lineup1", "lineup2"}

// lineupDue reports whether the match has reached the point where its lineup
// must be on record: a result proposed or confirmed, or a walkover.
func lineupDue(match *core.Record) bool {
	return match.GetString("submitted_by") != "" ||
		match.GetString("status") == StatusFinal ||
		match.GetString("review_type") == "walkover"
}

// lineupFrozen reports whether the match already carries a lineup.
func lineupFrozen(match *core.Record) bool {
	return len(match.GetStringSlice(lineupFields[0])) > 0 || len(match.GetStringSlice(lineupFields[1])) > 0
}

// FreezeLineup records each pair's current players on the match when a
// result exists and no lineup has been recorded yet. It reports whether it
// set the fields; the caller saves the record.
func FreezeLineup(app core.App, match *core.Record) bool {
	if !lineupDue(match) || lineupFrozen(match) {
		return false
	}
	match.Set(lineupFields[0], PlayersForPair(app, match.GetString("pair1")))
	match.Set(lineupFields[1], PlayersForPair(app, match.GetString("pair2")))
	return true
}

// MatchLineup returns the frozen players of the pair's side of the match, or
// nil when no lineup was recorded (a match with no result yet).
func MatchLineup(match *core.Record, pairID string) []string {
	switch PairTeam(match, pairID) {
	case 1:
		return match.GetStringSlice(lineupFields[0])
	case 2:
		return match.GetStringSlice(lineupFields[1])
	}
	return nil
}

// PlayedMatch reports whether the user was in the pair's frozen lineup. A
// match with no lineup counts for the pair's current players, who are the
// only ones that can have played it.
func PlayedMatch(match *core.Record, pairID, userID string) bool {
	lineup := MatchLineup(match, pairID)
	return len(lineup) == 0 || slices.Contains(lineup, userID)
}

// PlayerStatPairIDs returns every pair whose results belong in the player's
// own stats: their current pairs plus the pairs of any match whose frozen
// lineup includes them, so a player keeps the matches they played after
// leaving a pair.
func PlayerStatPairIDs(app core.App, userID string) []string {
	var ids []string
	if pairs, err := PairsForPlayer(app, userID); err == nil {
		for _, p := range pairs {
			ids = append(ids, p.Id)
		}
	}
	played, err := app.FindRecordsByFilter("matches",
		"lineup1 ~ {:u} || lineup2 ~ {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		return ids
	}
	for _, m := range played {
		for i, pairField := range [2]string{"pair1", "pair2"} {
			pid := m.GetString(pairField)
			if slices.Contains(m.GetStringSlice(lineupFields[i]), userID) && !slices.Contains(ids, pid) {
				ids = append(ids, pid)
			}
		}
	}
	return ids
}

// registerLineupFreeze freezes the lineup on every write of a match that has a
// result, and when a result proposal is first posted in the thread.
func registerLineupFreeze(app core.App) {
	freeze := func(e *core.RecordEvent) error {
		FreezeLineup(e.App, e.Record)
		return e.Next()
	}
	app.OnRecordCreate("matches").BindFunc(freeze)
	app.OnRecordUpdate("matches").BindFunc(freeze)
	app.OnRecordCreate("match_messages").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("type") == "result_submission" {
			freezeFromProposal(e.App, e.Record.GetString("match"))
		}
		return e.Next()
	})
}

// freezeFromProposal freezes the lineup of the match a result proposal was
// just posted on, from a fresh copy so no stale field is written back.
func freezeFromProposal(app core.App, matchID string) {
	match, err := app.FindRecordById("matches", matchID)
	if err != nil || lineupFrozen(match) {
		return
	}
	match.Set(lineupFields[0], PlayersForPair(app, match.GetString("pair1")))
	match.Set(lineupFields[1], PlayersForPair(app, match.GetString("pair2")))
	if err := app.Save(match); err != nil {
		app.Logger().Error("lineup: freeze on proposal", "match", matchID, "err", err)
	}
}

// reliabilityScope narrows the reliability inputs to one player's matches.
// The zero value (pair level) includes every match of the pair.
type reliabilityScope struct {
	userID       string
	currentPairs []string
}

// newReliabilityScope scopes reliability to the player's own matches; an
// empty userID is the pair-level scope.
func newReliabilityScope(app core.App, userID string) reliabilityScope {
	if userID == "" {
		return reliabilityScope{}
	}
	var current []string
	if pairs, err := PairsForPlayer(app, userID); err == nil {
		for _, p := range pairs {
			current = append(current, p.Id)
		}
	}
	return reliabilityScope{userID: userID, currentPairs: current}
}

// includes reports whether the match, played for pairID, belongs to the
// scope: a match with a frozen lineup counts for the players in it, and a
// match without one (still open) counts for the pair's current players.
func (r reliabilityScope) includes(match *core.Record, pairID string) bool {
	if r.userID == "" {
		return true
	}
	if lineup := MatchLineup(match, pairID); len(lineup) > 0 {
		return slices.Contains(lineup, r.userID)
	}
	return slices.Contains(r.currentPairs, pairID)
}
