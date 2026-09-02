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

// EmailPlayers sends a notification email to each player in the list.
func (n *Notifier) EmailPlayers(playerUserIDs []string, subject, body, link string) {
	if !IsMailerConfigured(n.app) {
		return
	}

	if strings.HasPrefix(link, "/") {
		if baseURL := strings.TrimRight(n.app.Settings().Meta.AppURL, "/"); baseURL != "" {
			link = baseURL + link
		}
	}

	for _, userID := range playerUserIDs {
		user, err := n.app.FindRecordById("users", userID)
		if err != nil {
			continue
		}

		email := user.Email()
		if email == "" {
			continue
		}

		displayName := user.GetString("display_name")
		htmlBody := BuildNotificationEmail(displayName, body, link)
		SendEmail(n.app, email, subject, htmlBody)
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
func BuildNotificationEmail(displayName, body, link string) string {
	linkHTML := ""
	if link != "" {
		label := "Ver partido"
		if strings.HasPrefix(link, "/competition/") || strings.Contains(link, "/competition/") {
			label = "Ver competición"
		}
		linkHTML = fmt.Sprintf(`<p><a href="%s">%s</a></p>`, link, label)
	}
	return fmt.Sprintf(`<h2>PadelLeague</h2>
<p>Hola %s,</p>
<p>%s</p>
%s
<p>— PadelLeague</p>`, html.EscapeString(displayName), html.EscapeString(body), linkHTML)
}
