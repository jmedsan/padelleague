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
