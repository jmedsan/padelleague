// Package notify handles notification delivery via push, in-app, and email.
package notify

import (
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"padelleague/league"
)

// IsMailerConfigured reports whether SMTP is enabled and configured.
func IsMailerConfigured(app core.App) bool {
	return app.Settings().SMTP.Enabled && app.Settings().SMTP.Host != ""
}

// devEnv is set once at startup via SetDevEnv. When true, emails get a
// "[TEST]" subject prefix and a red test banner.
var devEnv bool

// SetDevEnv stores whether the app is running in dev mode. Call once at
// startup from main; tests leave it at the default (false).
func SetDevEnv(dev bool) { devEnv = dev }

// SubjectPrefix returns "[TEST] " in dev, empty in prod.
func SubjectPrefix() string {
	if devEnv {
		return "[TEST] "
	}
	return ""
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
	subject = SubjectPrefix() + subject

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
		if !user.Verified() {
			slog.Info("skip email to unverified user", "to", maskEmail(email))
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

// ctaHTML returns a table-based CTA button that renders in all email
// clients including Outlook (which ignores display:inline-block on anchors).
func ctaHTML(href, label string) string {
	return fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:16px auto;"><tr><td align="center" bgcolor="#b5d334" style="border-radius:8px;">
<a href="%s" style="display:inline-block;padding:12px 24px;font-family:Arial,Helvetica,sans-serif;font-size:15px;font-weight:bold;color:#0b0b0b;text-decoration:none;border-radius:8px;">%s</a></td></tr></table>`,
		href, html.EscapeString(label))
}

// CtaHTML is the exported version for use in hooks/mailer.go.
func CtaHTML(href, label string) string { return ctaHTML(href, label) }

// BuildNotificationEmail returns the HTML body for a notification email.
func BuildNotificationEmail(displayName, body, link string) string {
	linkHTML := ""
	if link != "" {
		label := "Ver partido"
		if strings.Contains(link, "/competition/") {
			label = "Ver competición"
		}
		linkHTML = ctaHTML(link, label)
	}
	return fmt.Sprintf(`<p>Hola %s,</p>
<p>%s</p>
%s`, html.EscapeString(displayName), html.EscapeString(body), linkHTML)
}

// RenderEmail wraps bodyHTML in the league's branded email shell.
func RenderEmail(app core.App, compID, bodyHTML string) string {
	branding := league.Branding(app, compID)
	baseURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")

	leagueName := branding.Name
	logoURL := absoluteURL(baseURL, branding.LogoURL)

	compLine := ""
	if branding.Competition != nil {
		compLine = fmt.Sprintf(`<p style="margin:6px 0 0;color:#e2e8f0;font-size:14px;">%s</p>`,
			html.EscapeString(branding.Competition.Name))
		if compLogo := absoluteURL(baseURL, branding.Competition.LogoURL); compLogo != "" {
			logoURL = compLogo
		}
	}

	testBanner := ""
	if devEnv {
		testBanner = `<tr><td style="background:#e53e3e;padding:8px;text-align:center;color:#ffffff;font-size:13px;font-weight:bold;letter-spacing:1px;">ENTORNO DE PRUEBAS — ESTE EMAIL NO ES REAL</td></tr>
`
	}

	return fmt.Sprintf(`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="es">
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<meta name="color-scheme" content="light" />
<meta name="supported-color-schemes" content="light" />
</head>
<body style="margin:0;padding:0;background:#f2f2f2;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#f2f2f2;padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:600px;max-width:100%%;background:#ffffff;border-radius:8px;overflow:hidden;font-family:Arial,Helvetica,sans-serif;">
%s<tr><td style="background:#0b0b0b;padding:24px;text-align:center;">
%s
%s
%s
</td></tr>
<tr><td style="padding:24px;color:#1a1a1a;font-size:15px;line-height:1.5;">
%s
</td></tr>
%s
<tr><td style="padding:16px 24px;background:#f2f2f2;text-align:center;color:#666666;font-size:12px;">
<p style="margin:0 0 6px;">%s — <a href="%s" style="color:#666666;">%s</a></p>
<p style="margin:0;font-size:11px;color:#999999;">Recibes este email porque tienes cuenta en %s</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`,
		testBanner,
		logoImgHTML(logoURL, leagueName),
		wordmarkHTML(leagueName),
		taglineHTML(branding.Tagline)+compLine,
		bodyHTML,
		sponsorSectionHTML(baseURL, branding.Sponsors),
		html.EscapeString(leagueName), baseURL, baseURL,
		html.EscapeString(leagueName))
}

// wordmarkHTML renders the league name as the site wordmark: first word
// in green (#b5d334), rest in white, all uppercase italic bold.
func wordmarkHTML(name string) string {
	first, rest, _ := strings.Cut(name, " ")
	if rest == "" {
		return fmt.Sprintf(`<h1 style="margin:12px 0 0;font-family:Arial Black,Arial,Helvetica,sans-serif;font-weight:900;font-style:italic;text-transform:uppercase;letter-spacing:-0.5px;font-size:24px;line-height:28px;color:#ffffff;">%s</h1>`,
			html.EscapeString(first))
	}
	return fmt.Sprintf(`<h1 style="margin:12px 0 0;font-family:Arial Black,Arial,Helvetica,sans-serif;font-weight:900;font-style:italic;text-transform:uppercase;letter-spacing:-0.5px;font-size:24px;line-height:28px;color:#ffffff;"><span style="color:#b5d334;">%s</span> %s</h1>`,
		html.EscapeString(first), html.EscapeString(rest))
}

// taglineHTML renders the tagline in gray uppercase with wide letter spacing.
func taglineHTML(tagline string) string {
	if tagline == "" {
		return ""
	}
	return fmt.Sprintf(`<p style="margin:6px 0 0;color:#a3a3a3;font-size:12px;line-height:16px;text-transform:uppercase;letter-spacing:3px;">%s</p>`,
		html.EscapeString(tagline))
}

// logoImgHTML returns an <img> tag for the branding logo with explicit
// dimensions so the layout holds when images are blocked.
func logoImgHTML(logoURL, name string) string {
	if logoURL == "" {
		return ""
	}
	return fmt.Sprintf(`<img src="%s" alt="%s" width="96" height="96" style="display:block;width:96px;height:96px;object-fit:contain;border:0;margin:0 auto 12px;border-radius:12px;">`,
		logoURL, html.EscapeString(name))
}

// sponsorSectionHTML returns a table row with the sponsor logos.
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
		alt := "Patrocinador: " + html.EscapeString(s.Name)
		img := fmt.Sprintf(
			`<img src="%s" alt="%s" height="32" style="height:32px;width:auto;display:inline-block;vertical-align:middle;margin:0 10px;border:0;">`,
			logoURL, alt)
		if s.URL != "" {
			img = fmt.Sprintf(`<a href="%s" style="text-decoration:none;">%s</a>`, s.URL, img)
		}
		logos.WriteString(img)
	}
	return fmt.Sprintf(`<tr><td style="padding:0 24px 24px;text-align:center;border-top:1px solid #eeeeee;">
<p style="margin:16px 0 8px;color:#666666;font-size:11px;text-transform:uppercase;letter-spacing:2px;">%s</p>
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

// EscapeToken URL-encodes a token for use in email links.
func EscapeToken(token string) string {
	return url.QueryEscape(token)
}
