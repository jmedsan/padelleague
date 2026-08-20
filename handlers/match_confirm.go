package handlers

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func (h *MatchHandler) PartidoConfirm(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if partido.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido no está pendiente de confirmación</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	myTeam, err := getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	submittedByID := partido.GetString("submitted_by")
	if submittedByID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No se encontró quién envió el resultado</div>`)
	}
	submitterTeam, err := getJugadorTeam(h.app, submittedByID, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al verificar el equipo que envió el resultado</div>`)
	}

	if myTeam == submitterTeam {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No puedes confirmar tu propio resultado</div>`)
	}

	score := partido.GetString("scores")
	winnerID, err := determineWinner(partido, score)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al determinar el ganador: `+err.Error()+`</div>`)
	}

	partido.Set("confirmed_by", jugador.Id)
	partido.Set("winner", winnerID)
	partido.Set("status", "final")

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al confirmar el partido</div>`)
	}

	submitterParejaID := partido.GetString("pareja1")
	if submitterTeam == 2 {
		submitterParejaID = partido.GetString("pareja2")
	}
	submitterPlayers := getPlayersForPair(h.app, submitterParejaID)
	notifyPlayers(h.app, submitterPlayers, "general", "Resultado confirmado", "Tu rival ha confirmado el resultado del partido.", partido.Id)
	emailNotifyPlayers(h.app, submitterPlayers, "Resultado confirmado", "Tu rival ha confirmado el resultado del partido.", "/partido/"+id)

	e.Response.Header().Set("HX-Redirect", "/partido/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) PartidoDispute(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if partido.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido no está pendiente de confirmación</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	myTeam, err := getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	submittedByID := partido.GetString("submitted_by")
	if submittedByID != "" {
		submitterTeam, err := getJugadorTeam(h.app, submittedByID, partido)
		if err == nil && myTeam == submitterTeam {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">No puedes disputar tu propio resultado</div>`)
		}
	}

	disputeNotes := e.Request.FormValue("dispute_notes")
	partido.Set("disputed_by", jugador.Id)
	partido.Set("dispute_notes", disputeNotes)
	partido.Set("status", "disputed")

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al disputar el partido</div>`)
	}

	notifyAdmins(h.app, "dispute", "Partido disputado", disputeNotes, partido.Id)

	e.Response.Header().Set("HX-Redirect", "/partido/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) PartidoCorrect(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if partido.GetString("status") != "confirmed" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se puede corregir un partido en estado confirmado</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	submittedByID := partido.GetString("submitted_by")
	if submittedByID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No se encontró quién envió el resultado</div>`)
	}

	myTeam, err := getJugadorTeam(h.app, jugador.Id, partido)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}
	submitterTeam, err := getJugadorTeam(h.app, submittedByID, partido)
	if err != nil || myTeam != submitterTeam {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo el equipo que envió el resultado puede corregirlo</div>`)
	}

	submittedAt := partido.GetString("submitted_at")
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

	partido.Set("scores", scores)
	partido.Set("confirmed_by", "")
	partido.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al corregir el resultado</div>`)
	}

	rivalParejaID := partido.GetString("pareja2")
	if myTeam == 2 {
		rivalParejaID = partido.GetString("pareja1")
	}
	rivalPlayers := getPlayersForPair(h.app, rivalParejaID)
	notifyPlayers(h.app, rivalPlayers, "quorum_request", "Resultado corregido", "El rival ha corregido el resultado. Confirma o disputa.", partido.Id)

	e.Response.Header().Set("HX-Redirect", "/partido/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *MatchHandler) PartidoWalkover(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	partido, err := h.app.FindRecordById("partidos", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if partido.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se puede reportar incomparecencia en partidos pendientes</div>`)
	}

	jugador, err := findJugadorByUser(h.app, e.Auth.Id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No estás registrado como jugador</div>`)
	}

	if _, err := getJugadorTeam(h.app, jugador.Id, partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	absentTeam := e.Request.FormValue("absent_team")
	var winnerID string
	switch absentTeam {
	case "1":
		winnerID = partido.GetString("pareja2")
	case "2":
		winnerID = partido.GetString("pareja1")
	default:
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Debes indicar qué equipo no se presentó</div>`)
	}

	partido.Set("scores", "WO")
	partido.Set("winner", winnerID)
	partido.Set("submitted_by", jugador.Id)
	partido.Set("status", "final")

	if err := h.app.Save(partido); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al registrar la incomparecencia</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/partido/"+id)
	return e.NoContent(http.StatusNoContent)
}
