package migrations

import (
	"encoding/json"
	"time"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		comps.Fields.Add(&core.JSONField{Name: "round_arrange_dates"})
		if err := app.Save(comps); err != nil {
			return err
		}

		allComps, _ := app.FindRecordsByFilter("competitions", "type != 'playoff'", "", 0, 0, nil)
		for _, c := range allComps {
			matches, _ := app.FindRecordsByFilter("matches",
				"competition = {:cid}", "", 0, 0,
				map[string]any{"cid": c.Id})
			if len(matches) == 0 {
				continue
			}

			maxRound := 0
			for _, mv := range matches {
				if rn := mv.GetInt("round_number"); rn > maxRound {
					maxRound = rn
				}
			}
			c.Set("rounds", maxRound)

			start := c.GetDateTime("start_date").Time()
			end := c.GetDateTime("end_date").Time()
			if !start.IsZero() && !end.IsZero() && maxRound > 0 {
				sched := make(map[int]time.Time, maxRound)
				for r := 1; r <= maxRound; r++ {
					frac := float64(r) / float64(maxRound)
					sched[r] = start.Add(time.Duration(float64(end.Sub(start)) * frac))
				}
				if b, err := json.Marshal(sched); err == nil {
					c.Set("round_arrange_dates", string(b))
				}
			}

			if err := app.Save(c); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		comps, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return nil
		}
		comps.Fields.RemoveByName("round_arrange_dates")
		return app.Save(comps)
	})
}
