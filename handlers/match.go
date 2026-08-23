package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
	"padelleague/notify"
)

type MatchHandler struct {
	app             core.App
	notifier        *notify.Notifier
	renderPage      func(e *core.RequestEvent, page string, data map[string]any) error
	renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error
}

func NewMatchHandler(app core.App, notifier *notify.Notifier, renderPage func(e *core.RequestEvent, page string, data map[string]any) error, renderErrorPage func(e *core.RequestEvent, statusCode int, message string) error) *MatchHandler {
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
	case StatusPending:
		return "Pendiente"
	case StatusConfirmed:
		return "Enviado — esperando confirmación"
	case StatusDisputed:
		return "En disputa"
	case StatusFinal:
		return "Finalizado"
	}
	return status
}

func statusClass(status string) string {
	switch status {
	case StatusPending:
		return "badge-warning"
	case StatusConfirmed:
		return "badge-info"
	case StatusDisputed:
		return "badge-error"
	case StatusFinal:
		return "badge-success"
	}
	return "badge-ghost"
}

func (h *MatchHandler) buildMatchView(match *core.Record, userID string, pairNames map[string]string, isAdmin, isParticipant bool) MatchView {
	status := match.GetString("status")
	submittedBy := match.GetString("submitted_by")

	team, _ := league.PlayerTeam(h.app, userID, match)
	isSubmitter := false
	if submittedBy != "" {
		submitterTeam, err := league.PlayerTeam(h.app, submittedBy, match)
		if err == nil {
			isSubmitter = (submitterTeam == team)
		}
	}

	roundNum := int(match.GetFloat("round_number"))

	canCorrect := false
	if status == StatusConfirmed && team > 0 && isSubmitter {
		submittedAt := match.GetString("submitted_at")
		if submittedAt != "" {
			if dt, err := types.ParseDateTime(submittedAt); err == nil {
				canCorrect = time.Since(dt.Time()) < 24*time.Hour
			}
		}
	}

	return MatchView{
		Record:        match,
		Pair1Name:     pairNames[match.GetString("pair1")],
		Pair2Name:     pairNames[match.GetString("pair2")],
		RoundNum:      roundNum,
		CanSubmit:     status == StatusPending && team > 0,
		CanConfirm:    status == StatusConfirmed && team > 0 && !isSubmitter,
		CanDispute:    status == StatusConfirmed && team > 0 && !isSubmitter,
		CanEdit:       status == StatusPending && team > 0,
		CanWalkover:   status == StatusPending && team > 0,
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
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	isParticipant := teamErr == nil

	if !isParticipant && !isAdmin {
		return h.renderErrorPage(e, http.StatusForbidden, "No tienes acceso a este partido")
	}

	pairNames := league.PairNames(h.app, []string{
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
		submittedByName = league.PlayerName(h.app, sbID)
	}
	confirmedByName := ""
	if cbID := match.GetString("confirmed_by"); cbID != "" {
		confirmedByName = league.PlayerName(h.app, cbID)
	}
	disputedByName := ""
	if dbID := match.GetString("disputed_by"); dbID != "" {
		disputedByName = league.PlayerName(h.app, dbID)
	}

	shareText := ""
	if match.GetString("status") == StatusFinal {
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
		return alertError(e, "Record no encontrado")
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	if match.GetString("status") != StatusPending {
		return alertError(e, "Este partido ya tiene un resultado registrado")
	}

	scores := e.Request.FormValue("scores")
	if scores == "" {
		return alertError(e, "Debes indicar el marcador")
	}

	if strings.EqualFold(strings.TrimSpace(scores), "WO") {
		return alertError(e, "Usa el botón de incomparecencia para reportar un WO")
	}

	if _, err := league.ParseScore(scores); err != nil {
		return alertError(e, "Marcador no valido")
	}

	match.Set("scores", scores)
	match.Set("submitted_by", userID)
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("status", StatusConfirmed)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al guardar el resultado")
	}

	myTeam, _ := league.PlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, "quorum_request", "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", match.Id)
	notify.EmailNotifyPlayers(h.app, rivalPlayers, "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", "/match/"+match.Id)

	return redirectHX(e, "/")
}

func (h *MatchHandler) MatchEdit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Record no encontrado")
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	if match.GetString("status") != StatusPending {
		return alertError(e, "Solo se pueden editar partidos pendientes")
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
		return alertError(e, "Error al guardar los cambios")
	}

	return redirectHX(e, "/match/"+id)
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
	if err := h.app.Save(record); err != nil {
		slog.Error("save admin timeline entry", "match", matchID, "err", err)
	}
}

func (h *MatchHandler) AdminOverride(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Record no encontrado")
	}

	if e.Auth.GetString("role") != "admin" {
		return alertError(e, "Solo administradores")
	}

	changes, alertErr := h.detectChanges(e, match)
	if alertErr != nil {
		return alertErr
	}

	if len(changes) == 0 {
		return alertWarning(e, "No se detectaron cambios")
	}

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al guardar")
	}

	adminName := league.PlayerName(h.app, e.Auth.Id)
	h.createAdminTimelineEntry(id, e.Auth.Id, adminName+" (admin): "+strings.Join(changes, "; "))

	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) detectChanges(e *core.RequestEvent, match *core.Record) ([]string, error) {
	var changes []string

	if scores := e.Request.FormValue("scores"); scores != "" {
		oldScores := match.GetString("scores")
		if scores != oldScores {
			if _, err := league.ParseScore(scores); err != nil {
				return nil, alertError(e, "Marcador no valido")
			}
			winner, err := league.DetermineWinner(match, scores)
			if err != nil {
				return nil, alertError(e, "No se pudo determinar ganador")
			}
			match.Set("scores", scores)
			match.Set("winner", winner)
			if match.GetString("status") != StatusFinal {
				match.Set("status", StatusFinal)
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

	return changes, nil
}
