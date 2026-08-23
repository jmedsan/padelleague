package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Enabled = true
		settings.RateLimits.Rules = append(settings.RateLimits.Rules, core.RateLimitRule{
			Label:       "POST /login",
			MaxRequests: 20,
			Duration:    60,
		})
		return app.Save(settings)
	}, func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Enabled = false
		settings.RateLimits.Rules = nil
		return app.Save(settings)
	})
}
