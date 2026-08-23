package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type MatchHandler struct {
	app             core.App
	notifier        *Notifier
	renderPage      func(e *core.RequestEvent, page string, data map[string]any) error
	renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error
}

func NewMatchHandler(app core.App, notifier *Notifier, renderPage func(e *core.RequestEvent, page string, data map[string]any) error, renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error) *MatchHandler {
	return &MatchHandler{app: app, notifier: notifier, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

type MatchView struct {
	Record        *core.Record
	Pair1Name     string
	Pair2Name     string
	RoundNum      int
	CanSubmit     bool
	CanConfirm    bool
	CanDispute    bool
	CanEdit       bool
	CanWalkover   bool
	CanCorrect    bool
	IsAdmin       bool
	IsParticipant bool
	StatusLabel   string
	StatusClass   string
}

type MatchDetailData struct {
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

func (h *MatchHandler) buildMatchView(match *core.Record, userID string, pairNames map[string]string, isAdmin, isParticipant bool) MatchView {
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
		Record:        match,
		Pair1Name:     pairNames[match.GetString("pair1")],
		Pair2Name:     pairNames[match.GetString("pair2")],
		RoundNum:      roundNum,
		CanSubmit:     status == "pending" && team > 0,
		CanConfirm:    status == "confirmed" && team > 0 && !isSubmitter,
		CanDispute:    status == "confirmed" && team > 0 && !isSubmitter,
		CanEdit:       status == "pending" && team > 0,
		CanWalkover:   status == "pending" && team > 0,
		CanCorrect:    canCorrect,
		IsAdmin:       isAdmin,
		IsParticipant: isParticipant,
		StatusLabel:   statusLabel(status),
		StatusClass:   statusClass(status),
	}
}

func (h *MatchHandler) MatchDetail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Record no encontrado")
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	_, teamErr := getPlayerTeam(h.app, userID, match)
	isParticipant := teamErr == nil

	if !isParticipant && !isAdmin {
		return h.renderErrorPage(e, http.StatusForbidden, "No tienes acceso a este partido")
	}

	pairNames, _ := expandPairNames(h.app, []string{
		match.GetString("pair1"),
		match.GetString("pair2"),
	})

	mv := h.buildMatchView(match, userID, pairNames, isAdmin, isParticipant)

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

	return h.renderPage(e, "match.html", map[string]any{
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
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Record no encontrado</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	_, teamErr := getPlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido ya tiene un resultado registrado</div>`)
	}

	scores := e.Request.FormValue("scores")
	if scores == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar el marcador</div>`)
	}

	if _, _, _, _, err := parseScore(scores); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Marcador no valido</div>`)
	}

	match.Set("scores", scores)
	match.Set("submitted_by", userID)
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("status", "confirmed")

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar el resultado</div>`)
	}

	myTeam, _ := getPlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := getPlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, "quorum_request", "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", match.Id)
	emailNotifyPlayers(h.app, rivalPlayers, "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", "/match/"+match.Id)

	e.Response.Header().Set("HX-Redirect", "/")
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) MatchEdit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Record no encontrado</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	_, teamErr := getPlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
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

func (h *MatchHandler) createAdminTimelineEntry(matchID, adminID, content string) {
	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return
	}
	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", adminID)
	record.Set("type", "admin_action")
	record.Set("content", content)
	h.app.Save(record)
}

func (h *MatchHandler) AdminOverride(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Record no encontrado</div>`)
	}

	if e.Auth.GetString("role") != "admin" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo administradores</div>`)
	}

	var changes []string

	if scores := e.Request.FormValue("scores"); scores != "" {
		oldScores := match.GetString("scores")
		if scores != oldScores {
			if _, _, _, _, err := parseScore(scores); err != nil {
				return e.HTML(http.StatusOK, `<div class="alert alert-error">Marcador no valido</div>`)
			}
			winner, err := determineWinner(match, scores)
			if err != nil {
				return e.HTML(http.StatusOK, `<div class="alert alert-error">No se pudo determinar ganador</div>`)
			}
			match.Set("scores", scores)
			match.Set("winner", winner)
			if match.GetString("status") != "final" {
				match.Set("status", "final")
			}
			if oldScores == "" {
				changes = append(changes, "Resultado establecido: "+scores)
			} else {
				changes = append(changes, "Resultado corregido: "+oldScores+" → "+scores)
			}
		}
	}

	if date := e.Request.FormValue("date"); date != "" && date != match.GetString("date") {
		old := match.GetString("date")
		match.Set("date", date)
		if old == "" {
			changes = append(changes, "Fecha establecida: "+date)
		} else {
			changes = append(changes, "Fecha cambiada: "+old+" → "+date)
		}
	}

	if t := e.Request.FormValue("time"); t != "" && t != match.GetString("time") {
		old := match.GetString("time")
		match.Set("time", t)
		if old == "" {
			changes = append(changes, "Hora establecida: "+t)
		} else {
			changes = append(changes, "Hora cambiada: "+old+" → "+t)
		}
	}

	if venueID := e.Request.FormValue("venue_id"); venueID != "" {
		venueName := venueID
		if v, err := h.app.FindRecordById("venues", venueID); err == nil {
			venueName = v.GetString("name")
		}
		old := match.GetString("club")
		if venueName != old {
			match.Set("club", venueName)
			if old == "" {
				changes = append(changes, "Club establecido: "+venueName)
			} else {
				changes = append(changes, "Club cambiado: "+old+" → "+venueName)
			}
		}
	}

	if len(changes) == 0 {
		return e.HTML(http.StatusOK, `<div class="alert alert-warning">No se detectaron cambios</div>`)
	}

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar</div>`)
	}

	adminName := resolvePlayerName(h.app, e.Auth.Id)
	h.createAdminTimelineEntry(id, e.Auth.Id, adminName+" (admin): "+strings.Join(changes, "; "))

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}
