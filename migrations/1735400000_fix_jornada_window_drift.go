package migrations

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Recomputes arrange_by for every non-final leveled match, fixing the
// off-by-one Jornada window drift: the window unit was computed as
// (end_date - start_date) / target, treating the inclusive end_date as an
// exclusive instant, so the displayed Jornada range and the stored
// arrange_by deadline for the same slot disagreed by up to one day, and the
// deadline could fall on the next Jornada's first day. Existing matches
// generated before the fix carry the old, drifted arrange_by; this
// backfills them with the corrected value. Irreversible: the down
// migration cannot recover the pre-fix (wrong) values, and there is no
// reason to.
func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindRecordsByFilter("competitions", "target_matches > 0", "", 0, 0, nil)
		if err != nil {
			return err
		}
		for _, comp := range comps {
			if !isJornadaLeveledComp(comp) {
				continue
			}
			if err := fixCompetitionArrangeBy(app, comp); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}

func isJornadaLeveledComp(comp *core.Record) bool {
	target := comp.GetInt("target_matches")
	pairs := comp.GetStringSlice("pairs")
	return target > 0 && target < len(pairs)-1
}

func fixCompetitionArrangeBy(app core.App, comp *core.Record) error {
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp} && status != 'final'", "", 0, 0, map[string]any{"comp": comp.Id})
	if err != nil {
		return err
	}
	for _, rec := range matches {
		slot := rec.GetInt("slot")
		if slot <= 0 {
			continue
		}
		current := rec.GetDateTime("arrange_by").Time()
		oldComputed, ok := oldBuggySlotDeadline(comp, slot)
		if !ok {
			continue
		}
		// Every write path for a leveled match's arrange_by (setMatchFields
		// at generation, refreshLeveledArrangeBy after a date change) uses
		// this same formula — there is no admin UI to set arrange_by on an
		// individual leveled match. Still: only overwrite a value that
		// matches what the old (buggy) formula would have produced, so a
		// value set by any other means is left untouched rather than
		// assumed to be the bug's output. Compared by calendar date, not
		// full timestamp: arrange_by is stored as a date-only string
		// ("2006-01-02"), so it always reloads at midnight UTC regardless
		// of the noon-normalized time.Time the formula computed.
		cy, cm, cd := current.UTC().Date()
		oy, om, od := oldComputed.UTC().Date()
		if cy != oy || cm != om || cd != od {
			continue
		}
		deadline, ok := fixedSlotDeadline(comp, slot)
		if !ok {
			continue
		}
		rec.Set("arrange_by", deadline.Format("2006-01-02"))
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// oldBuggySlotDeadline reproduces the pre-fix formula (end_date treated as
// an exclusive instant) so fixCompetitionArrangeBy can recognize which
// stored arrange_by values were actually produced by the bug.
func oldBuggySlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
	target := comp.GetInt("target_matches")
	if target <= 0 || slot <= 0 {
		return time.Time{}, false
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	u := end.Sub(start) / time.Duration(target)
	deadline := start.Add(time.Duration(slot) * u)
	if deadline.After(end) {
		deadline = end
	}
	y, mo, d := deadline.UTC().Date()
	return time.Date(y, mo, d, 12, 0, 0, 0, time.UTC), true
}

// fixedSlotDeadline mirrors league.SlotDeadline / league.JornadaWindow's
// inclusive-end-date math — kept self-contained since migrations must not
// depend on application code that can change shape after they run.
func fixedSlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
	target := comp.GetInt("target_matches")
	if target <= 0 || slot <= 0 {
		return time.Time{}, false
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	u := end.AddDate(0, 0, 1).Sub(start) / time.Duration(target)
	deadline := start.Add(time.Duration(slot)*u - 24*time.Hour)
	if slot >= target || deadline.After(end) {
		deadline = end
	}
	y, mo, d := deadline.UTC().Date()
	return time.Date(y, mo, d, 12, 0, 0, 0, time.UTC), true
}
