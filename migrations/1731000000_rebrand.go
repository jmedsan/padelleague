package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()
		settings.Meta.AppName = "Dale Fuerte a la Bola"
		return app.Save(settings)
	}, func(app core.App) error {
		settings := app.Settings()
		settings.Meta.AppName = "PadelLeague"
		return app.Save(settings)
	})
}
