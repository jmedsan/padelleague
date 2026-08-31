package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerTasks_RecoveryPhase_FlagsOrganizeTask(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PtRecA")
	p2 := makePair(t, app, "PtRecB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	start := time.Now().AddDate(0, 0, -40)
	end := time.Now().AddDate(0, 0, -20)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("recovery_days", 30) // now (end+20d) stays within end+30d -> recovery
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	userID := p1.GetString("player1")
	tasks, err := PlayerTasks(app, userID, time.Now())
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	assert.Equal(t, TaskOrganize, tasks[0].Kind)
	assert.True(t, tasks[0].Recovery, "organize task in a recovery-phase competition must be flagged")
}

func TestPlayerTasks_DenominatorDrift_UsesStoredSchedule(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "DriftA")
	p2 := makePair(t, app, "DriftB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// 60-day window ending 10 days from now.
	// With rounds=2, round 1 deadline = start + 30d = 20 days ago → overdue → task appears.
	// With rounds=1, round 1 deadline = start + 60d = end = 10 days from now → future → no task.
	// Storing schedule with rounds=2 locks the correct (past) deadline.
	now := time.Now()
	start := now.AddDate(0, 0, -50)
	end := now.AddDate(0, 0, 10)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 2)
	comp.Set("round_arrange_dates", StoreRoundSchedule(start, end, 2))
	comp.Set("recovery_days", 30)
	require.NoError(t, app.Save(comp))

	// Now shrink rounds to 1 — simulating a pair drop after fixtures.
	comp.Set("rounds", 1)
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	userID := p1.GetString("player1")
	tasks, err := PlayerTasks(app, userID, now)
	require.NoError(t, err)

	// The stored schedule (rounds=2) puts round 1 at start+30d = 20 days ago → overdue.
	// If it recomputed with rounds=1, deadline would be end = 10 days from now → no task.
	require.Len(t, tasks, 1, "stored schedule must be used, not recomputed with shrunk denominator")
	assert.Equal(t, TaskOrganize, tasks[0].Kind)
	assert.Equal(t, WarnOverdue, tasks[0].Warning)
}

func TestPlayerTasks_FinishedByDate_SkipsPendingMatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "PtFinA")
	p2 := makePair(t, app, "PtFinB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	start := time.Now().AddDate(0, 0, -60)
	end := time.Now().AddDate(0, 0, -30)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("recovery_days", 14) // now (end+30d) is past end+14d -> finished
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	userID := p1.GetString("player1")
	tasks, err := PlayerTasks(app, userID, time.Now())
	require.NoError(t, err)

	assert.Empty(t, tasks, "a finished-by-date competition must not surface its pending match")
}

func TestPlayerTasks_ExactWarnHeadsUp_Boundary(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "HuBndA")
	p2 := makePair(t, app, "HuBndB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// headsUpStartDays=5, so WarnHeadsUp triggers when now >= deadline-5d.
	// With rounds=1, deadline = noon UTC on end date. Place now at noon - 5 days.
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC) // noon-anchored → Sep 15 12:00
	start := now.AddDate(0, 0, -20)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("round_arrange_dates", StoreRoundSchedule(start, end, 1))
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	userID := p1.GetString("player1")
	tasks, err := PlayerTasks(app, userID, now)
	require.NoError(t, err)

	require.Len(t, tasks, 1, "at exact WarnHeadsUp boundary (>=), organize task must appear")
	assert.Equal(t, TaskOrganize, tasks[0].Kind)
	assert.Equal(t, WarnHeadsUp, tasks[0].Warning)
}

func TestPlayerTasks_OpponentName_ResolvesRealPairName(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "OppNameA")
	p2 := makePair(t, app, "OppNameB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	// Place now at deadline - 3 days so the task surfaces (WarnHeadsUp).
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	end := now.AddDate(0, 0, 3) // deadline within headsUp window
	start := now.AddDate(0, 0, -20)
	sd, _ := types.ParseDateTime(start)
	ed, _ := types.ParseDateTime(end)
	comp.Set("start_date", sd)
	comp.Set("end_date", ed)
	comp.Set("rounds", 1)
	comp.Set("round_arrange_dates", StoreRoundSchedule(start, end, 1))
	require.NoError(t, app.Save(comp))

	makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusPending)

	userID := p1.GetString("player1")
	tasks, err := PlayerTasks(app, userID, now)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	assert.Equal(t, "OppNameB", tasks[0].Opponent, "opponent must resolve to pair name, not '?'")
}

func TestSortTasks_OrderByWarningDescThenKind(t *testing.T) {
	t.Parallel()
	tasks := []PlayerTask{
		{Kind: TaskOrganize, Warning: WarnHeadsUp, MatchID: "a"},
		{Kind: TaskOrganize, Warning: WarnOverdue, MatchID: "b"},
		{Kind: TaskPlay, RoundNumber: 2, MatchID: "c"},
		{Kind: TaskOrganize, Warning: WarnUrgent, MatchID: "d"},
		{Kind: TaskPlay, RoundNumber: 1, MatchID: "e"},
	}
	sortTasks(tasks)

	// TaskPlay (0) < TaskOrganize (1), so play tasks come first sorted by round.
	assert.Equal(t, "e", tasks[0].MatchID, "play round 1 first")
	assert.Equal(t, "c", tasks[1].MatchID, "play round 2 second")
	// Organize tasks sorted by warning desc: overdue > urgent > headsup.
	assert.Equal(t, "b", tasks[2].MatchID, "organize overdue")
	assert.Equal(t, "d", tasks[3].MatchID, "organize urgent")
	assert.Equal(t, "a", tasks[4].MatchID, "organize headsup")
}
