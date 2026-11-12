package migrations

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func matchStatusValues(t *testing.T, app core.App) []string {
	t.Helper()
	return mustCollection(t, app, "matches").Fields.GetByName("status").(*core.SelectField).Values
}

func TestMatchStatusReleased_UpAddsAndDownRemovesTheValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	require.NoError(t, removeReleasedStatus(app))
	assert.NotContains(t, matchStatusValues(t, app), "released")

	require.NoError(t, addReleasedStatus(app))
	assert.Contains(t, matchStatusValues(t, app), "released")

	// Idempotent: a second up must not duplicate the value.
	require.NoError(t, addReleasedStatus(app))
	count := 0
	for _, v := range matchStatusValues(t, app) {
		if v == "released" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestMatchStatusReleased_DownRefusesWhileAMatchIsReleased(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	pa := makeSlotTestPair(t, app, "RelA")
	pb := makeSlotTestPair(t, app, "RelB")
	pc := makeSlotTestPair(t, app, "RelC")
	comp := makeSlotTestCompetition(t, app, []*core.Record{pa, pb, pc}, 1)
	m := makeSlotTestMatch(t, app, comp.Id, pa.Id, pb.Id, time.Now())
	m.Set("status", "released")
	require.NoError(t, app.Save(m))

	require.Error(t, removeReleasedStatus(app))
	assert.Contains(t, matchStatusValues(t, app), "released")
}
