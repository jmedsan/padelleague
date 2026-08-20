package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type MatchHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewMatchHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *MatchHandler {
	return &MatchHandler{app: app, renderPage: renderPage}
}

type MatchView struct {
	Partido     *core.Record
	Pareja1Name string
	Pareja2Name string
	JornadaNum  int
	CanSubmit   bool
	CanConfirm  bool
	CanDispute  bool
	CanEdit     bool
	CanWalkover bool
	CanCorrect  bool
	StatusLabel string
	StatusClass string
}

type PartidoDetailData struct {
	Match           MatchView
	CompetitionName string
	SubmittedBy     string
	ConfirmedBy     string
	DisputedBy      string
	DisputeNotes    string
}

func statusLabel(status string) string {
	switch status {
	case "pending":
		return "Pendiente"
	case "confirmed":
		return "Enviado — esperando confirmación"
	case "disputed":
		return "En disputa"
	case "final":
		return "Finalizado"
	}
	return status
}

func statusClass(status string) string {
	switch status {
	case "pending":
		return "badge-warning"
	case "confirmed":
		return "badge-info"
	case "disputed":
		return "badge-error"
	case "final":
		return "badge-success"
	}
	return "badge-ghost"
}

func (h *MatchHandler) buildMatchView(match *core.Record, userID string, pairNames map[string]string) MatchView {
	status := match.GetString("status")
	submittedBy := match.GetString("submitted_by")

	team, _ := getPlayerTeam(h.app, userID, match)
	isSubmitter := false
	if submittedBy != "" {
		submitterTeam, err := getPlayerTeam(h.app, submittedBy, match)
		if err == nil {
			isSubmitter = (submitterTeam == team)
		}
	}

	roundNum := int(match.GetFloat("round_number"))

	canCorrect := false
	if status == "confirmed" && team > 0 && isSubmitter {
		submittedAt := match.GetString("submitted_at")
		if submittedAt != "" {
			if t, err := time.Parse(time.RFC3339, submittedAt); err == nil {
				canCorrect = time.Since(t) < 24*time.Hour
			}
		}
	}

	return MatchView{
		Partido:     match,
		Pareja1Name: pairNames[match.GetString("pair1")],
		Pareja2Name: pairNames[match.GetString("pair2")],
		JornadaNum:  roundNum,
		CanSubmit:   status == "pending" && team > 0,
		CanConfirm:  status == "confirmed" && team > 0 && !isSubmitter,
		CanDispute:  status == "confirmed" && team > 0 && !isSubmitter,
		CanEdit:     status == "pending" && team > 0,
		CanWalkover: status == "pending" && team > 0,
		CanCorrect:  canCorrect,
		StatusLabel: statusLabel(status),
		StatusClass: statusClass(status),
	}
}

func (h *MatchHandler) MatchDetail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusNotFound, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	userID := e.Auth.Id
	_, err = getPlayerTeam(h.app, userID, match)
	if err != nil {
		return e.HTML(http.StatusForbidden, `<div class="alert alert-error">No tienes acceso a este partido</div>`)
	}

	pairNames, _ := expandPairNames(h.app, []string{
		match.GetString("pair1"),
		match.GetString("pair2"),
	})

	mv := h.buildMatchView(match, userID, pairNames)

	compName := ""
	compID := match.GetString("competition")
	if compID != "" {
		comp, _ := h.app.FindRecordById("competitions", compID)
		if comp != nil {
			compName = comp.GetString("name")
		}
	}

	submittedByName := ""
	if sbID := match.GetString("submitted_by"); sbID != "" {
		submittedByName = resolvePlayerName(h.app, sbID)
	}
	confirmedByName := ""
	if cbID := match.GetString("confirmed_by"); cbID != "" {
		confirmedByName = resolvePlayerName(h.app, cbID)
	}
	disputedByName := ""
	if dbID := match.GetString("disputed_by"); dbID != "" {
		disputedByName = resolvePlayerName(h.app, dbID)
	}

	shareText := ""
	if match.GetString("status") == "final" {
		p1Name := pairNames[match.GetString("pair1")]
		p2Name := pairNames[match.GetString("pair2")]
		score := match.GetString("scores")
		winnerName := p2Name
		if match.GetString("winner") == match.GetString("pair1") {
			winnerName = p1Name
		}
		shareText = url.QueryEscape(fmt.Sprintf("Resultado: %s %s %s. Ganador: %s!", p1Name, score, p2Name, winnerName))
	}

	venues, _ := h.app.FindRecordsByFilter("venues", "", "name", 0, 0, nil)

	return h.renderPage(e, "partido.html", map[string]any{
		"Match":           mv,
		"CompetitionName": compName,
		"CompetitionID":   compID,
		"SubmittedBy":     submittedByName,
		"ConfirmedBy":     confirmedByName,
		"DisputedBy":      disputedByName,
		"DisputeNotes":    match.GetString("dispute_notes"),
		"ShareText":       shareText,
		"Venues":          venues,
	})
}

func (h *MatchHandler) MatchSubmit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	userID := e.Auth.Id
	_, err = getPlayerTeam(h.app, userID, match)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido ya tiene un resultado registrado</div>`)
	}

	scores := e.Request.FormValue("scores")
	if scores == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar el marcador</div>`)
	}

	match.Set("scores", scores)
	match.Set("submitted_by", userID)
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("status", "confirmed")

	if date := e.Request.FormValue("date"); date != "" {
		match.Set("date", date)
	}
	if t := e.Request.FormValue("time"); t != "" {
		match.Set("time", t)
	}
	venue := e.Request.FormValue("venue")
	if venue == "__other__" {
		venue = e.Request.FormValue("custom_venue")
	}
	if venue != "" {
		match.Set("club", venue)
	}
	if court := e.Request.FormValue("court_number"); court != "" {
		match.Set("court_number", court)
	}

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar el resultado</div>`)
	}

	myTeam, _ := getPlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := getPlayersForPair(h.app, rivalPairID)
	notifyPlayers(h.app, rivalPlayers, "quorum_request", "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", match.Id)
	emailNotifyPlayers(h.app, rivalPlayers, "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", "/match/"+match.Id)

	e.Response.Header().Set("HX-Redirect", "/")
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) MatchEdit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	userID := e.Auth.Id
	_, err = getPlayerTeam(h.app, userID, match)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se pueden editar partidos pendientes</div>`)
	}

	if date := e.Request.FormValue("date"); date != "" {
		match.Set("date", date)
	}
	if t := e.Request.FormValue("time"); t != "" {
		match.Set("time", t)
	}
	venue := e.Request.FormValue("venue")
	if venue == "__other__" {
		venue = e.Request.FormValue("custom_venue")
	}
	if venue != "" {
		match.Set("club", venue)
	}
	if court := e.Request.FormValue("court_number"); court != "" {
		match.Set("court_number", court)
	}

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar los cambios</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}
