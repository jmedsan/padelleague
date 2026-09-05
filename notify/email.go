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

	"padelleague/league"
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
			Name:    app.Settings().Meta.SenderName,
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
		htmlBody := RenderEmail(n.app, "", BuildNotificationEmail(displayName, body, link))
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
	return fmt.Sprintf(`<p>Hola %s,</p>
<p>%s</p>
%s`, html.EscapeString(displayName), html.EscapeString(body), linkHTML)
}

// RenderEmail wraps bodyHTML in the league's branded email shell: a dark
// header with the league (or competition) logo, name, and tagline; the
// white body; a sponsor section when any apply; and a footer linking back
// to the app. compID scopes the branding to a competition when non-empty,
// falling back to the league-wide defaults and global sponsors when empty.
func RenderEmail(app core.App, compID, bodyHTML string) string {
	branding := league.Branding(app, compID)
	baseURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")

	name := branding.Name
	logoURL := absoluteURL(baseURL, branding.LogoURL)
	if branding.Competition != nil {
		name = branding.Competition.Name
		if compLogo := absoluteURL(baseURL, branding.Competition.LogoURL); compLogo != "" {
			logoURL = compLogo
		}
	}

	return fmt.Sprintf(`<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f2f2f2;padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:600px;max-width:100%%;background:#ffffff;border-radius:8px;overflow:hidden;font-family:Arial,Helvetica,sans-serif;">
<tr><td style="background:#0b0b0b;padding:24px;text-align:center;">
%s
<h1 style="margin:12px 0 0;color:#ffffff;font-size:20px;">%s</h1>
<p style="margin:4px 0 0;color:#b5d334;font-size:13px;">%s</p>
</td></tr>
<tr><td style="padding:24px;color:#1a1a1a;font-size:15px;line-height:1.5;">
%s
</td></tr>
%s
<tr><td style="padding:16px 24px;background:#f2f2f2;text-align:center;color:#666666;font-size:12px;">
<p style="margin:0;">%s — <a href="%s" style="color:#666666;">%s</a></p>
</td></tr>
</table>
</td></tr>
</table>`,
		logoImgHTML(logoURL, name),
		html.EscapeString(name),
		html.EscapeString(branding.Tagline),
		bodyHTML,
		sponsorSectionHTML(baseURL, branding.Sponsors),
		html.EscapeString(name), baseURL, baseURL)
}

// logoImgHTML returns an <img> tag for the branding logo, or "" if logoURL
// is empty (no fallback image is inserted).
func logoImgHTML(logoURL, name string) string {
	if logoURL == "" {
		return ""
	}
	return fmt.Sprintf(`<img src="%s" alt="%s" width="96" style="max-width:96px;height:auto;">`,
		logoURL, html.EscapeString(name))
}

// sponsorSectionHTML returns a table row with the sponsor logos, or "" when
// sponsors is empty. The heading is singular/plural depending on count.
func sponsorSectionHTML(baseURL string, sponsors []league.FooterSponsor) string {
	if len(sponsors) == 0 {
		return ""
	}
	heading := "Patrocinado por"
	if len(sponsors) > 1 {
		heading = "Patrocinadores"
	}
	var logos strings.Builder
	for _, s := range sponsors {
		logoURL := absoluteURL(baseURL, s.LogoURL)
		if logoURL == "" {
			continue
		}
		fmt.Fprintf(&logos,
			`<img src="%s" alt="%s" height="32" style="max-height:32px;width:auto;margin:0 8px;">`,
			logoURL, html.EscapeString(s.Name))
	}
	return fmt.Sprintf(`<tr><td style="padding:0 24px 24px;text-align:center;border-top:1px solid #eeeeee;">
<p style="margin:16px 0 8px;color:#666666;font-size:12px;text-transform:uppercase;">%s</p>
%s
</td></tr>`, heading, logos.String())
}

// absoluteURL joins baseURL with a relative path, or returns "" when path
// is empty. Already-absolute paths are returned unchanged.
func absoluteURL(baseURL, path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return baseURL + path
}
