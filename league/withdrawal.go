package league

import (
	"github.com/pocketbase/pocketbase/core"
)

// NoResultEventText is the timeline line for a void match.
const NoResultEventText = "Sin resultado: no hay pareja que pueda jugar este partido"

// WalkoverScore is the score every approved walkover records. It is fixed, not configured.
const WalkoverScore = "6-0 6-0"

// ApplyWalkoverResult records the official walkover on m: the score in pair1/pair2 order, the
// winner, final, and the admin marker, with any arbitration request cleared.
func ApplyWalkoverResult(m *core.Record, winnerID, woScore string) {
	scores := woScore
	if winnerID == m.GetString("pair2") {
		scores = FlipScoreSides(woScore)
	}
	m.Set("scores", scores)
	m.Set("winner", winnerID)
	m.Set("status", StatusFinal)
	m.Set("review_type", "walkover")
	m.Set("admin_result", true)
	m.Set("carried_sets", "")
	m.Set("arbitration", "")
	m.Set("arbitration_by", "")
}

// ApplyVoidResult voids m: no score, no winner, final, and no review type, so stats never count it.
func ApplyVoidResult(m *core.Record) {
	m.Set("scores", "")
	m.Set("winner", "")
	m.Set("status", StatusFinal)
	m.Set("review_type", "")
	m.Set("admin_result", true)
	m.Set("carried_sets", "")
	m.Set("arbitration", "")
	m.Set("arbitration_by", "")
}

// IsVoid reports whether m is final with no winner: no pair could play it (see ApplyVoidResult).
func IsVoid(m *core.Record) bool {
	return m.GetString("status") == StatusFinal && m.GetString("winner") == ""
}

// OpponentOf is the other pair of match m.
func OpponentOf(m *core.Record, pairID string) string {
	if m.GetString("pair1") == pairID {
		return m.GetString("pair2")
	}
	return m.GetString("pair1")
}
