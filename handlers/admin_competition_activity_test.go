package handlers

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newActivityTestRecord creates and saves a competitions record, then
// re-fetches it — matching the real Update handler's before := record.
// Original() on a FindRecordById result, so Original() reflects the saved
// baseline rather than the zero-value record used at construction time.
func newActivityTestRecord(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", "Test Competition")
	record.Set("type", "league")
	require.NoError(t, app.Save(record))

	fresh, err := app.FindRecordById("competitions", record.Id)
	require.NoError(t, err)
	return fresh
}

func TestCompetitionUpdateDetail_GenderTypeUsesSpanishLabel(t *testing.T) {
	app := newTestApp(t)
	record := newActivityTestRecord(t, app)
	before := record.Original()

	record.Set("gender_type", "free")
	require.NoError(t, app.Save(record))

	detail := competitionUpdateDetail(before, record)
	assert.Equal(t, "actualizó la configuración: Género: — → Libre", detail)
}

func TestCompetitionUpdateDetail_TypeUsesSpanishLabel(t *testing.T) {
	app := newTestApp(t)
	record := newActivityTestRecord(t, app)
	before := record.Original()

	record.Set("type", "playoff")
	require.NoError(t, app.Save(record))

	detail := competitionUpdateDetail(before, record)
	assert.Equal(t, "actualizó la configuración: Tipo: Liga → Playoff", detail)
}

func TestCompetitionUpdateDetail_MultipleFieldsJoinedWithVerbPrefix(t *testing.T) {
	app := newTestApp(t)
	record := newActivityTestRecord(t, app)
	before := record.Original()

	record.Set("gender_type", "male")
	record.Set("walkover_score", "6-0 6-0")
	require.NoError(t, app.Save(record))

	detail := competitionUpdateDetail(before, record)
	assert.Equal(t, "actualizó la configuración: Género: — → Masculina; Marcador de incomparecencia: — → 6-0 6-0", detail)
}

func TestCompetitionUpdateDetail_NoChangeReturnsEmpty(t *testing.T) {
	app := newTestApp(t)
	record := newActivityTestRecord(t, app)
	before := record.Original()

	detail := competitionUpdateDetail(before, record)
	assert.Equal(t, "", detail)
}

func TestFmtActivityGenderType_UnrecognizedValuePassesThrough(t *testing.T) {
	assert.Equal(t, "—", fmtActivityGenderType(""))
	assert.Equal(t, "Femenina", fmtActivityGenderType("female"))
	assert.Equal(t, "mixto-raro", fmtActivityGenderType("mixto-raro"))
}

func TestFmtActivityCompType_UnrecognizedValuePassesThrough(t *testing.T) {
	assert.Equal(t, "—", fmtActivityCompType(""))
	assert.Equal(t, "Liga", fmtActivityCompType("league"))
	assert.Equal(t, "torneo-raro", fmtActivityCompType("torneo-raro"))
}
