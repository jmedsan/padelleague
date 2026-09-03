// Package handlers implements HTTP handlers for all application routes.
package handlers

import (
	"errors"
	"html"
	"log/slog"
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

// RecordQuery bundles a FindRecordsByFilter call's arguments.
type RecordQuery struct {
	Collection string
	Filter     string
	Sort       string
	Limit      int
	Offset     int
	Params     map[string]any
}

// findRecordsLogged wraps FindRecordsByFilter, logging (not returning) a
// query failure — callers that already treat an empty result the same as a
// query error (the common "show what we have" case) still see the failure
// in the logs instead of it silently returning an empty slice.
func findRecordsLogged(app core.App, logCtx string, q RecordQuery) []*core.Record {
	records, err := app.FindRecordsByFilter(q.Collection, q.Filter, q.Sort, q.Limit, q.Offset, q.Params)
	if err != nil {
		slog.Error(logCtx, "err", err)
	}
	return records
}

func alertError(e *core.RequestEvent, msg string) error {
	return flashAlert(e, "alert-error", msg)
}

func alertSuccess(e *core.RequestEvent, msg string) error {
	return flashAlert(e, "alert-success", msg)
}

func alertWarning(e *core.RequestEvent, msg string) error {
	return flashAlert(e, "alert-warning", msg)
}

func flashAlert(e *core.RequestEvent, class, msg string) error {
	fragment := `<div class="` + class + ` text-sm py-2">` + html.EscapeString(msg) + `</div>`
	if e.Request.Header.Get("HX-Request") == "true" {
		e.Response.Header().Set("HX-Retarget", "#flash")
		e.Response.Header().Set("HX-Reswap", "innerHTML")
	}
	return e.HTML(http.StatusOK, fragment)
}

// errHandled is returned by guards that have already written a response
// (redirect, alert) so the caller knows to stop without returning nil.
var errHandled = errors.New("response already written")

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
			e.NoContent(http.StatusNoContent)
		} else {
			e.Redirect(http.StatusFound, target)
		}
		return errHandled
	}
	return nil
}

func checkCompModifiable(app core.App, e *core.RequestEvent, match *core.Record) error {
	if isEffectiveAdmin(e) {
		return nil
	}
	comp, err := app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil {
		alertError(e, "Competición no encontrada")
		return errHandled
	}
	if !league.PlayerCanModify(comp, time.Now()) {
		alertError(e, "La competición está finalizada o archivada; no puedes modificar este partido.")
		return errHandled
	}
	return nil
}

func redirectHX(e *core.RequestEvent, url string) error {
	e.Response.Header().Set("HX-Redirect", url)
	return e.NoContent(http.StatusNoContent)
}
