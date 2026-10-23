package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var walkoverScoreCollections = []string{"competitions", "app_settings"}

func TestDropWalkoverScore_UpRemovesFieldsAndDownRestoresThem(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	require.NoError(t, removeWalkoverScoreFields(app))
	for _, name := range walkoverScoreCollections {
		assert.Nil(t, mustCollection(t, app, name).Fields.GetByName("walkover_score"), name)
	}

	require.NoError(t, addWalkoverScoreFields(app))
	for _, name := range walkoverScoreCollections {
		field := mustCollection(t, app, name).Fields.GetByName("walkover_score")
		require.NotNil(t, field, name)
		assert.Equal(t, core.FieldTypeText, field.Type(), name)
	}
}
