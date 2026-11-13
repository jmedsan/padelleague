package migrations

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropMatchRematch_UpDropsAndDownRestores(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	require.NoError(t, restoreMatchRematch(app))
	assert.NotNil(t, mustCollection(t, app, "matches").Fields.GetByName("rematch"))

	require.NoError(t, dropMatchRematch(app))
	assert.Nil(t, mustCollection(t, app, "matches").Fields.GetByName("rematch"))

	// Idempotent both ways.
	require.NoError(t, dropMatchRematch(app))
	require.NoError(t, restoreMatchRematch(app))
	require.NoError(t, restoreMatchRematch(app))
	assert.NotNil(t, mustCollection(t, app, "matches").Fields.GetByName("rematch"))
}

func TestDropMatchRematch_ReleasesOpenRematchesKeepsFinal(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	require.NoError(t, restoreMatchRematch(app))

	a, b := makeSlotTestPair(t, app, "RA"), makeSlotTestPair(t, app, "RB")
	comp := makeSlotTestCompetition(t, app, []*core.Record{a, b}, 1)
	mk := func(status string, rematch bool) *core.Record {
		m := makeSlotTestMatch(t, app, comp.Id, a.Id, b.Id, time.Now())
		m.Set("status", status)
		m.Set("rematch", rematch)
		require.NoError(t, app.Save(m))
		return m
	}
	openRematch, finalRematch, openNormal := mk("pending", true), mk("final", true), mk("pending", false)

	require.NoError(t, dropMatchRematch(app))

	status := func(m *core.Record) string {
		r, err := app.FindRecordById("matches", m.Id)
		require.NoError(t, err)
		return r.GetString("status")
	}
	assert.Equal(t, "released", status(openRematch))
	assert.Equal(t, "final", status(finalRematch))
	assert.Equal(t, "pending", status(openNormal))

	entries, err := app.FindRecordsByFilter("match_messages", "type = 'admin_action'", "", 0, 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, openRematch.Id, entries[0].GetString("match"))
	assert.Equal(t, "liberó el partido", entries[0].GetString("content"))
	assert.Empty(t, entries[0].GetString("author"))
}
