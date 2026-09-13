package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

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
	require.Equal(t, 2, applied, "C and D should get penalties")

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

	// pA has 3 unplayed matches → 1 excess
	makeMatch(t, app, comp.Id, pA.Id, opp1.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp2.Id, "scheduled")
	makeMatch(t, app, comp.Id, pA.Id, opp3.Id, "confirmed")

	n1, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Equal(t, 1, n1)

	n2, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Equal(t, 0, n2, "second run must not create new penalties")

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

	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")
	makeMatch(t, app, comp.Id, pA.Id, opp.Id, "pending")

	n, err := ApplyPendingMatchPenalties(app, comp)
	require.NoError(t, err)
	require.Equal(t, 0, n, "threshold=0 means disabled")

	totals, err := PenaltyTotals(app, comp.Id)
	require.NoError(t, err)
	require.Equal(t, 0.0, totals[pA.Id])
}
