package migrations

import (
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jornadaFixCompetition builds a 12-pair leveled competition (target=10 <
// 12-1=11) with a 101-day inclusive window (2026-01-01 to 2026-04-11),
// matching TestSlotDeadline/TestJornadaWindow's fixture dates.
func jornadaFixCompetition(t *testing.T, app core.App) *core.Record {
	t.Helper()
	pairs := make([]*core.Record, 12)
	for i := range pairs {
		pairs[i] = makeSlotTestPair(t, app, fmt.Sprintf("JornadaFix%d", i))
	}
	comp := makeSlotTestCompetition(t, app, pairs, 10)
	comp.Set("start_date", "2026-01-01T00:00:00Z")
	comp.Set("end_date", "2026-04-11T00:00:00Z")
	require.NoError(t, app.Save(comp))
	return comp
}

func jornadaFixMatch(t *testing.T, app core.App, compID, p1, p2 string, slot int, arrangeBy string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	m := core.NewRecord(col)
	m.Set("competition", compID)
	m.Set("pair1", p1)
	m.Set("pair2", p2)
	m.Set("status", "pending")
	m.Set("round_number", 0)
	m.Set("slot", slot)
	if arrangeBy != "" {
		m.Set("arrange_by", arrangeBy)
	}
	require.NoError(t, app.Save(m))
	return m
}

// TestFixCompetitionArrangeBy_RecomputesDriftedValue verifies the migration
// overwrites an arrange_by value that matches the old (exclusive-end-date)
// formula's output with the corrected (inclusive-end-date) value.
func TestFixCompetitionArrangeBy_RecomputesDriftedValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := jornadaFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	oldDeadline, ok := oldBuggySlotDeadline(comp, 3)
	require.True(t, ok)
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], 3, oldDeadline.Format("2006-01-02"))

	require.NoError(t, fixCompetitionArrangeBy(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	wantDeadline, ok := fixedSlotDeadline(comp, 3)
	require.True(t, ok)
	assert.Equal(t, wantDeadline.Format("2006-01-02"), got.GetString("arrange_by")[:10],
		"drifted arrange_by must be recomputed to the corrected deadline")
}

// TestFixCompetitionArrangeBy_PreservesAdminEditedValue verifies the
// migration leaves an arrange_by value untouched when it does NOT match
// what the old buggy formula would have produced — the admin-edited case.
func TestFixCompetitionArrangeBy_PreservesAdminEditedValue(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := jornadaFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	// An admin-edited date that does not match the old formula's output
	// for slot 3 (2026-01-31) at all — nowhere near either the old or new
	// computed deadline.
	adminEdited := "2026-02-15"
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], 3, adminEdited)

	require.NoError(t, fixCompetitionArrangeBy(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	assert.Equal(t, adminEdited, got.GetString("arrange_by")[:10],
		"an admin-edited arrange_by that doesn't match the old buggy formula must be left untouched")
}

// TestFixCompetitionArrangeBy_SkipsFinalMatches verifies the migration
// never touches a finalized match's arrange_by.
func TestFixCompetitionArrangeBy_SkipsFinalMatches(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := jornadaFixCompetition(t, app)
	pairs := comp.GetStringSlice("pairs")

	oldDeadline, ok := oldBuggySlotDeadline(comp, 3)
	require.True(t, ok)
	m := jornadaFixMatch(t, app, comp.Id, pairs[0], pairs[1], 3, oldDeadline.Format("2006-01-02"))
	m.Set("status", "final")
	require.NoError(t, app.Save(m))

	require.NoError(t, fixCompetitionArrangeBy(app, comp))

	got, err := app.FindRecordById("matches", m.Id)
	require.NoError(t, err)
	assert.Equal(t, oldDeadline.Format("2006-01-02"), got.GetString("arrange_by")[:10],
		"a final match's arrange_by must not be recomputed")
}

// TestOldBuggySlotDeadline_MatchesOldFormula sanity-checks
// oldBuggySlotDeadline reproduces the pre-fix (exclusive end-date) result
// for the scheduling_test.go TestSlotDeadline fixture.
func TestOldBuggySlotDeadline_MatchesOldFormula(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)
	comp := jornadaFixCompetition(t, app)

	got, ok := oldBuggySlotDeadline(comp, 3)
	require.True(t, ok)
	want := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, want, got)
}
