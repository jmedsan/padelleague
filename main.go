package main

import (
	"embed"
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"padelleague/config"
	"padelleague/handlers"
	"padelleague/hooks"
	_ "padelleague/migrations"
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
	notifier := handlers.NewNotifier(app, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey)

	hooks.Register(app, notifier)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		seed.Run(app, seedUsers(cfg))
		routes.Register(app, se, r, notifier, staticFS)
		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func seedUsers(cfg config.Config) []seed.User {
	return []seed.User{
		{Email: cfg.PBAdminEmail, Password: cfg.PBAdminPassword, Collection: core.CollectionNameSuperusers},
		{Email: cfg.AppAdminEmail, Password: cfg.AppAdminPassword, Collection: "users", Role: "admin", DisplayName: "Admin"},
		{Email: cfg.AppPlayerEmail, Password: cfg.AppPlayerPassword, Collection: "users", Role: "player", DisplayName: "Javi"},
	}
}
