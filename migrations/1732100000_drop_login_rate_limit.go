package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drops the PocketBase "POST /login" rate-limit rule. PocketBase keys its
// limiter by RemoteIP, which behind the Northflank proxy chain is the ingress
// address for every client, so the rule throttled all users as one bucket.
// The login route now uses middleware.LimitByClientIP keyed by the real
// client address.
func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Rules = slices.DeleteFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
			return r.Label == "POST /login"
		})
		return app.Save(settings)
	}, func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Rules = append(settings.RateLimits.Rules, core.RateLimitRule{
			Label:       "POST /login",
			MaxRequests: 5,
			Duration:    60,
		})
		return app.Save(settings)
	})
}
