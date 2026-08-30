package handlers

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
)

// MatchHandler handles match detail, score submission, and correction flows.
type MatchHandler struct {
	app             core.App
	notifier        *notify.Notifier
	renderPage      RenderFunc
	renderErrorPage RenderErrorFunc
}

// NewMatchHandler creates a MatchHandler with the given dependencies.
func NewMatchHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc, renderErrorPage RenderErrorFunc) *MatchHandler {
	return &MatchHandler{app: app, notifier: notifier, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

func statusLabel(status string) string {
	switch status {
	case league.StatusPending:
		return "Pendiente"
	case league.StatusConfirmed:
		return "Enviado — esperando confirmación"
	case league.StatusDisputed:
		return "En disputa"
	case league.StatusFinal:
		return "Finalizado"
	}
	return status
}

func statusClass(status string) string {
	switch status {
	case league.StatusPending:
		return "badge-warning"
	case league.StatusConfirmed:
		return "badge-info"
	case league.StatusDisputed:
		return "badge-error"
	case league.StatusFinal:
		return "badge-success"
	}
	return "badge-ghost"
}

// canReportUnplayed mirrors the status precondition ReportUnplayed enforces.
func canReportUnplayed(status string, team int) bool {
	return team > 0 && (status == league.StatusPending || status == league.StatusConfirmed)
}

// MatchDetail renders the match page with score, status, and available actions.
func (h *MatchHandler) MatchDetail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Record no encontrado")
	}

	userID := e.Auth.Id
	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	isParticipant := teamErr == nil

	if !isParticipant && !isAdmin {
		return h.renderErrorPage(e, http.StatusForbidden, "No tienes acceso a este partido")
	}

	mode := PlayerFull
	if render.AdminView(e) {
		mode = AdminFull
	}
	mc := NewMatchCard(h.app, match, mode, userID)

	compName := ""
	compID := match.GetString("competition")
	if compID != "" {
		comp, _ := h.app.FindRecordById("competitions", compID)
		if comp != nil {
			compName = comp.GetString("name")
		}
	}

	shareText := buildShareText(h.app, match)
	venues, _ := h.app.FindRecordsByFilter("venues", "", "name", 0, 0, nil)
	mc.Venues = venues

	return h.renderPage(e, "match.html", map[string]any{
		"Card":            mc,
		"CompetitionName": compName,
		"CompetitionID":   compID,
		"ShareText":       shareText,
	})
}

// MatchSubmit processes a score submission from one of the participating pairs.
func (h *MatchHandler) MatchSubmit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Record no encontrado")
	}

	userID := e.Auth.Id
	isAdmin := slices.Contains(e.Auth.GetStringSlice("roles"), "admin")
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	if match.GetString("status") != league.StatusPending {
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
		return alertError(e, "Marcador no válido")
	}

	match.Set("scores", scores)
	match.Set("submitted_by", userID)
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("status", league.StatusConfirmed)
	match.Set("confirm_reminded", false)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al guardar el resultado")
	}

	label := pairPlayerLabel(h.app, userID, match)
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_event", Detail: label + " registró el resultado: " + scores,
	})

	myTeam, _ := league.PlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "quorum_request", Title: "Resultado enviado",
		Body: "Tu rival ha registrado un resultado. Confirma o disputa.", MatchID: match.Id,
	})
	h.notifier.EmailPlayers(rivalPlayers, "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", "/match/"+match.Id)

	return redirectHX(e, "/match/"+match.Id)
}

// AdminOverride lets an admin set the final score, bypassing the normal flow.
func (h *MatchHandler) AdminOverride(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Record no encontrado")
	}

	if !slices.Contains(e.Auth.GetStringSlice("roles"), "admin") {
		return alertError(e, "Solo administradores")
	}

	changes, alertErr := h.detectChanges(e, match)
	if alertErr != nil {
		return alertErr
	}

	if len(changes) == 0 {
		return alertWarning(e, "No se detectaron cambios")
	}

	if e.Request.FormValue("date") != "" {
		if err := h.validatePlayoffDates(match); err != nil {
			return alertError(e, "Las fechas de playoff deben respetar el orden del cuadro (una ronda posterior no puede ir antes que una previa)")
		}
	}

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al guardar")
	}

	adminName := league.PlayerName(h.app, e.Auth.Id)
	addTimelineEntry(h.app, timelineEntry{
		MatchID: id, ActorID: e.Auth.Id,
		Kind: "admin_action", Detail: adminName + " (admin): " + strings.Join(changes, "; "),
	})

	return redirectHX(e, "/match/"+id)
}

func (h *MatchHandler) detectChanges(e *core.RequestEvent, match *core.Record) ([]string, error) {
	var changes []string

	scoreChange, err := h.detectScoreChange(e, match)
	if err != nil {
		return nil, err
	}
	changes = append(changes, scoreChange...)
	changes = append(changes, detectFieldChange(match, "date", e.Request.FormValue("date"), "Fecha")...)
	changes = append(changes, detectFieldChange(match, "time", e.Request.FormValue("time"), "Hora")...)
	changes = append(changes, h.detectVenueChange(match, e.Request.FormValue("venue_id"))...)
	changes = append(changes, detectFieldChange(match, "court_number", e.Request.FormValue("court_number"), "Pista")...)

	return changes, nil
}

func (h *MatchHandler) detectScoreChange(e *core.RequestEvent, match *core.Record) ([]string, error) {
	scores := e.Request.FormValue("scores")
	if scores == "" {
		return nil, nil
	}
	oldScores := match.GetString("scores")
	if scores == oldScores {
		return nil, nil
	}
	if _, err := league.ParseScore(scores); err != nil {
		return nil, alertError(e, "Marcador no válido")
	}
	winner, err := league.DetermineWinner(match, scores)
	if err != nil {
		return nil, alertError(e, "No se pudo determinar ganador")
	}
	match.Set("scores", scores)
	match.Set("winner", winner)
	if match.GetString("status") != league.StatusFinal {
		match.Set("status", league.StatusFinal)
	}
	if oldScores == "" {
		return []string{"Resultado establecido: " + scores}, nil
	}
	return []string{"Resultado corregido: " + oldScores + " → " + scores}, nil
}

func (h *MatchHandler) validatePlayoffDates(match *core.Record) error {
	comp, err := h.app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil || !league.IsPlayoff(comp) {
		return nil
	}
	allMatches, err := h.app.FindRecordsByFilter("matches",
		"competition = {:comp}", "", 0, 0,
		map[string]any{"comp": comp.Id})
	if err != nil {
		return nil
	}
	for i, m := range allMatches {
		if m.Id == match.Id {
			allMatches[i] = match
			break
		}
	}
	return league.ValidatePlayoffDates(allMatches)
}

func detectFieldChange(match *core.Record, field, newVal, label string) []string {
	if newVal == "" || newVal == match.GetString(field) {
		return nil
	}
	old := match.GetString(field)
	match.Set(field, newVal)
	if old == "" {
		return []string{label + " establecida: " + newVal}
	}
	return []string{label + " cambiada: " + old + " → " + newVal}
}

func (h *MatchHandler) detectVenueChange(match *core.Record, venueID string) []string {
	if venueID == "" {
		return nil
	}
	venueName := venueID
	if v, err := h.app.FindRecordById("venues", venueID); err == nil {
		venueName = v.GetString("name")
	}
	old := match.GetString("club")
	if venueName == old {
		return nil
	}
	match.Set("club", venueName)
	if old == "" {
		return []string{"Club establecido: " + venueName}
	}
	return []string{"Club cambiado: " + old + " → " + venueName}
}

// ReportUnplayed lets a participant report a match as unplayed (walkover request).
// The match moves to disputed with review_type=walkover for admin approval.
func (h *MatchHandler) ReportUnplayed(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	userID := e.Auth.Id
	reporterTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil {
		return alertError(e, "No eres participante de este partido")
	}

	if match.GetString("review_type") == "walkover" {
		return redirectHX(e, "/match/"+id)
	}

	status := match.GetString("status")
	if status != league.StatusPending && status != league.StatusConfirmed {
		return alertError(e, "Este partido no puede reportarse como no jugado")
	}

	reason := e.Request.FormValue("reason")
	match.Set("review_type", "walkover")
	match.Set("walkover_requested_by", userID)
	match.Set("status", league.StatusDisputed)
	match.Set("dispute_notes", "[No jugado] "+reason)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al reportar")
	}

	label := pairPlayerLabel(h.app, userID, match)
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_event", Detail: label + " reportó el partido como no jugado",
	})

	if err := h.notifier.NotifyAdmins("dispute", "Partido no jugado", "Un jugador ha reportado un partido como no jugado.", id); err != nil {
		slog.Error("notify admins walkover report", "match", id, "err", err)
	}

	// Tell the rival pair a walkover was filed against them.
	rivalPairID := match.GetString("pair2")
	if reporterTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, league.Notification{
		Type: "general", Title: "Partido reportado como no jugado",
		Body: "Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.", MatchID: id,
	})
	h.notifier.EmailPlayers(rivalPlayers, "Partido reportado como no jugado",
		"Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.", "/match/"+id)

	return redirectHX(e, "/match/"+id)
}

func playerNameIfSet(app core.App, userID string) string {
	if userID == "" {
		return ""
	}
	return league.PlayerName(app, userID)
}

func buildShareText(app core.App, match *core.Record) string {
	if match.GetString("status") != league.StatusFinal {
		return ""
	}
	return url.QueryEscape(NewMatchCard(app, match, PlayerFull, "").SummaryLine())
}
