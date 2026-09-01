// Package handlers implements HTTP handlers for all application routes.
package handlers

import (
	"html"
	"net/http"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// RenderFunc renders a full page template.
type RenderFunc func(e *core.RequestEvent, page string, data map[string]any) error

// RenderErrorFunc renders an error page with a status code and message.
type RenderErrorFunc func(e *core.RequestEvent, statusCode int, message string) error

func findMatchOr404(app core.App, e *core.RequestEvent, id string) (*core.Record, error) {
	match, err := app.FindRecordById("matches", id)
	if err != nil {
		return nil, alertError(e, "Partido no encontrado")
	}
	return match, nil
}

func alertError(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-error">`+html.EscapeString(msg)+`</div>`)
}

func alertSuccess(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-success">`+html.EscapeString(msg)+`</div>`)
}

func alertWarning(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-warning">`+html.EscapeString(msg)+`</div>`)
}

func isEffectiveAdmin(e *core.RequestEvent) bool {
	return render.AdminView(e)
}

func checkDocGate(app core.App, e *core.RequestEvent, match *core.Record) error {
	if slices.Contains(e.Auth.GetStringSlice("roles"), "admin") {
		return nil
	}
	compID := match.GetString("competition")
	if compID == "" {
		return nil
	}
	comp, err := app.FindRecordById("competitions", compID)
	if err != nil {
		return nil
	}
	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(app, userID)
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}
	if !league.IsParticipant(comp, playerPairIDs) {
		return nil
	}
	if pending := league.UnacknowledgedMandatory(app, comp, userID); len(pending) > 0 {
		target := "/competition/" + compID
		if e.Request.Header.Get("HX-Request") == "true" {
			e.Response.Header().Set("HX-Redirect", target)
			return e.NoContent(http.StatusNoContent)
		}
		return e.Redirect(http.StatusFound, target)
	}
	return nil
}

func checkCompModifiable(app core.App, e *core.RequestEvent, match *core.Record) error {
	if isEffectiveAdmin(e) {
		return nil
	}
	comp, err := app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	if !league.PlayerCanModify(comp, time.Now()) {
		return alertError(e, "La competición está finalizada o archivada; no puedes modificar este partido.")
	}
	return nil
}

func redirectHX(e *core.RequestEvent, url string) error {
	e.Response.Header().Set("HX-Redirect", url)
	return e.NoContent(http.StatusNoContent)
}
