package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	queryCompetitionsQuorum24h = `UPDATE competitions SET quorum_timeout_hours = 24 WHERE quorum_timeout_hours = 48`
	querySettingsQuorum24h     = `UPDATE app_settings SET quorum_timeout_hours = 24 WHERE quorum_timeout_hours = 48`
)

// Lowers the result auto-confirm time from the old 48h default to 24h, on
// every competition still at 48h and on the default for new competitions.
// Other values were chosen by an admin and stay. Irreversible: a 24h value
// set before this migration cannot be told apart.
func init() {
	m.Register(quorumTimeout24h, func(_ core.App) error { return nil })
}

func quorumTimeout24h(app core.App) error {
	if _, err := app.DB().NewQuery(queryCompetitionsQuorum24h).Execute(); err != nil {
		return err
	}
	_, err := app.DB().NewQuery(querySettingsQuorum24h).Execute()
	return err
}
