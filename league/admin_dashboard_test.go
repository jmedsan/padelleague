package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminDashboard_RecoveryPhase_FlagsOverdueAlert(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "AdRecA")
	p2 := makePair(t, app, "AdRecB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	start := time.Now().AddDate(0, 0, -40)
	end := time.Now().AddDate(0, 0, -20)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("recovery_days", 30)
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	_, alerts, err := AdminDashboard(app, time.Now())
	require.NoError(t, err)

	require.Len(t, alerts, 1)
	assert.Equal(t, "overdue", alerts[0].Kind)
	assert.True(t, alerts[0].Recovery, "overdue alert in a recovery-phase competition must be flagged")
}

func TestAdminDashboard_FinishedByDate_SkipsOverdueButKeepsWalkover(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "AdFinA")
	p2 := makePair(t, app, "AdFinB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	start := time.Now().AddDate(0, 0, -60)
	end := time.Now().AddDate(0, 0, -30)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("recovery_days", 14)
	require.NoError(t, app.Save(comp))

	m := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)
	m.Set("review_type", "walkover")
	require.NoError(t, app.Save(m))

	_, alerts, err := AdminDashboard(app, time.Now())
	require.NoError(t, err)

	require.Len(t, alerts, 1, "a finished-by-date competition still surfaces a pending walkover approval")
	assert.Equal(t, "walkover", alerts[0].Kind)
}

func TestOutstandingMatches_OrderingAndFields(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePair(t, app, "OutA")
	p2 := makePair(t, app, "OutB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	start := time.Now().AddDate(0, 0, -30)
	end := time.Now().AddDate(0, 0, 10)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 2)
	comp.Set("arrange_grace_days", 3)
	require.NoError(t, app.Save(comp))

	// Round 1 deadline = start + 20d = 10 days ago -> well past the 3-day grace.
	overdue := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)
	overdue.Set("round_number", 1)
	require.NoError(t, app.Save(overdue))

	// Round 2 deadline = end = 10 days from now -> WarnNone.
	future := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusConfirmed)
	future.Set("round_number", 2)
	require.NoError(t, app.Save(future))

	// Final match in the same competition must be excluded.
	final := makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusFinal)
	final.Set("round_number", 2)
	require.NoError(t, app.Save(final))

	// Match in an inactive competition must be excluded entirely.
	inactiveComp := makeCompetition(t, app, []*core.Record{p1, p2})
	inactiveComp.Set("active", false)
	require.NoError(t, app.Save(inactiveComp))
	makeMatch(t, app, inactiveComp.Id, p1.Id, p2.Id, StatusPending)

	// Playoff match: no round schedule, must show status only (no deadline/warning).
	playoffCol, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	playoffComp := core.NewRecord(playoffCol)
	playoffComp.Set("name", "Playoff Out")
	playoffComp.Set("type", "playoff")
	playoffComp.Set("active", true)
	playoffComp.Set("pairs", []string{p1.Id, p2.Id})
	require.NoError(t, app.Save(playoffComp))
	playoffMatch := makeMatch(t, app, playoffComp.Id, p1.Id, p2.Id, StatusPending)

	out := OutstandingMatches(app, time.Now())

	ids := make([]string, len(out))
	for i, om := range out {
		ids[i] = om.MatchID
	}
	assert.NotContains(t, ids, final.Id, "final matches must be excluded")
	assert.NotContains(t, ids, "", "no blank match IDs")
	for _, id := range ids {
		require.NotEqual(t, inactiveComp.Id, id, "inactive competition's matches must be excluded")
	}
	require.Len(t, out, 3)

	// Most-urgent first: the overdue round-1 match leads.
	assert.Equal(t, overdue.Id, out[0].MatchID)
	assert.Equal(t, "Test Competition", out[0].CompetitionName)
	assert.Equal(t, 1, out[0].RoundNumber)
	assert.Equal(t, "OutA", out[0].Pair1)
	assert.Equal(t, "OutB", out[0].Pair2)
	assert.Equal(t, StatusPending, out[0].Status)
	assert.Equal(t, WarnOverdue, out[0].Warning)
	assert.Equal(t, start.AddDate(0, 0, 20).Format("02/01"), out[0].ArrangeBy)

	// The playoff match (no deadline, WarnNone) and the future round-2 match
	// (WarnNone) both sort after the overdue one; the playoff match has no
	// deadline so it must sort after the deadline-bearing future match.
	assert.Equal(t, future.Id, out[1].MatchID)
	assert.Equal(t, WarnNone, out[1].Warning)
	assert.NotEmpty(t, out[1].ArrangeBy)

	assert.Equal(t, playoffMatch.Id, out[2].MatchID)
	assert.Equal(t, WarnNone, out[2].Warning)
	assert.Empty(t, out[2].ArrangeBy, "playoff matches show status only, no deadline")
}

func TestSortOutstanding_SameWarning_TiebreakByDeadline(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	out := []OutstandingMatch{
		{MatchID: "late-overdue", Warning: WarnOverdue, deadline: late},
		{MatchID: "early-overdue", Warning: WarnOverdue, deadline: early},
		{MatchID: "headsup", Warning: WarnHeadsUp, deadline: early},
		{MatchID: "urgent", Warning: WarnUrgent, deadline: early},
	}
	sortOutstanding(out)

	// Warning desc: overdue(3) > urgent(2) > headsup(1).
	// Within same warning, deadline asc: early before late.
	assert.Equal(t, "early-overdue", out[0].MatchID)
	assert.Equal(t, "late-overdue", out[1].MatchID)
	assert.Equal(t, "urgent", out[2].MatchID)
	assert.Equal(t, "headsup", out[3].MatchID)
}

func TestSortAlerts_OrderByKind(t *testing.T) {
	t.Parallel()
	alerts := []AdminAlert{
		{Kind: "overdue", MatchID: "a"},
		{Kind: "dispute", MatchID: "b"},
		{Kind: "walkover", MatchID: "c"},
		{Kind: "dispute", MatchID: "d"},
	}
	sortAlerts(alerts)

	assert.Equal(t, "dispute", alerts[0].Kind)
	assert.Equal(t, "dispute", alerts[1].Kind)
	assert.Equal(t, "walkover", alerts[2].Kind)
	assert.Equal(t, "overdue", alerts[3].Kind)
	// Verify stable sort preserves insertion order within same kind.
	assert.Equal(t, "b", alerts[0].MatchID)
	assert.Equal(t, "d", alerts[1].MatchID)
}
