package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/internal/testapp"
)

func TestPromotionsCollection(t *testing.T) {
	t.Parallel()
	app := testapp.New(t)
	col, err := app.FindCollectionByNameOrId("promotions")
	require.NoError(t, err)

	title, ok := col.Fields.GetByName("title").(*core.TextField)
	require.True(t, ok)
	assert.True(t, title.Required)
	assert.Equal(t, 100, title.Max)

	desc, ok := col.Fields.GetByName("description").(*core.TextField)
	require.True(t, ok)
	assert.False(t, desc.Required)
	assert.Equal(t, 300, desc.Max)

	img, ok := col.Fields.GetByName("image").(*core.FileField)
	require.True(t, ok)
	assert.Equal(t, 1, img.MaxSelect)
	assert.Equal(t, int64(5<<20), img.MaxSize)
	assert.False(t, img.Required)

	u, ok := col.Fields.GetByName("url").(*core.URLField)
	require.True(t, ok)
	assert.True(t, u.Required)

	_, ok = col.Fields.GetByName("expires_on").(*core.DateField)
	assert.True(t, ok)
	_, ok = col.Fields.GetByName("active").(*core.BoolField)
	assert.True(t, ok)
	for _, name := range []string{"views", "clicks"} {
		n, ok := col.Fields.GetByName(name).(*core.NumberField)
		require.True(t, ok, name)
		assert.True(t, n.OnlyInt, name)
		require.NotNil(t, n.Min, name)
		assert.Equal(t, 0.0, *n.Min, name)
	}
	for _, name := range []string{"created", "updated"} {
		_, ok := col.Fields.GetByName(name).(*core.AutodateField)
		assert.True(t, ok, name)
	}

	assert.Nil(t, col.ListRule)
	assert.Nil(t, col.ViewRule)
	assert.Nil(t, col.CreateRule)
	assert.Nil(t, col.UpdateRule)
	assert.Nil(t, col.DeleteRule)

	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("url", "https://a.example")
	require.NoError(t, app.Save(rec))
	assert.Equal(t, 0, rec.GetInt("views"))
	assert.Equal(t, 0, rec.GetInt("clicks"))
	assert.False(t, rec.GetBool("active"))
}
