package handlers

import (
	"fmt"
	"log"
	"net/mail"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

func IsMailerConfigured(app core.App) bool {
	return app.Settings().SMTP.Enabled && app.Settings().SMTP.Host != ""
}

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
		log.Printf("failed to send email to %s: %v", to, err)
	}
}

func emailNotifyPlayers(app core.App, playerUserIDs []string, subject, body, matchLink string) {
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
		html := buildNotificationEmail(displayName, body, matchLink)
		SendEmail(app, email, subject, html)
	}
}

func buildNotificationEmail(displayName, body, matchLink string) string {
	linkHTML := ""
	if matchLink != "" {
		linkHTML = fmt.Sprintf(`<p><a href="%s">Ver partido</a></p>`, matchLink)
	}
	return fmt.Sprintf(`<h2>PadelLeague</h2>
<p>Hola %s,</p>
<p>%s</p>
%s
<p>— PadelLeague</p>`, displayName, body, linkHTML)
}
