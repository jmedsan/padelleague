// Package notify handles notification delivery via push, in-app, and email.
package notify

import (
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

// IsMailerConfigured reports whether SMTP is enabled and configured.
func IsMailerConfigured(app core.App) bool {
	return app.Settings().SMTP.Enabled && app.Settings().SMTP.Host != ""
}

// SendEmail sends an HTML email via the configured SMTP mailer.
func SendEmail(app core.App, to, subject, htmlBody string) {
	if !IsMailerConfigured(app) {
		return
	}

	client := app.NewMailClient()
	msg := &mailer.Message{
		From: mail.Address{
			Name:    app.Settings().Meta.AppName,
			Address: app.Settings().Meta.SenderAddress,
		},
		To:      []mail.Address{{Address: to}},
		Subject: subject,
		HTML:    htmlBody,
	}
	if err := client.Send(msg); err != nil {
		slog.Error("send email failed", "to", maskEmail(to), "err", err)
	}
}

// EmailPlayers sends a notification email to each player in the list,
// delegating to EmailNotifyPlayers using the Notifier's app.
func (n *Notifier) EmailPlayers(playerUserIDs []string, subject, body, link string) {
	EmailNotifyPlayers(n.app, playerUserIDs, subject, body, link)
}

// EmailNotifyPlayers sends a notification email to each configured player.
func EmailNotifyPlayers(app core.App, playerUserIDs []string, subject, body, matchLink string) {
	if !IsMailerConfigured(app) {
		return
	}

	for _, userID := range playerUserIDs {
		user, err := app.FindRecordById("users", userID)
		if err != nil {
			continue
		}

		email := user.Email()
		if email == "" {
			continue
		}

		displayName := user.GetString("display_name")
		htmlBody := BuildNotificationEmail(displayName, body, matchLink)
		SendEmail(app, email, subject, htmlBody)
	}
}

func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	prefix := email[:1]
	if at > 1 {
		prefix = email[:2]
	}
	return prefix + "***" + email[at:]
}

// BuildNotificationEmail returns the HTML body for a notification email.
func BuildNotificationEmail(displayName, body, matchLink string) string {
	linkHTML := ""
	if matchLink != "" {
		linkHTML = fmt.Sprintf(`<p><a href="%s">Ver partido</a></p>`, matchLink)
	}
	return fmt.Sprintf(`<h2>PadelLeague</h2>
<p>Hola %s,</p>
<p>%s</p>
%s
<p>— PadelLeague</p>`, html.EscapeString(displayName), html.EscapeString(body), linkHTML)
}
