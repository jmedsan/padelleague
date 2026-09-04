package handlers

import (
	"errors"
	"log/slog"
	"mime/multipart"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"padelleague/league"
)

const avatarMaxUploadSize = 5 << 20 // matches the users.avatar FileField MaxSize

// PlayerAvatarUpload handles POST to upload and set a player's own avatar
// photo. Only the player themselves may set their avatar. The image is
// corrected for EXIF orientation, center-cropped to a square, resized, and
// re-encoded as JPEG before being saved on the record (league.CompressAvatarBytes).
func (h *PlayerHandler) PlayerAvatarUpload(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if e.Auth == nil || e.Auth.Id != id {
		return alertError(e, "No puedes cambiar la foto de otro jugador")
	}

	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	fh := fileHeader(e, "avatar")
	if fh == nil {
		return alertError(e, "Selecciona una imagen")
	}

	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return alertError(e, "El archivo debe ser una imagen")
	}

	if fh.Size > avatarMaxUploadSize {
		return alertError(e, "La imagen no puede superar los 5 MB")
	}

	f, errMsg := compressAvatar(fh, id+"_avatar.jpg")
	if errMsg != "" {
		return alertError(e, errMsg)
	}

	user.Set("avatar", f)
	if err := h.app.Save(user); err != nil {
		slog.Error("save avatar", "err", err)
		return alertError(e, "Error al guardar la foto")
	}

	return h.render.Partial(e, "avatar-fragment.html", map[string]any{
		"Mode":          PlayerFull,
		"ID":            user.Id,
		"Name":          user.GetString("display_name"),
		"Roles":         user.GetStringSlice("roles"),
		"AvatarURL":     league.AvatarURL(user.Id, user.GetString("avatar")),
		"CanEditAvatar": true,
	})
}

// compressAvatar opens the uploaded multipart file and runs it through
// league.CompressAvatarBytes, translating failures into user-facing Spanish
// messages instead of raw errors.
func compressLogo(fh *multipart.FileHeader, filename string) (*filesystem.File, string) {
	src, err := fh.Open()
	if err != nil {
		return nil, "No se pudo leer la imagen"
	}
	defer func() { _ = src.Close() }()

	f, err := league.CompressLogoBytes(src, filename)
	if err != nil {
		switch {
		case errors.Is(err, league.ErrAvatarTooLarge):
			return nil, "La imagen es demasiado grande"
		case errors.Is(err, league.ErrAvatarInvalid):
			return nil, "Imagen no válida"
		case errors.Is(err, league.ErrAvatarUnreadable):
			return nil, "No se pudo leer la imagen"
		default:
			return nil, "Error al procesar la imagen"
		}
	}
	return f, ""
}

func compressAvatar(fh *multipart.FileHeader, filename string) (*filesystem.File, string) {
	src, err := fh.Open()
	if err != nil {
		return nil, "No se pudo leer la imagen"
	}
	defer func() { _ = src.Close() }()

	f, err := league.CompressAvatarBytes(src, filename)
	if err != nil {
		switch {
		case errors.Is(err, league.ErrAvatarTooLarge):
			return nil, "La imagen es demasiado grande"
		case errors.Is(err, league.ErrAvatarInvalid):
			return nil, "Imagen no válida"
		case errors.Is(err, league.ErrAvatarUnreadable):
			return nil, "No se pudo leer la imagen"
		default:
			return nil, "Error al procesar la imagen"
		}
	}
	return f, ""
}
