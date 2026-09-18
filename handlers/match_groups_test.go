package handlers

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// newMatchCard builds a minimal MatchCard from a match record for group tests.
func newLeveledMatchCard(t *testing.T, app core.App, m *core.Record) MatchCard {
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

	setArrangeBy(t, app, m1, now.Add(5*24*time.Hour)) // later
	setArrangeBy(t, app, m2, now)                      // earliest

	cards := []MatchCard{
		newLeveledMatchCard(t, app, m1),
		newLeveledMatchCard(t, app, m2),
		newLeveledMatchCard(t, app, m3),
	}

	tz := time.UTC
	groups := leveledGroups(cards, tz)
	require.Len(t, groups, 1, "only pending group")
	pending := groups[0]
	assert.Equal(t, "pending", pending.Key)
	assert.Equal(t, "Por jugar", pending.Title)
	require.Len(t, pending.Matches, 3)
	// m2 (earliest) first, m1 (later) second, m3 (no deadline) last
	assert.Equal(t, m2.Id, pending.Matches[0].Match.Id)
	assert.Equal(t, m1.Id, pending.Matches[1].Match.Id)
	assert.Equal(t, m3.Id, pending.Matches[2].Match.Id)
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
	groups := leveledGroups(cards, tz)
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
	groups := leveledGroups(cards, madrid)

	require.Len(t, groups, 1)
	assert.Equal(t, "played-2026-10", groups[0].Key, "should be October in Madrid timezone")
	assert.Equal(t, "Jugados — octubre 2026", groups[0].Title)
}
