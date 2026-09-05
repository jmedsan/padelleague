package hooks

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// SMTPConfig configures outgoing mail (verification/reset emails, admin
// notifications) via environment variables instead of the admin dashboard.
// An empty Host leaves whatever SMTP settings are already configured
// through the dashboard untouched, so local dev without these env vars
// still works.
type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	TLS        bool
	Sender     string
	SenderName string
	AppURL     string
}

// registerSMTP applies cfg to the app's mail settings on serve, when Host is
// set.
func registerSMTP(app core.App, cfg SMTPConfig) {
	if cfg.Host == "" {
		slog.Info("startup", "smtp", "not configured")
		return
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		s := app.Settings()
		s.SMTP.Enabled = true
		s.SMTP.Host = cfg.Host
		s.SMTP.Port = cfg.Port
		s.SMTP.Username = cfg.Username
		s.SMTP.Password = cfg.Password
		s.SMTP.TLS = cfg.TLS
		s.SMTP.AuthMethod = "PLAIN"
		s.Meta.SenderAddress = cfg.Sender
		s.Meta.SenderName = cfg.SenderName
		if cfg.AppURL != "" {
			s.Meta.AppURL = cfg.AppURL
		}
		if err := app.Save(s); err != nil {
			slog.Error("smtp: failed to save settings", "err", err)
			return e.Next()
		}
		return e.Next()
	})

	slog.Info("startup", "smtp", cfg.Host, "sender", cfg.Sender)
}
