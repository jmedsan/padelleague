package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Backfills matches.submitted_at. The handlers set it with Set, a no-op on an
// autodate field, so it kept the match creation time; the quorum timer and the
// 24-hour correction window read it as the submission time. It becomes the
// creation time of the match's latest result proposal. Irreversible: the old
// values were the creation times, not data.
func init() {
	m.Register(backfillSubmittedAt, func(_ core.App) error { return nil })
}

func backfillSubmittedAt(app core.App) error {
	matches, err := app.FindRecordsByFilter("matches", "submitted_by != ''", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, match := range matches {
		latest, err := app.FindRecordsByFilter("match_messages",
			"match = {:m} && type = 'result_submission'", "-created", 1, 0,
			dbx.Params{"m": match.Id})
		if err != nil {
			return err
		}
		if len(latest) == 0 {
			continue
		}
		match.SetRaw("submitted_at", latest[0].GetDateTime("created")) // autodate: Set is a no-op
		if err := app.Save(match); err != nil {
			return err
		}
	}
	return nil
}
