package handlers

import (
	"net/http"
	"path/filepath"

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

// LeagueLogo serves the league logo from app_settings.
func (h *LogoHandler) LeagueLogo(e *core.RequestEvent) error {
	return h.serveFile(e, "app_settings", func(rec *core.Record) string {
		return rec.GetString("league_logo")
	})
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

func (h *LogoHandler) serveFile(e *core.RequestEvent, collection string, getFilename func(*core.Record) string) error {
	records, err := h.app.FindRecordsByFilter(collection, "", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	filename := getFilename(records[0])
	if filename == "" {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "no logo"})
	}
	return h.serveRecordFile(e, records[0], filename)
}

func (h *LogoHandler) serveRecordFile(e *core.RequestEvent, rec *core.Record, filename string) error {
	fsys, err := h.app.NewFilesystem()
	if err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"error": "filesystem error"})
	}
	defer fsys.Close()

	key := filepath.Join(rec.Collection().Id, rec.Id, filename)
	blob, err := fsys.GetFile(key)
	if err != nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "file not found"})
	}
	defer blob.Close()

	contentType := "image/jpeg"
	switch filepath.Ext(filename) {
	case ".png":
		contentType = "image/png"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	}
	e.Response.Header().Set("Cache-Control", "public, max-age=86400")
	return e.Stream(http.StatusOK, contentType, blob)
}
