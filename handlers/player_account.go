package handlers

import (
	"log/slog"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PlayerNameUpdate handles POST to change a player's own display name. Only
// the player themselves may edit it.
func (h *PlayerHandler) PlayerNameUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if e.Auth == nil || e.Auth.Id != id {
		return alertError(e, "No puedes editar el perfil de otro jugador")
	}

	displayName := e.Request.FormValue("display_name")
	if displayName == "" {
		return alertError(e, "El nombre no puede estar vacío")
	}

	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	user.Set("display_name", displayName)
	if err := h.app.Save(user); err != nil {
		slog.Error("update player display name", "err", err)
		return alertError(e, "Error al guardar el nombre")
	}

	flash(e, "Nombre actualizado")
	return redirectHX(e, "/player/"+id)
}

// PlayerPhoneUpdate handles POST to change a player's own phone number.
// Only the player themselves may edit it.
func (h *PlayerHandler) PlayerPhoneUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if e.Auth == nil || e.Auth.Id != id {
		return alertError(e, "No puedes editar el perfil de otro jugador")
	}

	phone, phoneErr := league.NormalizePhone(strings.TrimSpace(e.Request.FormValue("phone")))
	if phoneErr != nil {
		return alertError(e, phoneErr.Error()) //nolint:goerr113 // user-facing Spanish
	}

	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	user.Set("phone", phone)
	if err := h.app.Save(user); err != nil {
		slog.Error("update player phone", "err", err)
		return alertError(e, "Error al guardar el teléfono")
	}

	flash(e, "Teléfono actualizado")
	return redirectHX(e, "/player/"+id)
}

// PlayerPasswordUpdate handles POST to change a player's own password. Only
// the player themselves may change it, and the current password must be
// supplied and correct.
func (h *PlayerHandler) PlayerPasswordUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if e.Auth == nil || e.Auth.Id != id {
		return alertError(e, "No puedes cambiar la contraseña de otro jugador")
	}

	currentPassword := e.Request.FormValue("current_password")
	newPassword := e.Request.FormValue("new_password")
	newPasswordConfirm := e.Request.FormValue("new_password_confirm")

	if newPassword != newPasswordConfirm {
		return alertError(e, "Las contraseñas nuevas no coinciden")
	}

	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	if !user.ValidatePassword(currentPassword) {
		return alertError(e, "La contraseña actual no es correcta")
	}

	user.SetPassword(newPassword)
	if err := h.app.Save(user); err != nil {
		slog.Error("update player password", "err", err)
		return alertError(e, "Error al guardar la contraseña (debe tener al menos 8 caracteres)")
	}

	flash(e, "Contraseña actualizada")
	return redirectHX(e, "/player/"+id)
}
