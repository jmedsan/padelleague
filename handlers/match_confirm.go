package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

func (h *MatchHandler) MatchConfirm(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido no está pendiente de confirmación</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	submittedByID := match.GetString("submitted_by")
	if submittedByID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No se encontró quién envió el resultado</div>`)
	}
	submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al verificar el equipo que envió el resultado</div>`)
	}

	if myTeam == submitterTeam {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No puedes confirmar tu propio resultado</div>`)
	}

	score := match.GetString("scores")
	winnerID, err := league.DetermineWinner(match, score)
	if err != nil {
		slog.Error("determine winner failed", "match", match.Id, "err", err)
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al determinar el ganador</div>`)
	}

	match.Set("confirmed_by", userID)
	match.Set("winner", winnerID)
	match.Set("status", "final")

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al confirmar el partido</div>`)
	}

	submitterPairID := match.GetString("pair1")
	if submitterTeam == 2 {
		submitterPairID = match.GetString("pair2")
	}
	submitterPlayers := league.PlayersForPair(h.app, submitterPairID)
	h.notifier.NotifyPlayers(submitterPlayers, "general", "Resultado confirmado", "Tu rival ha confirmado el resultado del partido.", match.Id)
	notify.EmailNotifyPlayers(h.app, submitterPlayers, "Resultado confirmado", "Tu rival ha confirmado el resultado del partido.", "/match/"+id)

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) MatchDispute(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido no está pendiente de confirmación</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	submittedByID := match.GetString("submitted_by")
	if submittedByID != "" {
		submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
		if err == nil && myTeam == submitterTeam {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">No puedes disputar tu propio resultado</div>`)
		}
	}

	disputeNotes := e.Request.FormValue("dispute_notes")
	match.Set("disputed_by", userID)
	match.Set("dispute_notes", disputeNotes)
	match.Set("status", "disputed")

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al disputar el partido</div>`)
	}

	if err := h.notifier.NotifyAdmins("dispute", "Partido disputado", disputeNotes, match.Id); err != nil {
		slog.Error("notify admins failed", "err", err)
	}

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) MatchCorrect(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se puede corregir un partido en estado confirmado</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	submittedByID := match.GetString("submitted_by")
	if submittedByID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No se encontró quién envió el resultado</div>`)
	}

	myTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}
	if !isAdmin {
		submitterTeam, err := league.PlayerTeam(h.app, submittedByID, match)
		if err != nil || myTeam != submitterTeam {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo el equipo que envió el resultado puede corregirlo</div>`)
		}
	}

	submittedAt := match.GetString("submitted_at")
	if submittedAt == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No se encontró la fecha de envío</div>`)
	}
	t, err := time.Parse(time.RFC3339, submittedAt)
	if err != nil || time.Since(t) >= 24*time.Hour {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El plazo de 24 horas para corregir ha expirado</div>`)
	}

	scores := e.Request.FormValue("scores")
	if scores == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar el marcador corregido</div>`)
	}

	if strings.EqualFold(strings.TrimSpace(scores), "WO") {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Usa el botón de incomparecencia para reportar un WO</div>`)
	}

	if _, err := league.ParseScore(scores); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Marcador no valido</div>`)
	}

	match.Set("scores", scores)
	match.Set("confirmed_by", "")
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al corregir el resultado</div>`)
	}

	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, "quorum_request", "Resultado corregido", "El rival ha corregido el resultado. Confirma o disputa.", match.Id)

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) MatchWalkover(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se puede reportar incomparecencia en partidos pendientes</div>`)
	}

	userID := e.Auth.Id
	isAdmin := e.Auth.GetString("role") == "admin"
	if _, err := league.PlayerTeam(h.app, userID, match); err != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	absentTeam := e.Request.FormValue("absent_team")
	var winnerID string
	switch absentTeam {
	case "1":
		winnerID = match.GetString("pair2")
	case "2":
		winnerID = match.GetString("pair1")
	default:
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar qué equipo no se presentó</div>`)
	}

	match.Set("scores", "WO")
	match.Set("winner", winnerID)
	match.Set("submitted_by", userID)
	match.Set("status", "final")

	if err := h.app.Save(match); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al registrar la incomparecencia</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/match/"+id)
	return e.NoContent(http.StatusNoContent)
}
