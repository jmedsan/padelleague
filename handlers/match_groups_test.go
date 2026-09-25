package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// newMatchCard builds a minimal MatchCard from a match record for group tests.
func newLeveledMatchCard(t *testing.T, _ core.App, m *core.Record) MatchCard {
	t.Helper()
	return NewMatchRow(m, map[string]string{}, map[string]struct{}{})
}

// makeRawMatch inserts a match record and optionally sets arrange_by /
// finalized_at / status.
func makeRawMatch(t *testing.T, app core.App, compID, p1, p2, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	m := core.NewRecord(col)
	m.Set("competition", compID)
	m.Set("pair1", p1)
	m.Set("pair2", p2)
	m.Set("status", status)
	m.Set("round_number", 0)
	require.NoError(t, app.Save(m))
	return m
}

func setArrangeBy(t *testing.T, app core.App, m *core.Record, d time.Time) {
	t.Helper()
	m.Set("arrange_by", d.Format("2006-01-02"))
	require.NoError(t, app.Save(m))
}

func setFinalizedAt(t *testing.T, app core.App, m *core.Record, ts time.Time) {
	t.Helper()
	m.Set("finalized_at", ts.UTC().Format("2006-01-02 15:04:05.000Z"))
	require.NoError(t, app.Save(m))
}

func setSlot(t *testing.T, app core.App, m *core.Record, slot int) {
	t.Helper()
	m.Set("slot", slot)
	require.NoError(t, app.Save(m))
}

func newTestAppForGroups(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

// TestLeveledGroups_DeadlineOrder verifies pending matches are sorted by
// arrange_by ascending, with empty deadlines last.
func TestLeveledGroups_DeadlineOrder(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "DLA")
	p2 := makePairTB(t, app, "DLB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	m1 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m3 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending") // no deadline
	setSlot(t, app, m1, 1)
	setSlot(t, app, m2, 1)
	setSlot(t, app, m3, 1)

	setArrangeBy(t, app, m1, now.Add(5*24*time.Hour)) // later
	setArrangeBy(t, app, m2, now)                     // earliest

	cards := []MatchCard{
		newLeveledMatchCard(t, app, m1),
		newLeveledMatchCard(t, app, m2),
		newLeveledMatchCard(t, app, m3),
	}

	tz := time.UTC
	groups := leveledGroups(cards, tz, leveledWindow{target: 3})
	require.Len(t, groups, 1, "only jornada-1 group")
	jornada := groups[0]
	assert.Equal(t, "jornada-1", jornada.Key)
	require.Len(t, jornada.Matches, 3)
	// m2 (earliest) first, m1 (later) second, m3 (no deadline) last
	assert.Equal(t, m2.Id, jornada.Matches[0].Match.Id)
	assert.Equal(t, m1.Id, jornada.Matches[1].Match.Id)
	assert.Equal(t, m3.Id, jornada.Matches[2].Match.Id)
}

// TestLeveledGroups_SchedulingStatusOrder verifies pending matches sort by
// scheduling state first (pending < scheduled < confirmed < disputed), so
// unscheduled matches float above proposed/confirmed ones regardless of
// arrange_by — the admin triage order.
func TestLeveledGroups_SchedulingStatusOrder(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "DLA")
	p2 := makePairTB(t, app, "DLB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	// Earlier deadline but confirmed — should sort AFTER the later-deadline
	// pending match, because scheduling state wins over arrange_by.
	mConfirmed := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusConfirmed)
	mScheduled := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusScheduled)
	mPending := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusPending)
	setSlot(t, app, mConfirmed, 1)
	setSlot(t, app, mScheduled, 1)
	setSlot(t, app, mPending, 1)

	setArrangeBy(t, app, mConfirmed, now) // earliest deadline
	setArrangeBy(t, app, mScheduled, now.Add(24*time.Hour))
	setArrangeBy(t, app, mPending, now.Add(48*time.Hour)) // latest deadline

	cards := []MatchCard{
		newLeveledMatchCard(t, app, mConfirmed),
		newLeveledMatchCard(t, app, mScheduled),
		newLeveledMatchCard(t, app, mPending),
	}

	groups := leveledGroups(cards, time.UTC, leveledWindow{target: 3})
	require.Len(t, groups, 1)
	pending := groups[0].Matches
	require.Len(t, pending, 3)
	// pending (rank 0) first despite latest deadline, then scheduled (rank
	// 1), then confirmed (rank 2) last despite earliest deadline.
	assert.Equal(t, mPending.Id, pending[0].Match.Id)
	assert.Equal(t, mScheduled.Id, pending[1].Match.Id)
	assert.Equal(t, mConfirmed.Id, pending[2].Match.Id)
}

// TestLeveledGroups_MonthGroups verifies finalized matches are grouped by
// month, newest first, with correct Spanish titles.
func TestLeveledGroups_MonthGroups(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "MGA")
	p2 := makePairTB(t, app, "MGB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	sep := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	jan := time.Date(2027, 1, 5, 12, 0, 0, 0, time.UTC) // January next year

	mSep := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusFinal)
	mJan := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusFinal)
	setFinalizedAt(t, app, mSep, sep)
	setFinalizedAt(t, app, mJan, jan)

	cards := []MatchCard{
		newLeveledMatchCard(t, app, mSep),
		newLeveledMatchCard(t, app, mJan),
	}

	tz := time.UTC
	groups := leveledGroups(cards, tz, leveledWindow{target: 3})
	// Newest month first: Jan 2027, then Sep 2026
	require.Len(t, groups, 2)
	assert.Equal(t, "played-2027-01", groups[0].Key)
	assert.Equal(t, "Jugados — enero 2027", groups[0].Title)
	assert.Equal(t, "played-2026-09", groups[1].Key)
	assert.Equal(t, "Jugados — septiembre 2026", groups[1].Title)
}

// TestLeveledGroups_Timezone verifies that a match finalized at 23:50 UTC
// in Europe/Madrid time (UTC+2 in summer) falls in the correct local month.
func TestLeveledGroups_Timezone(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "TZA")
	p2 := makePairTB(t, app, "TZB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	// 2026-09-30 23:50 UTC = 2026-10-01 01:50 Europe/Madrid (CEST, UTC+2)
	// So it's October in Madrid, not September.
	ts := time.Date(2026, 9, 30, 23, 50, 0, 0, time.UTC)
	m := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, league.StatusFinal)
	setFinalizedAt(t, app, m, ts)

	madrid, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)

	cards := []MatchCard{newLeveledMatchCard(t, app, m)}
	groups := leveledGroups(cards, madrid, leveledWindow{target: 3})

	require.Len(t, groups, 1)
	assert.Equal(t, "played-2026-10", groups[0].Key, "should be October in Madrid timezone")
	assert.Equal(t, "Jugados — octubre 2026", groups[0].Title)
}

// TestLeveledGroups_JornadaGrouping verifies matches group by slot under
// "Jornada N" titles, with a date range derived from the COMPETITION window
// (start/end/target), not from the matches' own arrange_by values.
func TestLeveledGroups_JornadaGrouping(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "JGA")
	p2 := makePairTB(t, app, "JGB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	// end_date is inclusive, so a 30-day-later end gives a 31-day window;
	// target 3 → ~10.33-day units. Expected ranges come from jornadaRange
	// itself below, so this test tracks the real formula automatically.
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 30)
	w := leveledWindow{start: start, end: end, target: 3}

	m1 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m3 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	setSlot(t, app, m1, 1)
	setSlot(t, app, m2, 2)
	setSlot(t, app, m3, 3)
	// arrange_by values deliberately do NOT match the window's slot ranges —
	// the title must ignore them and use the competition window instead.
	setArrangeBy(t, app, m1, start.AddDate(1, 0, 0))
	setArrangeBy(t, app, m2, start.AddDate(1, 0, 0))
	setArrangeBy(t, app, m3, start.AddDate(1, 0, 0))

	cards := []MatchCard{
		newLeveledMatchCard(t, app, m1),
		newLeveledMatchCard(t, app, m2),
		newLeveledMatchCard(t, app, m3),
	}

	groups := leveledGroups(cards, time.UTC, w)
	require.Len(t, groups, 3)
	assert.Equal(t, "jornada-1", groups[0].Key)
	assert.Equal(t, "jornada-2", groups[1].Key)
	assert.Equal(t, "jornada-3", groups[2].Key)

	lo1, hi1 := w.jornadaRange(1)
	lo2, hi2 := w.jornadaRange(2)
	lo3, hi3 := w.jornadaRange(3)
	assert.Equal(t, fmt.Sprintf("Jornada 1 · %s – %s", fmtShortDate(lo1, time.UTC), fmtShortDate(hi1, time.UTC)), groups[0].Title)
	assert.Equal(t, fmt.Sprintf("Jornada 2 · %s – %s", fmtShortDate(lo2, time.UTC), fmtShortDate(hi2, time.UTC)), groups[1].Title)
	assert.Equal(t, fmt.Sprintf("Jornada 3 · %s – %s", fmtShortDate(lo3, time.UTC), fmtShortDate(hi3, time.UTC)), groups[2].Title)
	assert.Equal(t, end, hi3, "last Jornada is capped at the competition end date")
}

// TestLeveledGroups_JornadaNoDates verifies a competition with no play
// window (legacy data) falls back to a plain "Jornada N" title.
func TestLeveledGroups_JornadaNoDates(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "BGA")
	p2 := makePairTB(t, app, "BGB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	m1 := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	setSlot(t, app, m1, 1)

	cards := []MatchCard{newLeveledMatchCard(t, app, m1)}
	groups := leveledGroups(cards, time.UTC, leveledWindow{target: 3})
	require.Len(t, groups, 1)
	assert.Equal(t, "jornada-1", groups[0].Key)
	assert.Equal(t, "Jornada 1", groups[0].Title, "no window → no date range in title")
}

// TestLeveledGroups_SlotZeroFallback verifies a match with slot=0 (defensive:
// pre-migration data or a non-leveled match reaching this path) lands in the
// "Sin asignar" fallback group instead of being dropped.
func TestLeveledGroups_SlotZeroFallback(t *testing.T) {
	t.Parallel()
	app := newTestAppForGroups(t)

	p1 := makePairTB(t, app, "SZA")
	p2 := makePairTB(t, app, "SZB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})

	m := makeRawMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	// slot left at 0 (default) — no setSlot call.

	cards := []MatchCard{newLeveledMatchCard(t, app, m)}
	groups := leveledGroups(cards, time.UTC, leveledWindow{target: 3})
	require.Len(t, groups, 1)
	assert.Equal(t, "bloque-0", groups[0].Key)
	assert.Equal(t, "Sin asignar", groups[0].Title)
}
