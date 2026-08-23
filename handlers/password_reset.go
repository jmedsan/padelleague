package handlers

import (
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"

	"padelleague/notify"
)

type PasswordResetHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPasswordResetHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PasswordResetHandler {
	return &PasswordResetHandler{app: app, renderPage: renderPage}
}

func (h *PasswordResetHandler) ForgotPassword(e *core.RequestEvent) error {
	return h.renderPage(e, "forgot-password.html", nil)
}

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
			}
		}
	}

	return alertSuccess(e, "Si el email está registrado, recibirás un enlace para restablecer tu contraseña.")
}

func (h *PasswordResetHandler) ResetPassword(e *core.RequestEvent) error {
	token := e.Request.URL.Query().Get("token")
	return h.renderPage(e, "reset-password.html", map[string]any{
		"Token": token,
	})
}

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

	return redirectHX(e, "/login")
}
