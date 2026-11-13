package migrations

import (
	"testing"

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
