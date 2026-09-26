package migrations

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wholeDayFixCompetition builds a 12-pair leveled competition (target=10 <
// 12-1=11) with a 101-day inclusive window (2026-01-01 to 2026-04-11),
// matching TestSlotDeadline/TestJornadaWindow's fixture dates — a non-
// multiple-of-target season, so the fractional-day and whole-day formulas
// disagree.
func wholeDayFixCompetition(t *testing.T, app core.App) *core.Record {
	t.Helper()
	pairs := make([]*core.Record, 12)
	for i := range pairs {
		pairs[i] = makeSlotTestPair(t, app, fmt.Sprintf("WholeDayFix%d", i))
	}
	comp := makeSlotTestCompetition(t, app, pairs, 10)
	comp.Set("start_date", "2026-01-01T00:00:00Z")
	comp.Set("end_date", "2026-04-11T00:00:00Z")
	require.NoError(t, app.Save(comp))
	return comp
}

// TestFixCompetitionArrangeByWholeDay_RecomputesFractionalDayValue verifies
// the migration overwrites an arrange_by produced by 1735400000's own
// (still fractional-day) formula with the corrected whole-day value.
func TestFixCompetitionArrangeByWholeDay_RecomputesFractionalDayValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := wholeDayFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	fractional, ok := fractionalDaySlotDeadline(comp, 3)
	require.True(t, ok)
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], fractional.Format("2006-01-02"))

	require.NoError(t, fixCompetitionArrangeByWholeDay(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	want, ok := wholeDaySlotDeadline(comp, 3)
	require.True(t, ok)
	assert.Equal(t, want.Format("2006-01-02"), got.GetString("arrange_by")[:10],
		"a value produced by 1735400000's fractional-day formula must be recomputed to the whole-day value")
}

// TestFixCompetitionArrangeByWholeDay_RecomputesOriginalBuggyValue verifies
// the migration also catches a database that never ran 1735400000 at all —
// an arrange_by still carrying the very first (pre-1735400000) buggy
// formula's output.
func TestFixCompetitionArrangeByWholeDay_RecomputesOriginalBuggyValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := wholeDayFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	original, ok := originalBuggySlotDeadline(comp, 3)
	require.True(t, ok)
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], original.Format("2006-01-02"))

	require.NoError(t, fixCompetitionArrangeByWholeDay(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	want, ok := wholeDaySlotDeadline(comp, 3)
	require.True(t, ok)
	assert.Equal(t, want.Format("2006-01-02"), got.GetString("arrange_by")[:10],
		"a value produced by the original pre-1735400000 formula must also be recomputed to the whole-day value")
}

// TestFixCompetitionArrangeByWholeDay_PreservesAdminEditedValue verifies the
// migration leaves an arrange_by untouched when it matches neither prior
// formula's output — the admin-edited case.
func TestFixCompetitionArrangeByWholeDay_PreservesAdminEditedValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := wholeDayFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	adminEdited := "2026-02-15"
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], adminEdited)

	require.NoError(t, fixCompetitionArrangeByWholeDay(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	assert.Equal(t, adminEdited, got.GetString("arrange_by")[:10],
		"an admin-edited arrange_by matching neither prior formula must be left untouched")
}

// TestFixCompetitionArrangeByWholeDay_SkipsFinalMatches verifies the
// migration never touches a finalized match's arrange_by.
func TestFixCompetitionArrangeByWholeDay_SkipsFinalMatches(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := wholeDayFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	fractional, ok := fractionalDaySlotDeadline(comp, 3)
	require.True(t, ok)
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], fractional.Format("2006-01-02"))
	m.Set("status", "final")
	require.NoError(t, app.Save(m))

	require.NoError(t, fixCompetitionArrangeByWholeDay(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	assert.Equal(t, fractional.Format("2006-01-02"), got.GetString("arrange_by")[:10],
		"a final match's arrange_by must not be recomputed")
}
