// Package main is the entry point for the PadelLeague server.
package main

import (
	"embed"
	"log"
	"log/slog"
	"sync/atomic"
	_ "time/tzdata"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

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

	notify.SetDevEnv(cfg.AppEnv != "prod")
	r := render.New(viewsFS, cfg.VAPIDPublicKey, cfg.AppDevTools)
	notifier := notify.NewNotifier(app, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey)
	leagueSvc := league.New(app, notifier)

	searchIndex := &search.Index{}
	hooks.Register(app, hooks.Deps{
		Svc:         leagueSvc,
		Notifier:    notifier,
		SearchIndex: searchIndex,
		Backup: hooks.BackupConfig{
			Email:         cfg.BackupEmail,
			EncryptionKey: cfg.BackupEncryptionKey,
			IntervalHours: cfg.BackupIntervalHours,
		},
		SMTP: hooks.SMTPConfig{
			Host:       cfg.SMTPHost,
			Port:       cfg.SMTPPort,
			Username:   cfg.SMTPUser,
			Password:   cfg.SMTPPass,
			TLS:        cfg.SMTPTLS,
			Sender:     cfg.SMTPSender,
			SenderName: cfg.SMTPSenderName,
			AppURL:     cfg.AppURL,
		},
	})

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
			AppEnv:      cfg.AppEnv,
			Version:     Version,
		})
		return se.Next()
	})

	if err := start(app); err != nil {
		log.Fatal(err)
	}
}

// start is app.Start that also returns the error of the command that ran.
// pocketbase.Execute discards it, so a failed `serve` (e.g. "bind: address
// already in use") exited 0 and looked like a clean stop.
func start(app *pocketbase.PocketBase) error {
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))
	// true mirrors Start's !hideStartBanner under pocketbase.New()'s default config.
	app.RootCmd.AddCommand(cmd.NewServeCommand(app, true))
	return execute(app)
}

// execute runs app's root command and returns the first error a RunE
// returned (Execute's own error wins: it means bootstrap failed).
// Execute runs the command in its own goroutine and also returns on
// SIGINT/SIGTERM while a RunE may still be running, so the error slot is
// atomic, and a RunE that ends after that return is not seen. A graceful
// shutdown is nil anyway: serve filters http.ErrServerClosed
// (apis/serve.go and cmd/serve.go).
func execute(app *pocketbase.PocketBase) error {
	var runErr atomic.Pointer[error]
	captureRunErrors(app.RootCmd, &runErr)
	if err := app.Execute(); err != nil {
		return err
	}
	if err := runErr.Load(); err != nil {
		return *err
	}
	return nil
}

func captureRunErrors(c *cobra.Command, runErr *atomic.Pointer[error]) {
	if run := c.RunE; run != nil {
		c.RunE = func(c *cobra.Command, args []string) error {
			err := run(c, args)
			if err != nil {
				runErr.CompareAndSwap(nil, &err)
			}
			return err
		}
	}
	for _, sub := range c.Commands() {
		captureRunErrors(sub, runErr)
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
