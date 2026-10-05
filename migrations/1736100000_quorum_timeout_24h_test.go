package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuorumTimeout24h_LowersOnly48hValues(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	hoursByID := map[string]float64{}
	for _, hours := range []float64{0, 12, 48, 48, 72} {
		comp := core.NewRecord(col)
		comp.Set("name", "Quorum")
		comp.Set("type", "league")
		comp.Set("quorum_timeout_hours", hours)
		require.NoError(t, app.Save(comp))
		hoursByID[comp.Id] = hours
	}
	settings, err := app.FindFirstRecordByFilter("app_settings", "")
	require.NoError(t, err)
	settings.Set("quorum_timeout_hours", 48)
	require.NoError(t, app.Save(settings))

	require.NoError(t, quorumTimeout24h(app))

	want := map[float64]float64{0: 0, 12: 12, 48: 24, 72: 72}
	for id, before := range hoursByID {
		got, err := app.FindRecordById("competitions", id)
		require.NoError(t, err)
		assert.Equal(t, want[before], got.GetFloat("quorum_timeout_hours"), "was %v", before)
	}
	settings, err = app.FindRecordById("app_settings", settings.Id)
	require.NoError(t, err)
	assert.Equal(t, float64(24), settings.GetFloat("quorum_timeout_hours"))
}

func TestQuorumTimeout24h_KeepsCustomDefault(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	settings, err := app.FindFirstRecordByFilter("app_settings", "")
	require.NoError(t, err)
	settings.Set("quorum_timeout_hours", 36)
	require.NoError(t, app.Save(settings))

	require.NoError(t, quorumTimeout24h(app))

	settings, err = app.FindRecordById("app_settings", settings.Id)
	require.NoError(t, err)
	assert.Equal(t, float64(36), settings.GetFloat("quorum_timeout_hours"))
}
