package league

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// PlayerTeam returns 1 or 2 indicating which pair the user belongs to in the match.
func PlayerTeam(app core.App, userID string, match *core.Record) (int, error) {
	pair1, err := app.FindRecordById("pairs", match.GetString("pair1"))
	if err != nil {
		return 0, fmt.Errorf("pair1 not found: %w", err)
	}
	if pair1.GetString("player1") == userID || pair1.GetString("player2") == userID {
		return 1, nil
	}
	pair2, err := app.FindRecordById("pairs", match.GetString("pair2"))
	if err != nil {
		return 0, fmt.Errorf("pair2 not found: %w", err)
	}
	if pair2.GetString("player1") == userID || pair2.GetString("player2") == userID {
		return 2, nil
	}
	return 0, fmt.Errorf("user %s is not a participant", userID)
}

// PairNames resolves pair IDs to their display names.
func PairNames(app core.App, pairIDs []string) map[string]string {
	names := make(map[string]string, len(pairIDs))
	for _, id := range pairIDs {
		if id == "" {
			continue
		}
		pair, err := app.FindRecordById("pairs", id)
		if err != nil {
			names[id] = "Pareja desconocida"
			continue
		}
		names[id] = pair.GetString("name")
	}
	return names
}

// EntityURL returns the canonical URL for a known entity kind, or "#" for
// unknown kinds or empty IDs.
func EntityURL(kind, id string) string {
	if id == "" {
		return "#"
	}
	switch kind {
	case "player", "competition", "match", "pair":
		return "/" + kind + "/" + id
	default:
		return "#"
	}
}

// CompetitionName returns the display name for a competition, or "?" if not found.
func CompetitionName(app core.App, compID string) string {
	if compID == "" {
		return "?"
	}
	comp, err := app.FindRecordById("competitions", compID)
	if err != nil {
		return "?"
	}
	return comp.GetString("name")
}

// PlayerName returns the display name for a user, or "?" if not found.
func PlayerName(app core.App, userID string) string {
	if userID == "" {
		return "?"
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return "?"
	}
	return user.GetString("display_name")
}

// PlayerAvatarURL returns the served URL for a user's avatar, or "" if the
// user has none set.
func PlayerAvatarURL(app core.App, userID string) string {
	if userID == "" {
		return ""
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return ""
	}
	return AvatarURL(userID, user.GetString("avatar"))
}

// AvatarURL builds the served URL for a user's avatar file, or "" if filename is empty.
func AvatarURL(userID, filename string) string {
	if filename == "" {
		return ""
	}
	return "/api/files/users/" + userID + "/" + filename
}

// PlayersForPair returns the user IDs of both players in a pair.
func PlayersForPair(app core.App, pairID string) []string {
	pair, err := app.FindRecordById("pairs", pairID)
	if err != nil {
		return nil
	}
	var userIDs []string
	if p1 := pair.GetString("player1"); p1 != "" {
		userIDs = append(userIDs, p1)
	}
	if p2 := pair.GetString("player2"); p2 != "" {
		userIDs = append(userIDs, p2)
	}
	return userIDs
}

// PairsForPlayer returns all pairs that include the given user.
func PairsForPlayer(app core.App, userID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("pairs",
		"player1 = {:uid} || player2 = {:uid}",
		"name", 0, 0,
		map[string]any{"uid": userID})
}

// PrecedentsSummary is the pair-vs-pair head-to-head record across every
// finalized match between two pairs, in any competition.
type PrecedentsSummary struct {
	Pair1ID, Pair2ID     string
	Pair1Wins, Pair2Wins int
	LastMatchID          string
	LastScore            string
}

// Precedents finds all finalized matches between pair1ID and pair2ID across
// every competition (excluding excludeMatchID, the match currently being
// viewed), tallies wins for each pair, and returns the most recent meeting's
// score. ok is false when the pairs have never played each other before.
func Precedents(app core.App, pair1ID, pair2ID, excludeMatchID string) (summary PrecedentsSummary, ok bool) {
	matches, err := app.FindRecordsByFilter("matches",
		"status = 'final' && ((pair1 = {:p1} && pair2 = {:p2}) || (pair1 = {:p2} && pair2 = {:p1})) && id != {:exclude}",
		"-date,-created", 0, 0,
		map[string]any{"p1": pair1ID, "p2": pair2ID, "exclude": excludeMatchID})
	if err != nil || len(matches) == 0 {
		return PrecedentsSummary{}, false
	}

	summary = PrecedentsSummary{Pair1ID: pair1ID, Pair2ID: pair2ID}
	for _, m := range matches {
		switch m.GetString("winner") {
		case pair1ID:
			summary.Pair1Wins++
		case pair2ID:
			summary.Pair2Wins++
		}
	}

	last := matches[0]
	summary.LastMatchID = last.Id
	summary.LastScore = last.GetString("scores")
	if last.GetString("pair1") == pair2ID {
		// Normalize the last score to pair1/pair2 order regardless of which
		// side each pair was on in that older match.
		summary.LastScore = flipScoreSides(summary.LastScore)
	}

	return summary, true
}

// flipScoreSides swaps a "S1-S2[(tb)] S1-S2[(tb)] ..." score string to
// pair2/pair1 order, so a past match's score can be displayed in the current
// match's pair1/pair2 order. The optional "(N)" tiebreak annotation stays
// attached to the set it was recorded on.
func flipScoreSides(score string) string {
	if strings.EqualFold(strings.TrimSpace(score), "WO") {
		return score
	}
	sets := strings.Fields(score)
	flipped := make([]string, len(sets))
	for i, set := range sets {
		m := scoreSetRe.FindStringSubmatch(set)
		if m == nil {
			flipped[i] = set
			continue
		}
		g1, g2, tb := m[1], m[2], m[3]
		flipped[i] = g2 + "-" + g1 + tb
	}
	return strings.Join(flipped, " ")
}

var scoreSetRe = regexp.MustCompile(`^(\d+)-(\d+)(\(\d+\))?$`)

// Truncate shortens s to max runes, appending "..." if truncated.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
