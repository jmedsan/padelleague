// Package main is the entry point for the PadelLeague server.
package main

import (
	"embed"
	"log"
	"log/slog"
	_ "time/tzdata"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"padelleague/config"
	"padelleague/hooks"
	"padelleague/league"
	_ "padelleague/migrations"
	"padelleague/notify"
	"padelleague/render"
	"padelleague/routes"
	"padelleague/search"
	"padelleague/seed"
)

var Version = "dev"

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

	r := render.New(viewsFS, cfg.VAPIDPublicKey, cfg.AppDevTools)
	notifier := notify.NewNotifier(app, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey)
	leagueSvc := league.New(app, notifier)

	searchIndex := &search.Index{}
	hooks.Register(app, leagueSvc, notifier, searchIndex)

	slog.Info("startup",
		"app_env", cfg.AppEnv,
		"dev_tools", cfg.AppDevTools,
		"push_enabled", cfg.VAPIDPublicKey != "",
		"seed_users", len(seedUsers(cfg)),
	)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		seed.Run(app, seedUsers(cfg))
		routes.Register(se, routes.Deps{
			App:         app,
			Renderer:    r,
			Notifier:    notifier,
			LeagueSvc:   leagueSvc,
			SearchIndex: searchIndex,
			StaticFS:    staticFS,
			AppDevTools: cfg.AppDevTools,
			Version:     Version,
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
		{Email: cfg.AppAdmin1Email, Password: cfg.AppAdmin1Password, Collection: "users", Roles: []string{"admin", "player"}, DisplayName: cfg.AppAdmin1Name, Gender: "male"},
	}
	if cfg.AppAdmin2Email != "" {
		users = append(users, seed.User{Email: cfg.AppAdmin2Email, Password: cfg.AppAdmin2Password, Collection: "users", Roles: []string{"admin", "player"}, DisplayName: cfg.AppAdmin2Name, Gender: "male"})
	}
	if cfg.AppEnv != "prod" {
		users = append(users, seed.User{Email: cfg.AppPlayerEmail, Password: cfg.AppPlayerPassword, Collection: "users", Roles: []string{"player"}, DisplayName: cfg.AppPlayerName, Gender: "male"})
		if cfg.AppPlayer2Email != "" {
			users = append(users, seed.User{Email: cfg.AppPlayer2Email, Password: cfg.AppPlayer2Password, Collection: "users", Roles: []string{"player"}, DisplayName: cfg.AppPlayer2Name, Gender: "male"})
		}
	}
	return users
}
