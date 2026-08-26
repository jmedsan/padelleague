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
