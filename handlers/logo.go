package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
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
	return h.redirect(e, league.PBFileURL("competitions", rec.Id, filename))
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
	return h.redirect(e, league.PBFileURL("sponsors", rec.Id, filename))
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
	return h.redirect(e, league.PBFileURL("app_settings", records[0].Id, filename))
}

func (h *LogoHandler) redirect(e *core.RequestEvent, path string) error {
	return e.Redirect(http.StatusFound, path)
}
