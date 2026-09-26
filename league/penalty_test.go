package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

// setRecoveryPhase puts a competition in recovery phase (end_date yesterday, 30 recovery days).
func setRecoveryPhase(t *testing.T, app core.App, comp *core.Record) {
	t.Helper()
	comp.Set("end_date", time.Now().AddDate(0, 0, -1))
	comp.Set("recovery_days", 30)
	require.NoError(t, app.Save(comp))
}

// setFinishedPhase puts a competition in finished phase (finalized=true).
func setFinishedPhase(t *testing.T, app core.App, comp *core.Record) {
	t.Helper()
	comp.Set("finalized", true)
	require.NoError(t, app.Save(comp))
}

// pendingCounts counts unplayed matches per pair in the test — mirrors the function's logic.
// Each match contributes to both pair1 and pair2.
// We use a dedicated 4-pair competition where match assignments are explicit:
//
//	pA: appears in 1 unplayed match  → no penalty
//	pB: appears in 2 unplayed matches → no penalty (at threshold)
//	pC: appears in 3 unplayed matches → penalty -1
//	pD: appears in 5 unplayed matches → penalty -3
//
// To isolate counts, we use distinct pairs that only share matches with their
// "target" pair — no match appears in two targets' counts.
func TestApplyPendingMatchPenalties(t *testing.T) {
	app := newTestApp(t)

	// Use separate "opponent" pairs for each target so counts don't bleed.
	pA := makePair(t, app, "Pair A") // 1 unplayed
	pB := makePair(t, app, "Pair B") // 2 unplayed
	pC := makePair(t, app, "Pair C") // 3 unplayed
	pD := makePair(t, app, "Pair D") // 5 unplayed

	// Opponent pairs used only to create matches without inflating A/B/C/D counts.
	oppA := makePair(t, app, "Opp A1")
	oppB1 := makePair(t, app, "Opp B1")
	oppB2 := makePair(t, app, "Opp B2")
	oppC1 := makePair(t, app, "Opp C1")
	oppC2 := makePair(t, app, "Opp C2")
	oppC3 := makePair(t, app, "Opp C3")
	oppD1 := makePair(t, app, "Opp D1")
	oppD2 := makePair(t, app, "Opp D2")
	oppD3 := makePair(t, app, "Opp D3")
	oppD4 := makePair(t, app, "Opp D4")
	oppD5 := makePair(t, app, "Opp D5")

	allPairs := []*core.Record{pA, pB, pC, pD,
		oppA, oppB1, oppB2, oppC1, oppC2, oppC3,
		oppD1, oppD2, oppD3, oppD4, oppD5}
	comp := makeCompetition(t, app, allPairs)
	comp.Set("max_pending_matches", 2)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	// pA: 1 unplayed
	makeMatch(t, app, comp.Id, pA.Id, oppA.Id, "pending")

	// pB: 2 unplayed (at threshold — no penalty)
	makeMatch(t, app, comp.Id, pB.Id, oppB1.Id, "pending")
	makeMatch(t, app, comp.Id, pB.Id, oppB2.Id, "scheduled")

	// pC: 3 unplayed (-1 excess)
	makeMatch(t, app, comp.Id, pC.Id, oppC1.Id, "pending")
	makeMatch(t, app, comp.Id, pC.Id, oppC2.Id, "scheduled")
	makeMatch(t, app, comp.Id, pC.Id, oppC3.Id, "confirmed")

	// pD: 5 unplayed (-3 excess)
	makeMatch(t, app, comp.Id, pD.Id, oppD1.Id, "pending")
	makeMatch(t, app, comp.Id, pD.Id, oppD2.Id, "pending")
	makeMatch(t, app, comp.Id, pD.Id, oppD3.Id, "scheduled")
	makeMatch(t, app, comp.Id, pD.Id, oppD4.Id, "confirmed")
	makeMatch(t, app, comp.Id, pD.Id, oppD5.Id, "pending")

	applied, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	// pC: 1 excess → 1 record; pD: 3 excess → 3 records = 4 total
	require.Len(t, applied, 4, "C (1 excess) and D (3 excess) should get 4 penalty records")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 0.0, totals[pA.Id], "A: 1 pending, no penalty")
	require.Equal(t, 0.0, totals[pB.Id], "B: 2 pending, no penalty")
	require.Equal(t, 1.0, totals[pC.Id], "C: 3 pending, -1")
	require.Equal(t, 3.0, totals[pD.Id], "D: 5 pending, -3")
}

func TestApplyPendingMatchPenalties_Idempotent(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "Pair A")
	opp1 := makePair(t, app, "Opp 1")
	opp2 := makePair(t, app, "Opp 2")
	opp3 := makePair(t, app, "Opp 3")

	comp := makeCompetition(t, app, []*core.Record{pA, opp1, opp2, opp3})
	comp.Set("max_pending_matches", 2)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	// pA has 3 unplayed matches → 1 excess
	makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "scheduled")
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "confirmed")

	n1, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, n1, 1)

	n2, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Empty(t, n2, "second run must not create new penalties")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 1.0, totals[pA.Id], "penalty applied exactly once")
}

func TestApplyPendingMatchPenalties_DisabledWhenThresholdZero(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "Pair A")
	opp := makePair(t, app, "Opp")

	comp := makeCompetition(t, app, []*core.Record{pA, opp})
	comp.Set("max_pending_matches", 0)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")

	n, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Empty(t, n, "threshold=0 means disabled")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 0.0, totals[pA.Id])
}

// TestApplyPendingMatchPenalties_RecoveryPhase checks that the recovery-phase
// rule (threshold applies, each excess = 1 penalty) fires correctly.
// 4 pending matches, threshold 2 → 2 new penalties.
func TestApplyPendingMatchPenalties_RecoveryPhase(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "RecPairA")
	opp1 := makePair(t, app, "RecOpp1")
	opp2 := makePair(t, app, "RecOpp2")
	opp3 := makePair(t, app, "RecOpp3")
	opp4 := makePair(t, app, "RecOpp4")

	comp := makeCompetition(t, app, []*core.Record{pA, opp1, opp2, opp3, opp4})
	comp.Set("max_pending_matches", 2)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp4.Id, "pending")

	applied, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, applied, 2, "4 pending − threshold 2 = 2 penalties")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 2.0, totals[pA.Id])
}

// TestApplyPendingMatchPenalties_RecoveryToFinished verifies state-based growth:
// after recovery run (2 penalties for 4 pending), transition to finished phase
// (target = 3 remaining pending) creates 1 more penalty (total 3).
func TestApplyPendingMatchPenalties_RecoveryToFinished(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "TransPairA")
	opp1 := makePair(t, app, "TransOpp1")
	opp2 := makePair(t, app, "TransOpp2")
	opp3 := makePair(t, app, "TransOpp3")
	opp4 := makePair(t, app, "TransOpp4")

	comp := makeCompetition(t, app, []*core.Record{pA, opp1, opp2, opp3, opp4})
	comp.Set("max_pending_matches", 2)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	// 4 pending → 2 excess during recovery.
	m1 := makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp4.Id, "pending")

	r1, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, r1, 2)

	// One match gets played; 3 remain pending.
	m1.Set("status", "final")
	require.NoError(t, app.Save(m1))

	// Transition to finished phase; target becomes pendingCount = 3.
	// Already have 2 auto-penalties → create 1 more.
	setFinishedPhase(t, app, comp)
	comp, err = app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)

	r2, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	// Count only new penalties for pA (opponents also get penalized in finished phase).
	newForPA := 0
	for _, rec := range r2 {
		if rec.GetString("pair") == pA.Id {
			newForPA++
		}
	}
	require.Equal(t, 1, newForPA, "finished phase should add 1 more penalty for pA (3 pending, 2 already applied)")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 3.0, totals[pA.Id])
}

// TestApplyPendingMatchPenalties_VoidedNotRecreated verifies that an
// admin-voided auto-penalty is not re-created on the next cron run.
func TestApplyPendingMatchPenalties_VoidedNotRecreated(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "VoidPairA")
	opp1 := makePair(t, app, "VoidOpp1")
	opp2 := makePair(t, app, "VoidOpp2")
	opp3 := makePair(t, app, "VoidOpp3")

	comp := makeCompetition(t, app, []*core.Record{pA, opp1, opp2, opp3})
	comp.Set("max_pending_matches", 2)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	// 3 pending → 1 excess penalty.
	makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "pending")

	r1, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, r1, 1)

	// Admin voids the auto-penalty.
	_, err = VoidPenalty(app, VoidPenaltyInput{
		PenaltyID: r1[0].Id,
		AdminID:   "",
		Reason:    "test void",
	})
	require.NoError(t, err)

	// Next cron run: target still 1, but 1 voided auto-penalty counts as applied → no new penalty.
	r2, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Empty(t, r2, "voided auto-penalty must not be re-created")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 0.0, totals[pA.Id], "voided penalty leaves zero active penalty points")
}

// TestApplyPendingMatchPenalties_GrowingCount verifies that when pending count
// grows (2→3 during recovery with threshold 1), one new penalty is created.
func TestApplyPendingMatchPenalties_GrowingCount(t *testing.T) {
	app := newTestApp(t)

	pA := makePair(t, app, "GrowPairA")
	opp1 := makePair(t, app, "GrowOpp1")
	opp2 := makePair(t, app, "GrowOpp2")
	opp3 := makePair(t, app, "GrowOpp3")

	comp := makeCompetition(t, app, []*core.Record{pA, opp1, opp2, opp3})
	comp.Set("max_pending_matches", 1)
	require.NoError(t, app.Save(comp))
	setRecoveryPhase(t, app, comp)

	// 2 pending → 1 excess.
	makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "pending")

	r1, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, r1, 1)

	// A third match is created (count grows to 3 → 2 excess; 1 already applied → 1 new).
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "pending")

	r2, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Len(t, r2, 1, "count grew from 2→3 pending, one new penalty expected")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 2.0, totals[pA.Id])
}

// TestApplyPendingMatchPenalties_LeveledCloseProjected: at the close of a
// leveled league the penalty counts target − played, assigned or not, and a
// withdrawn pair counts nothing.
func TestApplyPendingMatchPenalties_LeveledCloseProjected(t *testing.T) {
	app := newTestApp(t)
	pairs := make([]*core.Record, 6)
	for i := range pairs {
		pairs[i] = makePair(t, app, "Close")
	}
	comp := makeLeveledCompetition(t, app, pairs, 4, 2)
	comp.Set("max_pending_matches", 2)
	comp.Set("withdrawn_pairs", []string{pairs[5].Id})
	require.NoError(t, app.Save(comp))
	// pairs[0] played one match (with pairs[1]) and has one assigned but unplayed.
	makeMatch(t, app, comp.Id, pairs[0].Id, pairs[1].Id, "final")
	makeMatch(t, app, comp.Id, pairs[0].Id, pairs[2].Id, "pending")
	setFinishedPhase(t, app, comp)
	comp, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)

	applied, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	perPair := map[string]int{}
	reasons := map[string]string{}
	for _, r := range applied {
		perPair[r.GetString("pair")]++
		reasons[r.GetString("pair")] = r.GetString("reason")
	}
	require.Equal(t, "Partido no disputado al cierre de la competición (3 sin jugar)", reasons[pairs[0].Id])
	require.Equal(t, "Partido no disputado al cierre de la competición (4 sin jugar)", reasons[pairs[3].Id])
	require.Equal(t, 3, perPair[pairs[0].Id], "played 1 of 4")
	require.Equal(t, 3, perPair[pairs[1].Id], "played 1 of 4, nothing assigned")
	require.Equal(t, 4, perPair[pairs[2].Id], "one assigned but unplayed still counts as 4 short")
	require.Equal(t, 4, perPair[pairs[3].Id], "never assigned: 4 short")
	require.Equal(t, 0, perPair[pairs[5].Id], "withdrawn pair is not penalized")
}

// TestAutoCloseCompetition closes only once the recovery window has ended,
// logs the event as system-originated, and is idempotent.
func TestAutoCloseCompetition(t *testing.T) {
	app := newTestApp(t)
	pairs := []*core.Record{makePair(t, app, "AC1"), makePair(t, app, "AC2")}
	comp := makeCompetition(t, app, pairs)
	comp.Set("end_date", time.Now().AddDate(0, 0, -3))
	comp.Set("recovery_days", 7)
	require.NoError(t, app.Save(comp))

	closed, err := AutoCloseCompetition(app, comp, time.Now())
	require.NoError(t, err)
	require.False(t, closed, "still inside the extra week")

	closed, err = AutoCloseCompetition(app, comp, time.Now().AddDate(0, 0, 5))
	require.NoError(t, err)
	require.True(t, closed)
	fresh, err := app.FindRecordById("competitions", comp.Id)
	require.NoError(t, err)
	require.True(t, fresh.GetBool("finalized"))
	events, err := app.FindRecordsByFilter("competition_events",
		"competition = {:c} && kind = 'finalized'", "", 0, 0, map[string]any{"c": comp.Id})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "", events[0].GetString("actor"), "system-originated")

	closed, err = AutoCloseCompetition(app, fresh, time.Now().AddDate(0, 0, 5))
	require.NoError(t, err)
	require.False(t, closed, "already closed")
}
