package handlers

import (
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"

	"padelleague/middleware"
	"padelleague/notify"
)

// PasswordResetHandler handles forgot-password and reset-password flows.
type PasswordResetHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewPasswordResetHandler creates a PasswordResetHandler with the given dependencies.
func NewPasswordResetHandler(app core.App, renderPage RenderFunc) *PasswordResetHandler {
	return &PasswordResetHandler{app: app, renderPage: renderPage}
}

// ForgotPassword renders the forgot-password form.
func (h *PasswordResetHandler) ForgotPassword(e *core.RequestEvent) error {
	return h.renderPage(e, "forgot-password.html", map[string]any{"PageTitle": "Contraseña"})
}

// ForgotPasswordSubmit sends a password-reset email if the address exists.
func (h *PasswordResetHandler) ForgotPasswordSubmit(e *core.RequestEvent) error {
	email := e.Request.FormValue("email")

	if !notify.IsMailerConfigured(h.app) {
		return e.HTML(http.StatusOK, `<div class="alert alert-info">SMTP no configurado. Contacta al administrador para restablecer tu contraseña.</div>`)
	}

	if email != "" {
		record, err := h.app.FindAuthRecordByEmail("users", email)
		if err == nil && record != nil {
			if err := mails.SendRecordPasswordReset(h.app, record); err != nil {
				slog.Error("password reset email failed", "err", err)
				return alertError(e, "No se pudo enviar el correo. Contacta al administrador.")
			}
		}
	}

	return alertSuccess(e, "Si el email está registrado, recibirás un enlace para restablecer tu contraseña.")
}

// ResetPassword renders the reset-password form with the token from the URL.
func (h *PasswordResetHandler) ResetPassword(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	if token == "" {
		return h.renderPage(e, "reset-password.html", map[string]any{
			"PageTitle": "Contraseña",
			"Expired":   true,
		})
	}
	_, err := h.app.FindAuthRecordByToken(token, core.TokenTypePasswordReset)
	if err != nil {
		return h.renderPage(e, "reset-password.html", map[string]any{
			"PageTitle": "Contraseña",
			"Expired":   true,
		})
	}
	return h.renderPage(e, "reset-password.html", map[string]any{
		"PageTitle": "Contraseña",
		"Token":     token,
	})
}

// ResetPasswordSubmit processes the new password and confirms the token.
func (h *PasswordResetHandler) ResetPasswordSubmit(e *core.RequestEvent) error {
	token := e.Request.FormValue("token")
	password := e.Request.FormValue("password")
	passwordConfirm := e.Request.FormValue("passwordConfirm")

	if token == "" {
		return alertError(e, "Token inválido o expirado.")
	}

	if password == "" || password != passwordConfirm {
		return alertError(e, "Las contraseñas no coinciden.")
	}

	record, err := h.app.FindAuthRecordByToken(token, core.TokenTypePasswordReset)
	if err != nil || record == nil {
		return alertError(e, "Token inválido o expirado.")
	}

	record.SetPassword(password)
	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al cambiar la contraseña.")
	}

	middleware.ClearAuthCookie(e)
	return redirectHX(e, "/login")
}
