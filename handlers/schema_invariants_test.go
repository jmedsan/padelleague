package handlers

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProtectedFilesRequireViewRule verifies that every collection with a
// Protected file field has a ViewRule that allows authenticated users.
// Without a ViewRule, PocketBase's /api/files/ endpoint rejects file-token
// requests — even valid ones — because the user cannot "view" the record.
// This caught the documents collection bug where ViewRule=nil + Protected=true
// made all document downloads return 404.
func TestProtectedFilesRequireViewRule(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	collections, err := app.FindAllCollections()
	require.NoError(t, err)

	for _, col := range collections {
		if col.System || strings.HasPrefix(col.Name, "_pb") || strings.HasPrefix(col.Name, "demo") {
			continue
		}
		for _, f := range col.Fields {
			ff, ok := f.(*core.FileField)
			if !ok || !ff.Protected {
				continue
			}
			assert.NotNilf(t, col.ViewRule, "collection %q has Protected file field %q but ViewRule is nil (superuser-only) — authenticated users cannot download files via /api/files/", col.Name, ff.Name)
		}
	}
}
