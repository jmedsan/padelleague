package hooks

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSMTP_DisabledWithoutHost(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	before := app.Settings().SMTP.Enabled

	registerSMTP(app, SMTPConfig{})
	require.NoError(t, app.OnServe().Trigger(&core.ServeEvent{App: app}, func(*core.ServeEvent) error { return nil }))

	assert.Equal(t, before, app.Settings().SMTP.Enabled, "SMTP settings must be untouched without a host")
}

func TestRegisterSMTP_AppliesSettingsOnServe(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerSMTP(app, SMTPConfig{
		Host:       "smtp.example.com",
		Port:       465,
		Username:   "bot@example.com",
		Password:   "secret",
		TLS:        true,
		Sender:     "no-reply@example.com",
		SenderName: "Padel League Test",
		AppURL:     "https://padel.example.com",
	})
	require.NoError(t, app.OnServe().Trigger(&core.ServeEvent{App: app}, func(*core.ServeEvent) error { return nil }))

	s := app.Settings()
	assert.True(t, s.SMTP.Enabled)
	assert.Equal(t, "smtp.example.com", s.SMTP.Host)
	assert.Equal(t, 465, s.SMTP.Port)
	assert.Equal(t, "bot@example.com", s.SMTP.Username)
	assert.Equal(t, "secret", s.SMTP.Password)
	assert.True(t, s.SMTP.TLS)
	assert.Equal(t, "PLAIN", s.SMTP.AuthMethod)
	assert.Equal(t, "no-reply@example.com", s.Meta.SenderAddress)
	assert.Equal(t, "Padel League Test", s.Meta.SenderName)
	assert.Equal(t, "https://padel.example.com", s.Meta.AppURL)
}

func TestRegisterSMTP_EmptyAppURLLeavesExistingValue(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	s := app.Settings()
	s.Meta.AppURL = "https://existing.example.com"
	require.NoError(t, app.Save(s))

	registerSMTP(app, SMTPConfig{Host: "smtp.example.com", Port: 587, Sender: "a@example.com", SenderName: "A"})
	require.NoError(t, app.OnServe().Trigger(&core.ServeEvent{App: app}, func(*core.ServeEvent) error { return nil }))

	assert.Equal(t, "https://existing.example.com", app.Settings().Meta.AppURL, "empty AppURL config must not overwrite an existing value")
}
