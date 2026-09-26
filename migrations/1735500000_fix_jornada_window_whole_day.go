package migrations

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Recomputes arrange_by for every non-final leveled match, fixing the
// fractional-day Jornada drift that 1735400000 left in place: that
// migration's own "fixed" formula still divided the season into a Duration
// (D/target), which truncates to sub-day units whenever D isn't a multiple
// of target, so a Jornada's deadline could still land before "now" for a
// match created inside that same Jornada. This backfills every match whose
// stored arrange_by matches either formula 1735400000 could have produced —
// its own corrected-but-still-fractional output, or the original pre-1735400000
// buggy output (for a database that never ran that migration) — to the new
// whole-day value. An arrange_by matching neither is an admin edit and is
// left untouched. Irreversible: the down migration cannot recover the
// pre-fix (wrong) values, and there is no reason to.
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
			if err := fixCompetitionArrangeByWholeDay(app, comp); err != nil {
				return err
			}
		}
		return nil
	}, func(_ core.App) error {
		return nil
	})
}

func fixCompetitionArrangeByWholeDay(app core.App, comp *core.Record) error {
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
		if !matchesPriorFormula(comp, slot, current) {
			continue
		}
		deadline, ok := wholeDaySlotDeadline(comp, slot)
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

// matchesPriorFormula reports whether current (compared by calendar date,
// since arrange_by reloads at midnight UTC regardless of the noon-normalized
// time.Time a formula computes) matches what either prior formula would have
// produced for slot — 1735400000's own corrected-but-fractional output, or
// the original pre-1735400000 buggy output for a database that skipped it.
// Only a match against one of these two known-buggy outputs is overwritten;
// anything else is presumed an admin edit.
func matchesPriorFormula(comp *core.Record, slot int, current time.Time) bool {
	for _, formula := range []func(*core.Record, int) (time.Time, bool){
		fractionalDaySlotDeadline,
		originalBuggySlotDeadline,
	} {
		computed, ok := formula(comp, slot)
		if !ok {
			continue
		}
		cy, cm, cd := current.UTC().Date()
		fy, fm, fd := computed.UTC().Date()
		if cy == fy && cm == fm && cd == fd {
			return true
		}
	}
	return false
}

// originalBuggySlotDeadline reproduces the pre-1735400000 formula (end_date
// treated as an exclusive instant).
func originalBuggySlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
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

// fractionalDaySlotDeadline reproduces 1735400000's formula: end_date
// treated as inclusive, but the per-Jornada unit is still a Duration
// (D/target), which truncates to a fractional day whenever D isn't a
// multiple of target.
func fractionalDaySlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
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

// wholeDaySlotDeadline mirrors league.SlotDeadline / league.JornadaWindow's
// whole-day ceil-division partition — kept self-contained since migrations
// must not depend on application code that can change shape after they run.
func wholeDaySlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
	target := comp.GetInt("target_matches")
	if target <= 0 || slot <= 0 {
		return time.Time{}, false
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	sy, sm, sd := start.UTC().Date()
	ey, em, ed := end.UTC().Date()
	startDay := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	endDay := time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC)
	days := int(endDay.Sub(startDay).Hours()/24) + 1
	if days <= 0 {
		return time.Time{}, false
	}
	hiDay := ceilDivWholeDay(slot*days, target) - 1
	loDay := ceilDivWholeDay((slot-1)*days, target)
	if hiDay < loDay {
		hiDay = loDay
	}
	deadline := startDay.AddDate(0, 0, hiDay)
	y, mo, d := deadline.Date()
	return time.Date(y, mo, d, 12, 0, 0, 0, time.UTC), true
}

// ceilDivWholeDay returns ceil(a/b) for non-negative a and positive b.
func ceilDivWholeDay(a, b int) int {
	return (a + b - 1) / b
}
