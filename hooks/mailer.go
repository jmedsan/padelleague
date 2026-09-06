package hooks

import (
	"fmt"
	"html"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/notify"
)

// registerMailerBranding intercepts PocketBase's system emails (password
// reset, verification, email change) and replaces the default English
// templates with branded Spanish versions wrapped in the league's email
// shell (dark header, logo, sponsors, footer).
func registerMailerBranding(app core.App) {
	app.OnMailerRecordPasswordResetSend().BindFunc(func(e *core.MailerRecordEvent) error {
		token, _ := e.Meta["token"].(string)
		appURL := app.Settings().Meta.AppURL
		name := html.EscapeString(e.Record.GetString("display_name"))

		body := fmt.Sprintf(`<p>Hola %s,</p>
<p>Haz clic en el botón para restablecer tu contraseña.</p>
%s
<p><small>Si no solicitaste este cambio, ignora este email.</small></p>`,
			name, notify.CtaHTML(appURL+"/reset-password?token="+notify.EscapeToken(token), "Restablecer contraseña"))

		e.Message.Subject = notify.SubjectPrefix() + "Restablecer contraseña — Liga Dale Fuerte"
		e.Message.HTML = notify.RenderEmail(app, "", body)
		return e.Next()
	})

	app.OnMailerRecordVerificationSend().BindFunc(func(e *core.MailerRecordEvent) error {
		token, _ := e.Meta["token"].(string)
		appURL := app.Settings().Meta.AppURL
		name := html.EscapeString(e.Record.GetString("display_name"))

		body := fmt.Sprintf(`<p>Hola %s,</p>
<p>Confirma tu dirección de email para recibir notificaciones de tus partidos.</p>
%s
<p><small>Si no te registraste recientemente, ignora este email.</small></p>`,
			name, notify.CtaHTML(appURL+"/verify?token="+notify.EscapeToken(token), "Confirmar email"))

		e.Message.Subject = notify.SubjectPrefix() + "Confirma tu email — Liga Dale Fuerte"
		e.Message.HTML = notify.RenderEmail(app, "", body)
		return e.Next()
	})

	app.OnMailerRecordEmailChangeSend().BindFunc(func(e *core.MailerRecordEvent) error {
		token, _ := e.Meta["token"].(string)
		appURL := app.Settings().Meta.AppURL
		name := html.EscapeString(e.Record.GetString("display_name"))

		body := fmt.Sprintf(`<p>Hola %s,</p>
<p>Haz clic en el botón para confirmar tu nueva dirección de email.</p>
%s
<p><small>Si no solicitaste este cambio, ignora este email.</small></p>`,
			name, notify.CtaHTML(appURL+"/verify?token="+notify.EscapeToken(token), "Confirmar nuevo email"))

		e.Message.Subject = notify.SubjectPrefix() + "Confirmar nuevo email — Liga Dale Fuerte"
		e.Message.HTML = notify.RenderEmail(app, "", body)
		return e.Next()
	})
}
