package handlers

import (
	"cmp"
	"encoding/json"
	"fmt"
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

func statusClass(status string) string {
	switch status {
	case league.StatusPending:
		return "badge-ghost"
	case league.StatusScheduled:
		return "badge-success"
	case league.StatusConfirmed:
		return "badge-warning"
	case league.StatusDisputed:
		return "badge-error"
	case league.StatusFinal:
		return "badge-success"
	}
	return "badge-ghost"
}

// canReportUnplayed mirrors the status precondition ReportUnplayed enforces.
func canReportUnplayed(status string, team int) bool {
	return team > 0 && (league.IsPreScore(status) || status == league.StatusConfirmed)
}

// matchRoundLabel returns the breadcrumb label for a match's round: the
// bracket round name ("Final", "Semifinal", ...) for playoffs, "Jornada N"
// otherwise.
func matchRoundLabel(app core.App, comp *core.Record, roundNum int) string {
	if maxRound, ok := league.PlayoffMaxRound(app, comp); ok {
		return bracketRoundName(roundNum, maxRound)
	}
	return fmt.Sprintf("Jornada %d", roundNum)
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

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	mode := PlayerReadOnly
	if render.AdminView(e) {
		mode = AdminReadOnly
	}
	mc := NewMatchCard(h.app, match, mode, userID)

	compID := match.GetString("competition")
	comp, _ := h.app.FindRecordById("competitions", compID)
	compName := ""
	roundLabel := fmt.Sprintf("Jornada %d", mc.RoundNum)
	if comp != nil {
		compName = comp.GetString("name")
		if !isAdmin && !league.PlayerCanModify(comp, time.Now()) {
			mc.CanSubmit = false
			mc.CanEdit = false
			mc.CanWalkover = false
			mc.CanCorrect = false
		}
		roundLabel = matchRoundLabel(h.app, comp, mc.RoundNum)
	}

	matchPath := "/match/" + match.Id
	shareText, shareURL := buildShareText(h.app, match, requestBaseURL(e), matchPath)
	venues, _ := h.app.FindRecordsByFilter("venues", "", "name", 0, 0, nil)
	mc.Venues = venues

	precedentes := buildPrecedentesView(h.app, match, mc.Pair1Name, mc.Pair2Name)

	return h.renderPage(e, "match.html", map[string]any{
		"PageTitle":       matchPageTitle(mc),
		"Card":            mc,
		"CompetitionName": compName,
		"CompetitionID":   compID,
		"RoundLabel":      roundLabel,
		"ShareText":       shareText,
		"ShareURL":        shareURL,
		"Precedentes":     precedentes,
	})
}

// PrecedentesView is the match-page view-model for the pair-vs-pair
// head-to-head strip. Show reports whether there is any prior meeting to
// display — the template hides the whole section when false.
type PrecedentesView struct {
	Show                 bool
	Pair1Name, Pair2Name string
	Pair1Wins, Pair2Wins int
	LastMatchID          string
	LastScore            string
}

// buildPrecedentesView looks up the head-to-head record between a match's two
// pairs, excluding the match itself. Returns Show=false when either pair is
// unresolved (a playoff feeder slot) or the pairs have never met before.
func buildPrecedentesView(app core.App, match *core.Record, pair1Name, pair2Name string) PrecedentesView {
	pair1ID, pair2ID := match.GetString("pair1"), match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return PrecedentesView{}
	}
	summary, ok := league.Precedents(app, pair1ID, pair2ID, match.Id)
	if !ok {
		return PrecedentesView{}
	}
	return PrecedentesView{
		Show:        true,
		Pair1Name:   pair1Name,
		Pair2Name:   pair2Name,
		Pair1Wins:   summary.Pair1Wins,
		Pair2Wins:   summary.Pair2Wins,
		LastMatchID: summary.LastMatchID,
		LastScore:   summary.LastScore,
	}
}

// matchPageTitle builds the browser-tab title from the match's pair names,
// falling back to the same "Por definir" placeholder the page header uses
// for a not-yet-decided playoff slot.
func matchPageTitle(mc MatchCard) string {
	p1, p2 := mc.Pair1Name, mc.Pair2Name
	if p1 == "" {
		p1 = cmp.Or(mc.Feeder1, "Por definir")
	}
	if p2 == "" {
		p2 = cmp.Or(mc.Feeder2, "Por definir")
	}
	return p1 + " vs " + p2
}

// MatchSubmit processes a score submission from one of the participating pairs.
func (h *MatchHandler) MatchSubmit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Record no encontrado")
	}

	if match.GetString("pair1") == "" || match.GetString("pair2") == "" {
		return alertError(e, "Este partido aun no tiene parejas asignadas")
	}

	userID := e.Auth.Id
	isAdmin := isEffectiveAdmin(e)
	_, teamErr := league.PlayerTeam(h.app, userID, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya tiene un resultado registrado")
	}

	if match.GetString("date") == "" || match.GetString("club") == "" {
		return alertError(e, "Primero acuerda una fecha y lugar para el partido")
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

	if h.rivalHasPendingResult(match, userID) {
		return alertError(e, "Ya hay una propuesta de resultado del rival pendiente. Revisa el hilo del partido para aceptar o rechazar.")
	}

	if err := h.submitResultProposal(e, match, userID, scores); err != nil {
		return err
	}
	return redirectHX(e, "/match/"+match.Id)
}

func (h *MatchHandler) submitResultProposal(e *core.RequestEvent, match *core.Record, userID, scores string) error {
	match.Set("submitted_by", userID)
	match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
	match.Set("confirm_reminded", false)
	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al guardar el resultado")
	}

	h.supersedeMyPendingResults(match.Id, userID)

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}
	pdJSON, _ := json.Marshal(ProposalData{Scores: scores})
	proposal := core.NewRecord(col)
	proposal.Set("match", match.Id)
	proposal.Set("author", userID)
	proposal.Set("type", "result_submission")
	proposal.Set("content", scores)
	proposal.Set("proposal_status", "pending")
	proposal.Set("proposal_data", string(pdJSON))
	if err := h.app.Save(proposal); err != nil {
		return alertError(e, "Error al crear la propuesta de resultado")
	}

	h.notifyResultProposal(match, userID, scores)
	return nil
}

func (h *MatchHandler) supersedeMyPendingResults(matchID, userID string) {
	pending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'pending'",
		"", 0, 0,
		map[string]any{"mid": matchID, "uid": userID})
	for _, p := range pending {
		p.Set("proposal_status", "superseded")
		if err := h.app.Save(p); err != nil {
			slog.Error("supersede result proposal", "msg", p.Id, "err", err)
		}
	}
}

func (h *MatchHandler) rivalHasPendingResult(match *core.Record, userID string) bool {
	myTeam, _ := league.PlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	for _, rp := range league.PlayersForPair(h.app, rivalPairID) {
		pending, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && author = {:uid} && proposal_status = 'pending'",
			"", 1, 0,
			map[string]any{"mid": match.Id, "uid": rp})
		if len(pending) > 0 {
			return true
		}
	}
	return false
}

func (h *MatchHandler) notifyResultProposal(match *core.Record, userID, scores string) {
	myTeam, _ := league.PlayerTeam(h.app, userID, match)
	rivalPairID := match.GetString("pair2")
	if myTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	myPairID := match.GetString("pair1")
	if myTeam == 2 {
		myPairID = match.GetString("pair2")
	}
	myPairName := league.PairNames(h.app, []string{myPairID})[myPairID]
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	n := league.NotifResultSubmitted(match.Id, myPairName, compName, scores)
	h.notifier.NotifyPlayers(rivalPlayers, n)
	h.notifier.EmailPlayers(rivalPlayers, n.Title, n.Body, "/match/"+match.Id)

	participants := matchParticipantUserIDs(h.app, match)
	an := league.NotifAdminMatchProgress(match.Id, "Resultado propuesto: "+scores)
	if err := h.notifier.NotifyAdmins(an, participants...); err != nil {
		slog.Error("notify admins match progress failed", "match", match.Id, "err", err)
	}
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

	addTimelineEntry(h.app, timelineEntry{
		MatchID: id, ActorID: e.Auth.Id,
		Kind: "admin_action", Detail: strings.Join(changes, "; "),
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
	dateChange := detectFieldChange(match, "date", e.Request.FormValue("date"), "Fecha")
	changes = append(changes, dateChange...)
	changes = append(changes, detectFieldChange(match, "time", e.Request.FormValue("time"), "Hora")...)
	changes = append(changes, h.detectVenueChange(match, e.Request.FormValue("venue_id"))...)
	changes = append(changes, detectFieldChange(match, "court_number", e.Request.FormValue("court_number"), "Pista")...)

	if len(dateChange) > 0 {
		match.Set("reminder_sent", false)
	}

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

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if match.GetString("review_type") == "walkover" {
		return redirectHX(e, "/match/"+id)
	}

	status := match.GetString("status")
	if !league.IsPreScore(status) && status != league.StatusConfirmed {
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

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID,
		Kind: "result_event", Detail: "reportó el partido como no jugado",
	})

	an := league.NotifAdminMatchUnplayed(id)
	if err := h.notifier.NotifyAdmins(an); err != nil {
		slog.Error("notify admins walkover report", "match", id, "err", err)
	}

	// Tell the rival pair a walkover was filed against them.
	rivalPairID := match.GetString("pair2")
	if reporterTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	n := league.NotifMatchReportedUnplayed(id)
	h.notifier.NotifyPlayers(rivalPlayers, n)
	h.notifier.EmailPlayers(rivalPlayers, n.Title, n.Body, "/match/"+id)

	return redirectHX(e, "/match/"+id)
}

func playerNameIfSet(app core.App, userID string) string {
	if userID == "" {
		return ""
	}
	return league.PlayerName(app, userID)
}

func buildShareText(app core.App, match *core.Record, baseURL, matchPath string) (text, shareURL string) {
	if match.GetString("status") != league.StatusFinal {
		return "", ""
	}
	fullURL := baseURL + matchPath
	line := NewMatchCard(app, match, PlayerFull, "").SummaryLine()
	return url.QueryEscape(line + "\n" + fullURL), fullURL
}
