package league

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// WithdrawalAction is what a withdrawal does to one match.
type WithdrawalAction int

// Leave keeps the match as it is, Release deletes a leveled match not yet proposed, and Official
// records the walkover score.
const (
	WithdrawalLeave WithdrawalAction = iota
	WithdrawalRelease
	WithdrawalOfficial
)

// NoResultEventText is the timeline line for a void match.
const NoResultEventText = "Sin resultado: no hay pareja que pueda jugar este partido"

// WithdrawalEventText is the result_event text a withdrawal writes on each match it resolves.
func WithdrawalEventText(pairName string) string {
	return fmt.Sprintf("Incomparecencia — %s se ha retirado de la competición", pairName)
}

// WalkoverScore is the score every approved walkover records. It is fixed, not configured.
const WalkoverScore = "6-0 6-0"

// ApplyWalkoverResult records the official walkover on m: the score in pair1/pair2 order, the
// winner, final, and the admin marker, with any arbitration request cleared.
func ApplyWalkoverResult(m *core.Record, winnerID, woScore string) {
	m.Set("scores", OfficialScore(m, winnerID, woScore))
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

// WithdrawalPlan decides what the withdrawal of pairID does to match m. proposed holds the
// match ids with a standing result proposal.
func WithdrawalPlan(comp, m *core.Record, pairID string, proposed map[string]bool) WithdrawalAction {
	opponent := OpponentOf(m, pairID)
	status := m.GetString("status")
	woScore := WalkoverScore
	switch {
	case IsPlayoff(comp) && opponent == "":
		return WithdrawalLeave // next round not seeded yet; advanceRound resolves it
	case status == StatusFinal && WithdrawnSet(comp)[opponent]:
		return WithdrawalLeave // both pairs withdrew; the match stands as the first withdrawal set it
	case status == StatusFinal && IsPlayoff(comp):
		return WithdrawalLeave // a playoff match that is final stands, a played or walkover one alike
	case status == StatusFinal && m.GetBool("admin_result") &&
		m.GetString("winner") == opponent && isOfficialScore(m, woScore):
		return WithdrawalLeave // an admin already set exactly this
	case IsLeveled(comp) && IsPreScore(status) && !proposed[m.Id]:
		return WithdrawalRelease
	default:
		return WithdrawalOfficial
	}
}

// OpponentOf is the other pair of match m.
func OpponentOf(m *core.Record, pairID string) string {
	if m.GetString("pair1") == pairID {
		return m.GetString("pair2")
	}
	return m.GetString("pair1")
}

// OfficialScore is woScore in pair1/pair2 order, so the winner holds the sets.
func OfficialScore(m *core.Record, winnerID, woScore string) string {
	if winnerID == m.GetString("pair2") {
		return FlipScoreSides(woScore)
	}
	return woScore
}

// isOfficialScore reports whether m's stored score is woScore in either order.
// Walkovers stored before the orientation fix kept woScore unflipped when pair 2 won.
func isOfficialScore(m *core.Record, woScore string) bool {
	s := m.GetString("scores")
	return s == woScore || s == FlipScoreSides(woScore)
}

// RealScore is the score the pair actually played, or "" when none stands: a walkover, a
// deadlock, a disputed match or an unplayed one.
func RealScore(app core.App, m *core.Record) (string, error) {
	if m.GetString("status") == StatusFinal {
		if m.GetString("review_type") == "walkover" {
			return "", nil
		}
		return m.GetString("scores"), nil
	}
	synth, err := ProvisionalResults(app, []*core.Record{m})
	if err != nil || len(synth) == 0 {
		return "", err
	}
	return synth[0].GetString("scores"), nil
}
