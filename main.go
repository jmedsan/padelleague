// Package main is the entry point for the PadelLeague server.
package main

import (
	"embed"
	"log"
	"log/slog"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"padelleague/config"
	"padelleague/hooks"
	"padelleague/league"
	_ "padelleague/migrations"
	"padelleague/notify"
	"padelleague/render"
	"padelleague/routes"
	"padelleague/seed"
)

//go:embed all:views
var viewsFS embed.FS

//go:embed all:static/*
var staticFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := pocketbase.New()

	r := render.New(viewsFS, cfg.VAPIDPublicKey)
	notifier := notify.NewNotifier(app, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey)
	leagueSvc := league.New(app, notifier)

	hooks.Register(app, leagueSvc, notifier)

	slog.Info("startup",
		"app_env", cfg.AppEnv,
		"push_enabled", cfg.VAPIDPublicKey != "",
		"seed_users", len(seedUsers(cfg)),
	)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		seed.Run(app, seedUsers(cfg))
		routes.Register(se, routes.Deps{
			App:       app,
			Renderer:  r,
			Notifier:  notifier,
			LeagueSvc: leagueSvc,
			StaticFS:  staticFS,
			AppEnv:    cfg.AppEnv,
		})
		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func seedUsers(cfg config.Config) []seed.User {
	users := []seed.User{
		{Email: cfg.PBAdminEmail, Password: cfg.PBAdminPassword, Collection: core.CollectionNameSuperusers},
		{Email: cfg.AppAdmin1Email, Password: cfg.AppAdmin1Password, Collection: "users", Role: "admin", DisplayName: "Javi"},
	}
	if cfg.AppAdmin2Email != "" {
		users = append(users, seed.User{Email: cfg.AppAdmin2Email, Password: cfg.AppAdmin2Password, Collection: "users", Role: "admin", DisplayName: "Luis"})
	}
	if cfg.AppEnv != "prod" {
		users = append(users, seed.User{Email: cfg.AppPlayerEmail, Password: cfg.AppPlayerPassword, Collection: "users", Role: "player", DisplayName: "Javi"})
		if cfg.AppPlayer2Email != "" {
			users = append(users, seed.User{Email: cfg.AppPlayer2Email, Password: cfg.AppPlayer2Password, Collection: "users", Role: "player", DisplayName: "Carlos"})
		}
	}
	return users
}
