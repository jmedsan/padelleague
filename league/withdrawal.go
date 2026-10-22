package league

import (
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

// WalkoverScore is the competition's walkover score, falling back to 6-0 6-0 when unset.
func WalkoverScore(comp *core.Record) string {
	if s := comp.GetString("walkover_score"); s != "" {
		return s
	}
	return "6-0 6-0"
}

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

// WithdrawalPlan decides what the withdrawal of pairID does to match m. proposed holds the
// match ids with a standing result proposal.
func WithdrawalPlan(comp, m *core.Record, pairID string, proposed map[string]bool) WithdrawalAction {
	opponent := OpponentOf(m, pairID)
	status := m.GetString("status")
	woScore := WalkoverScore(comp)
	switch {
	case IsPlayoff(comp) && opponent == "":
		return WithdrawalLeave // next round not seeded yet; advanceRound resolves it
	case status == StatusFinal && IsPlayoff(comp):
		return WithdrawalLeave // earlier rounds stand
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
