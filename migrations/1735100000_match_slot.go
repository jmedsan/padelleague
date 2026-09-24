package migrations

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds matches.slot (per-pair ordinal for leveled-league date-range display)
// and backfills it for existing leveled matches.
func init() {
	m.Register(func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.Add(&core.NumberField{Name: "slot", Min: floatPtr(0)})
		if err := app.Save(matches); err != nil {
			return err
		}
		return backfillMatchSlots(app)
	}, func(app core.App) error {
		matches, err := app.FindCollectionByNameOrId("matches")
		if err != nil {
			return err
		}
		matches.Fields.RemoveByName("slot")
		return app.Save(matches)
	})
}

// backfillMatchSlots assigns a per-pair ordinal slot to every match of every
// leveled competition, ordered by creation time, and recomputes arrange_by
// for non-final matches when the competition has start/end dates.
func backfillMatchSlots(app core.App) error {
	comps, err := app.FindRecordsByFilter("competitions", "target_matches > 0", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, comp := range comps {
		if !isLeveledComp(comp) {
			continue
		}
		if err := backfillCompetitionSlots(app, comp); err != nil {
			return err
		}
	}
	return nil
}

func isLeveledComp(comp *core.Record) bool {
	target := comp.GetInt("target_matches")
	pairs := comp.GetStringSlice("pairs")
	return target > 0 && target < len(pairs)-1
}

func backfillCompetitionSlots(app core.App, comp *core.Record) error {
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp}", "created", 0, 0, map[string]any{"comp": comp.Id})
	if err != nil {
		return err
	}
	counter := map[string]int{}
	for _, rec := range matches {
		p1, p2 := rec.GetString("pair1"), rec.GetString("pair2")
		slot := max(counter[p1], counter[p2]) + 1
		counter[p1], counter[p2] = slot, slot
		rec.Set("slot", slot)
		if rec.GetString("status") != "final" {
			if deadline, ok := backfillSlotDeadline(comp, slot); ok {
				rec.Set("arrange_by", deadline.Format("2006-01-02"))
			}
		}
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// backfillSlotDeadline mirrors league.slotDeadline (added in a later
// migration-independent change); kept self-contained since migrations must
// not depend on application code that can change shape after they run.
func backfillSlotDeadline(comp *core.Record, slot int) (time.Time, bool) {
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
