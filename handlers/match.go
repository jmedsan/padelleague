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

// MatchView holds a match record with display-ready fields and permission flags.
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

// MatchDetailData bundles a MatchView with competition context for the detail page.
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
	if status == league.StatusConfirmed && team > 0 && isSubmitter {
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
		CanSubmit:     status == league.StatusPending && team > 0,
		CanConfirm:    status == league.StatusConfirmed && team > 0 && !isSubmitter,
		CanDispute:    status == league.StatusConfirmed && team > 0 && !isSubmitter,
		CanEdit:       status == league.StatusPending && team > 0,
		CanWalkover:   canReportUnplayed(status, team),
		CanCorrect:    canCorrect,
		IsAdmin:       isAdmin,
		IsParticipant: isParticipant,
		StatusLabel:   statusLabel(status),
		StatusClass:   statusClass(status),
	}
}

// MatchDetail renders the match page with score, status, and available actions.
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

	submittedByName := playerNameIfSet(h.app, match.GetString("submitted_by"))
	confirmedByName := playerNameIfSet(h.app, match.GetString("confirmed_by"))
	disputedByName := playerNameIfSet(h.app, match.GetString("disputed_by"))
	shareText := buildShareText(match, pairNames)

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

// MatchSubmit processes a score submission from one of the participating pairs.
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

	myTeam, _ := league.PlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, "quorum_request", "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", match.Id)
	notify.EmailNotifyPlayers(h.app, rivalPlayers, "Resultado enviado", "Tu rival ha registrado un resultado. Confirma o disputa.", "/match/"+match.Id)

	return redirectHX(e, "/match/"+match.Id)
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

// AdminOverride lets an admin set the final score, bypassing the normal flow.
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

	if e.Request.FormValue("date") != "" {
		if err := h.validatePlayoffDates(match); err != nil {
			return alertError(e, "Las fechas de playoff deben respetar el orden del cuadro (una ronda posterior no puede ir antes que una previa)")
		}
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
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	userID := e.Auth.Id
	if _, err := league.PlayerTeam(h.app, userID, match); err != nil {
		return alertError(e, "No eres participante de este partido")
	}

	if match.GetString("review_type") == "walkover" {
		return redirectHX(e, "/match/"+id)
	}

	status := match.GetString("status")
	if status != league.StatusPending && status != league.StatusConfirmed {
		return alertError(e, "Este partido no puede reportarse como no jugado")
	}

	notes := match.GetString("dispute_notes")
	match.Set("review_type", "walkover")
	match.Set("walkover_requested_by", userID)
	match.Set("status", league.StatusDisputed)
	match.Set("dispute_notes", "[No jugado] "+notes)

	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al reportar")
	}

	if err := h.notifier.NotifyAdmins("dispute", "Partido no jugado", "Un jugador ha reportado un partido como no jugado.", id); err != nil {
		slog.Error("notify admins walkover report", "match", id, "err", err)
	}

	return redirectHX(e, "/match/"+id)
}

func playerNameIfSet(app core.App, userID string) string {
	if userID == "" {
		return ""
	}
	return league.PlayerName(app, userID)
}

func buildShareText(match *core.Record, pairNames map[string]string) string {
	if match.GetString("status") != league.StatusFinal {
		return ""
	}
	p1Name := pairNames[match.GetString("pair1")]
	p2Name := pairNames[match.GetString("pair2")]
	score := match.GetString("scores")
	winnerName := p2Name
	if match.GetString("winner") == match.GetString("pair1") {
		winnerName = p1Name
	}
	return url.QueryEscape(fmt.Sprintf("Resultado: %s %s %s. Ganador: %s!", p1Name, score, p2Name, winnerName))
}
