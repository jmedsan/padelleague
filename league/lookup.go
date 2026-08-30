package league

import (
	"fmt"

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
		"", 0, 0,
		map[string]any{"uid": userID})
}

// Truncate shortens s to max runes, appending "..." if truncated.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
