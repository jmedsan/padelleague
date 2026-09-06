package handlers

import (
	"net/http"
	"path"

	"github.com/pocketbase/pocketbase/core"
)

// LogoHandler serves logo files at predictable URLs so email clients
// can reference them without knowing PocketBase's random filenames.
type LogoHandler struct {
	app core.App
}

// NewLogoHandler creates a LogoHandler.
func NewLogoHandler(app core.App) *LogoHandler {
	return &LogoHandler{app: app}
}

// LeagueLogo serves the league logo from app_settings, falling back to the
// static PWA icon when no logo is uploaded.
func (h *LogoHandler) LeagueLogo(e *core.RequestEvent) error {
	records, err := h.app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return e.Redirect(http.StatusFound, "/static/img/icon-192.png")
	}
	filename := records[0].GetString("league_logo")
	if filename == "" {
		return e.Redirect(http.StatusFound, "/static/img/icon-192.png")
	}
	return h.serveRecordFile(e, records[0], filename)
}

// CompetitionLogo serves a competition's logo.
func (h *LogoHandler) CompetitionLogo(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	rec, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	filename := rec.GetString("logo")
	if filename == "" {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "no logo"})
	}
	return h.serveRecordFile(e, rec, filename)
}

// SponsorLogo serves a sponsor's logo.
func (h *LogoHandler) SponsorLogo(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	rec, err := h.app.FindRecordById("sponsors", id)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	filename := rec.GetString("logo")
	if filename == "" {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "no logo"})
	}
	return h.serveRecordFile(e, rec, filename)
}

func (h *LogoHandler) serveRecordFile(e *core.RequestEvent, rec *core.Record, filename string) error {
	fsys, err := h.app.NewFilesystem()
	if err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"error": "filesystem error"})
	}
	defer func() { _ = fsys.Close() }()

	key := path.Join(rec.BaseFilesPath(), filename)
	blob, err := fsys.GetReader(key)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "file not found"})
	}
	defer func() { _ = blob.Close() }()

	e.Response.Header().Set("Cache-Control", "public, max-age=86400")
	return e.Stream(http.StatusOK, "application/octet-stream", blob)
}
