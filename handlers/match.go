package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
)

var (
	errNotParticipant    = errors.New("not participant")
	errWithdrawn         = errors.New("withdrawn")
	errMatchResolved     = errors.New("match already resolved")
	errArbitrationExists = errors.New("arbitration already requested")
	// errOtherSidePending: another side already holds a pending result proposal.
	errOtherSidePending = errors.New("other side has a pending result proposal")
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

// effectiveStatusBadge overrides the plain status label/class with "Arbitraje
// solicitado" (badge-warning) while a request is open — unless status is
// already disputed or final, which keep their own badge ("En disputa" wins
// over an open request; a final match can't have one open anyway).
func effectiveStatusBadge(status, arbitration string) (label, class string) {
	if arbitration != "" && status != league.StatusDisputed && status != league.StatusFinal {
		return "Arbitraje solicitado", "badge-warning"
	}
	return league.StatusLabel(status), statusClass(status)
}

// canRequestArbitration mirrors the status precondition RequestArbitration
// enforces: shown to a participant while the match isn't final and no
// arbitration request is already open.
func canRequestArbitration(status string, team int, openArbitration string) bool {
	return team > 0 && status != league.StatusFinal && openArbitration == ""
}

// matchRoundLabel returns the breadcrumb label for a match's round: the
// bracket round name ("Final", "Semifinal", ...) for playoffs, "Jornada N"
// for regular rounds, and "" for round 0 (leveled-league assignments).
func matchRoundLabel(app core.App, comp *core.Record, roundNum int) string {
	if maxRound, ok := league.PlayoffMaxRound(app, comp); ok {
		return bracketRoundName(roundNum, maxRound)
	}
	return league.RoundLabel(roundNum)
}

// MatchDetail renders the match page with score, status, and available actions.
func (h *MatchHandler) MatchDetail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Partido no encontrado")
	}
	if !matchVisibleTo(h.app, e, match) {
		return h.renderErrorPage(e, http.StatusNotFound, "Partido no encontrado")
	}

	userID := e.Auth.Id
	// The view decides, not the role: an admin in player view is a pure player.
	isAdmin := render.AdminView(e)

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	mode := PlayerReadOnly
	if isAdmin {
		mode = AdminReadOnly
	}
	mc := NewMatchCard(h.app, match, mode, userID)

	compID := match.GetString("competition")
	comp, _ := h.app.FindRecordById("competitions", compID)
	compName := ""
	roundLabel := league.RoundLabel(mc.RoundNum)
	if comp != nil {
		compName = comp.GetString("name")
		if !isAdmin && !league.PlayerCanModify(comp, time.Now()) {
			mc.CanSubmit = false
			mc.CanEdit = false
			mc.CanRequestArbitration = false
			mc.CanCorrect = false
		}
		roundLabel = matchRoundLabel(h.app, comp, mc.RoundNum)
	}

	matchPath := "/match/" + match.Id
	shareText, shareURL := buildShareText(h.app, match, render.RequestBaseURL(e), matchPath)
	venues, _ := h.app.FindRecordsByFilter("venues", "", "name", 0, 0, nil)
	mc.Venues = venues

	precedentes := buildPrecedentesView(h.app, match, mc.Pair1Name, mc.Pair2Name)

	canRelease := isAdmin && comp != nil && league.IsLeveled(comp) && league.IsPreScore(match.GetString("status"))

	return h.renderPage(e, "match.html", map[string]any{
		"PageTitle":           matchPageTitle(mc),
		"Card":                mc,
		"CompetitionName":     compName,
		"CompetitionID":       compID,
		"RoundLabel":          roundLabel,
		"ShareText":           shareText,
		"ShareURL":            shareURL,
		"Precedentes":         precedentes,
		"OGImage":             mc.CompetitionLogo,
		"FooterCompetitionID": compID,
		"CanRelease":          canRelease,
		"RivalContacts":       matchRivalContacts(h.app, match, mode, userID),
	})
}

// matchRivalContacts resolves the opposing pair's contacts for the "Contactar
// rivales" card, hidden from admins per design (they aren't a participant).
func matchRivalContacts(app core.App, match *core.Record, mode Mode, userID string) []league.PlayerContact {
	if mode.Admin {
		return nil
	}
	rivals, err := league.RivalContacts(app, match, userID)
	if err != nil {
		slog.Error("match detail: rival contacts", "match", match.Id, "err", err)
		return nil
	}
	return rivals
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
	LastProvisional      bool
	HasProvisional       bool
}

// buildPrecedentesView looks up the head-to-head record between a match's two
// pairs, excluding the match itself. Returns Show=false when either pair is
// unresolved (a playoff feeder slot) or the pairs have never met before.
func buildPrecedentesView(app core.App, match *core.Record, pair1Name, pair2Name string) PrecedentesView {
	pair1ID, pair2ID := match.GetString("pair1"), match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return PrecedentesView{}
	}
	summary, ok := league.Precedents(app, league.PrecedentsQuery{
		Pair1ID: pair1ID, Pair2ID: pair2ID,
		CompetitionID:  match.GetString("competition"),
		ExcludeMatchID: match.Id,
	})
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

		LastProvisional: summary.LastProvisional,

		HasProvisional: summary.HasProvisional,
	}
}

// matchPageTitle builds the browser-tab title from the match's pair names,
// falling back to the same "Por definir" placeholder the page header uses
// for a not-yet-decided playoff slot.
func matchPageTitle(mc MatchCard) string {
	p1, p2 := mc.Pair1Name, mc.Pair2Name
	if p1 == "" {
		p1 = "Por definir"
	}
	if p2 == "" {
		p2 = "Por definir"
	}
	return p1 + " vs " + p2
}

// MatchSubmit processes a score submission from one of the participating pairs.
func (h *MatchHandler) MatchSubmit(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	if match.GetString("pair1") == "" || match.GetString("pair2") == "" {
		return alertError(e, "Este partido aun no tiene parejas asignadas")
	}

	userID := e.Auth.Id
	isAdmin := render.AdminView(e)
	if !isAdmin {
		if _, err := playerActionGate(h.app, userID, match); err != nil {
			return mapActionGateError(e, err)
		}
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

	scores, err := readScoreForm(e, match.GetString("carried_sets"), "scores")
	if err != nil || scores == "" {
		return err
	}

	if err := h.submitResultProposal(match, userID, scores); err != nil {
		return submitErrorAlert(e, err)
	}
	h.notifyResultProposal(match, userID, scores)
	return redirectHX(e, "/match/"+match.Id)
}

func submitErrorAlert(e *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, errOtherSidePending):
		return alertError(e, "Ya hay una propuesta de resultado del rival pendiente. Revisa el hilo del partido para aceptar o rechazar.")
	case errors.Is(err, league.ErrMatchNotPreScore):
		return alertError(e, "Este partido ya tiene un resultado registrado")
	default:
		return alertError(e, "Error al guardar el resultado")
	}
}

// submitResultProposal supersedes the author's own pending proposals and creates
// the new one, after checking inside the same transaction that the match is
// still pre-score and no other side holds a pending proposal. PocketBase runs
// write transactions on a single connection, so the check and the insert are
// atomic: two sides racing cannot both create a pending proposal.
func (h *MatchHandler) submitResultProposal(match *core.Record, userID, scores string) error {
	return h.app.RunInTransaction(func(txApp core.App) error {
		fresh, err := txApp.FindRecordById("matches", match.Id)
		if err != nil {
			return err
		}
		if !league.IsPreScore(fresh.GetString("status")) {
			return league.ErrMatchNotPreScore
		}
		if err := ensureNoOtherSidePending(txApp, fresh, userID); err != nil {
			return err
		}
		// Supersede before creating the new proposal so the new one is not caught.
		h.supersedePendingResultsTx(txApp, fresh.Id, userID)
		col, err := txApp.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		pdJSON, _ := json.Marshal(ProposalData{Scores: scores})
		proposal := core.NewRecord(col)
		proposal.Set("match", fresh.Id)
		proposal.Set("author", userID)
		proposal.Set("type", "result_submission")
		proposal.Set("content", scores)
		proposal.Set("proposal_status", "pending")
		proposal.Set("proposal_data", string(pdJSON))
		if err := txApp.Save(proposal); err != nil {
			return err
		}
		fresh.Set("submitted_by", userID)
		fresh.SetRaw("submitted_at", types.NowDateTime()) // autodate: Set is a no-op
		fresh.Set("confirm_reminded", false)
		return txApp.Save(fresh)
	})
}

// ensureNoOtherSidePending returns errOtherSidePending when a pending result
// proposal exists whose author is on a different side than userID. Sides are
// pair1, pair2 and "neither" (an admin submitting for the match): an admin
// is not a pair, so any other author's pending proposal conflicts with theirs.
func ensureNoOtherSidePending(app core.App, match *core.Record, userID string) error {
	pending, err := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending' && author != {:uid}",
		"", 0, 0, map[string]any{"mid": match.Id, "uid": userID})
	if err != nil {
		return err
	}
	mySide := league.AuthorSide(app, match, userID)
	for _, p := range pending {
		if side := league.AuthorSide(app, match, p.GetString("author")); side != mySide || side == league.SideNeither {
			return errOtherSidePending
		}
	}
	return nil
}

// supersedePendingResultsTx supersedes the pending result proposals of the
// match: only authorID's when given, every author's when empty.
func (h *MatchHandler) supersedePendingResultsTx(app core.App, matchID, authorID string) {
	filter := "match = {:mid} && type = 'result_submission' && proposal_status = 'pending'"
	params := map[string]any{"mid": matchID}
	if authorID != "" {
		filter += " && author = {:uid}"
		params["uid"] = authorID
	}
	pending, _ := app.FindRecordsByFilter("match_messages", filter, "", 0, 0, params)
	for _, p := range pending {
		p.Set("proposal_status", "superseded")
		if err := app.Save(p); err != nil {
			slog.Error("supersede result proposal", "msg", p.Id, "err", err)
		}
	}
}

// playersOtherThanAuthorSide returns the players a result action must tell: the
// opposing pair's, or both pairs' when the author is on neither side (an admin).
func playersOtherThanAuthorSide(app core.App, match *core.Record, authorID string) []string {
	pair1, pair2 := match.GetString("pair1"), match.GetString("pair2")
	switch league.AuthorSide(app, match, authorID) {
	case league.Side1:
		return league.PlayersForPair(app, pair2)
	case league.Side2:
		return league.PlayersForPair(app, pair1)
	}
	return append(league.PlayersForPair(app, pair1), league.PlayersForPair(app, pair2)...)
}

func (h *MatchHandler) notifyResultProposal(match *core.Record, userID, scores string) {
	rivalPlayers := playersOtherThanAuthorSide(h.app, match, userID)
	submitterLabel := pairPlayerLabel(h.app, userID, match)
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	n := league.NotifResultSubmitted(match.Id, submitterLabel, compName, scores)
	h.notifier.NotifyPlayers(rivalPlayers, n)

	participants := matchParticipantUserIDs(h.app, match)
	an := league.NotifAdminMatchProgress(match.Id, "Resultado propuesto: "+scores)
	if err := h.notifier.NotifyAdmins(an, participants...); err != nil {
		slog.Error("notify admins match progress failed", "match", match.Id, "err", err)
	}
}

// releaseMatchTx deletes a match and its notifications, re-checking on a fresh
// record that it is still unplayed.
func releaseMatchTx(txApp core.App, id string) error {
	match, err := txApp.FindRecordById("matches", id)
	if err != nil {
		return err
	}
	if !league.IsPreScore(match.GetString("status")) {
		return league.ErrMatchNotPreScore
	}
	notifs, err := txApp.FindRecordsByFilter("notifications", "related_match = {:m}", "", 0, 0, map[string]any{"m": id})
	if err != nil {
		return err
	}
	for _, n := range notifs {
		if err := txApp.Delete(n); err != nil {
			return err
		}
	}
	return txApp.Delete(match)
}

// AdminRelease deletes a pending leveled-league match so the pair-assignment
// cron can re-pair the affected pairs. Only valid for leveled competitions and
// pre-score match statuses.
func (h *MatchHandler) AdminRelease(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", id)
	if err != nil {
		return alertError(e, "Partido no encontrado")
	}

	compID := match.GetString("competition")
	comp, _ := h.app.FindRecordById("competitions", compID)

	if comp == nil || !league.IsLeveled(comp) || !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Solo se puede liberar un partido sin resultado de una liga nivelada")
	}

	if err := h.app.RunInTransaction(func(txApp core.App) error { return releaseMatchTx(txApp, id) }); err != nil {
		if errors.Is(err, league.ErrMatchNotPreScore) {
			return alertError(e, "Solo se puede liberar un partido sin resultado de una liga nivelada")
		}
		return alertError(e, "Error al liberar el partido")
	}

	league.LogCompetitionEvent(h.app, league.CompetitionEvent{
		CompetitionID: compID,
		ActorID:       e.Auth.Id,
		Kind:          "assignment_released",
		Detail:        "liberó un partido de liga nivelada",
	})

	flash(e, "Partido liberado")
	return redirectHX(e, "/competition/"+compID)
}

func detectFieldChange(match *core.Record, field, newVal, label string) []string {
	if newVal == "" {
		return nil
	}
	old := match.GetString(field)
	normalizedOld := strings.Split(strings.TrimSpace(old), " ")[0]
	normalizedNew := strings.Split(strings.TrimSpace(newVal), " ")[0]
	if normalizedNew == normalizedOld {
		return nil
	}
	match.Set(field, newVal)
	if old == "" {
		return []string{label + " establecida: " + newVal}
	}
	return []string{label + " cambiada: " + normalizedOld + " → " + normalizedNew}
}

// arbitrationCategories are the valid values for the "category" form field,
// matched against league.Arbitration* constants.
var arbitrationCategories = map[string]bool{
	league.ArbitrationResult:      true,
	league.ArbitrationScheduling:  true,
	league.ArbitrationAbandonment: true,
	league.ArbitrationNoShow:      true,
	league.ArbitrationOther:       true,
}

// RequestArbitration lets a participant flag a match for admin review, with a
// category and a required explanation. Category no_show additionally sets
// review_type=walkover (so the existing WalkoverApprove flow keeps working)
// and moves the match to disputed, exactly as the walkover report used to;
// result does the same. scheduling/abandonment/other leave status untouched
// so the pairs can keep negotiating while the admin looks into it.
func (h *MatchHandler) RequestArbitration(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	userID := e.Auth.Id
	requesterTeam, err := league.PlayerTeam(h.app, userID, match)
	if err != nil {
		return alertError(e, "No eres participante de este partido")
	}
	if err := checkNotWithdrawn(h.app, e, match, requesterTeam); err != nil {
		return err
	}
	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}
	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if match.GetString("arbitration") != "" {
		return redirectHX(e, "/match/"+id)
	}
	if match.GetString("status") == league.StatusFinal {
		return alertError(e, "Este partido ya está resuelto")
	}

	category := e.Request.FormValue("category")
	if !arbitrationCategories[category] {
		return alertError(e, "Selecciona un motivo válido")
	}
	notes := strings.TrimSpace(e.Request.FormValue("notes"))
	if notes == "" {
		return alertError(e, "Explica el motivo")
	}

	match, err = h.saveArbitration(match.Id, category, notes, userID)
	switch {
	case errors.Is(err, errMatchResolved):
		return alertError(e, "Este partido ya está resuelto")
	case errors.Is(err, errArbitrationExists):
		return redirectHX(e, "/match/"+id)
	case err != nil:
		return alertError(e, "Error al solicitar arbitraje")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: userID, Kind: "result_event",
		Detail: "solicitó arbitraje: " + league.ArbitrationLabel(category) + " — " + notes,
	})
	h.notifyArbitrationRequested(match, category)

	return redirectHX(e, "/match/"+id)
}

// saveArbitration re-reads the match inside a transaction: a concurrent accept
// may have finalized it, and saving a stale record would overwrite that.
func (h *MatchHandler) saveArbitration(matchID, category, notes, userID string) (*core.Record, error) {
	var saved *core.Record
	err := h.app.RunInTransaction(func(txApp core.App) error {
		fresh, err := txApp.FindRecordById("matches", matchID)
		if err != nil {
			return err
		}
		if fresh.GetString("status") == league.StatusFinal {
			return errMatchResolved
		}
		if fresh.GetString("arbitration") != "" {
			return errArbitrationExists
		}
		applyArbitrationFields(fresh, category, notes, userID)
		saved = fresh
		return txApp.Save(fresh)
	})
	return saved, err
}

// applyArbitrationFields sets the fields RequestArbitration writes on match.
// category no_show additionally sets review_type=walkover (so the existing
// WalkoverApprove flow keeps working) and moves the match to disputed,
// exactly as the walkover report used to; result does the same.
// scheduling/abandonment/other leave status untouched so the pairs can keep
// negotiating while the admin looks into it.
func applyArbitrationFields(match *core.Record, category, notes, userID string) {
	match.Set("arbitration", category)
	match.Set("arbitration_by", userID)
	match.Set("dispute_notes", notes)
	if category == league.ArbitrationNoShow {
		match.Set("review_type", "walkover")
		match.Set("walkover_requested_by", userID)
	}
	if category == league.ArbitrationNoShow || category == league.ArbitrationResult {
		match.Set("status", league.StatusDisputed)
	}
}

func (h *MatchHandler) notifyArbitrationRequested(match *core.Record, category string) {
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	an := league.NotifArbitrationRequested(match.Id, league.ArbitrationLabel(category), compName)
	if err := h.notifier.NotifyAdmins(an); err != nil {
		slog.Error("notify admins arbitration requested", "match", match.Id, "err", err)
	}
}

// CloseArbitration lets an admin close an open arbitration request without
// otherwise changing the match, for categories that don't need a status
// change (scheduling/abandonment/other) or once the admin has already
// resolved it another way (walkover approval, dispute resolution).
func (h *MatchHandler) CloseArbitration(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}
	if match.GetString("arbitration") == "" {
		return alertError(e, "Este partido no tiene arbitraje abierto")
	}
	category := match.GetString("arbitration")

	match.Set("arbitration", "")
	match.Set("arbitration_by", "")
	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al cerrar el arbitraje")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id, Kind: "admin_action",
		Detail: "cerró el arbitraje",
	})

	compName := league.CompetitionName(h.app, match.GetString("competition"))
	n := league.NotifArbitrationClosed(id, league.ArbitrationLabel(category), compName)
	h.notifier.NotifyPlayers(matchParticipantUserIDs(h.app, match), n)

	flash(e, "Arbitraje cerrado")
	return redirectHX(e, "/match/"+id)
}

func playerNameIfSet(app core.App, userID string) string {
	if userID == "" {
		return ""
	}
	return league.PlayerName(app, userID)
}

// CancelDate lets a participant cancel a scheduled match date, reverting status
// to pending and notifying the rival pair and admins.
func (h *MatchHandler) CancelDate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, id)
	if err != nil {
		return err
	}

	userID := e.Auth.Id
	myTeam, err := h.cancelDateGate(e, match)
	if err != nil {
		return err
	}

	reason := strings.TrimSpace(e.Request.FormValue("reason"))
	if reason == "" {
		return alertError(e, "Debes indicar un motivo")
	}

	var within24h bool
	err = h.app.RunInTransaction(func(txApp core.App) error {
		match, within24h, err = cancelDateTx(txApp, id, userID, reason)
		return err
	})
	if errors.Is(err, errDateNotScheduled) {
		return alertError(e, "Este partido no tiene fecha confirmada")
	}
	if err != nil {
		return alertError(e, "Error al cancelar la fecha")
	}

	h.notifyCancelDate(cancelInfo{match: match, team: myTeam, cancellerID: userID, reason: reason, within24h: within24h})
	return redirectHX(e, "/match/"+id)
}

// cancelDateGate validates that the caller may cancel the match's confirmed
// date and returns their team; the error is already rendered to the client.
func (h *MatchHandler) cancelDateGate(e *core.RequestEvent, match *core.Record) (int, error) {
	if match.GetString("status") != league.StatusScheduled {
		return 0, alertError(e, "Este partido no tiene fecha confirmada")
	}
	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil {
		return 0, alertError(e, "No eres participante de este partido")
	}
	if err := checkNotWithdrawn(h.app, e, match, myTeam); err != nil {
		return 0, err
	}
	if err := checkDocGate(h.app, e, match); err != nil {
		return 0, err
	}
	if err := checkCompModifiable(h.app, e, match); err != nil {
		return 0, err
	}
	if comp, cErr := h.app.FindRecordById("competitions", match.GetString("competition")); cErr == nil && league.IsPlayoff(comp) {
		return 0, alertError(e, "No se puede cancelar la fecha de un partido de playoff")
	}
	return myTeam, nil
}

type cancelInfo struct {
	match       *core.Record
	cancellerID string
	team        int
	reason      string
	within24h   bool
}

func (h *MatchHandler) notifyCancelDate(ci cancelInfo) {
	match := ci.match
	cancellerTeam := ci.team
	cancellerID := ci.cancellerID
	reason := ci.reason
	within24h := ci.within24h
	id := match.Id
	compName := league.CompetitionName(h.app, match.GetString("competition"))
	playerName := pairPlayerLabel(h.app, cancellerID, match)

	rivalPairID := match.GetString("pair2")
	if cancellerTeam == 2 {
		rivalPairID = match.GetString("pair1")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	h.notifier.NotifyPlayers(rivalPlayers, league.NotifDateCancelled(league.DateCancelledParams{
		MatchID: id, PlayerName: playerName, Reason: reason, CompName: compName,
	}))

	urgency := ""
	if within24h {
		urgency = " (MENOS DE 24H — revisar consecuencias)"
	}
	pairIDs := []string{match.GetString("pair1"), match.GetString("pair2")}
	names := league.PairNames(h.app, pairIDs)
	pair1Name := names[pairIDs[0]]
	pair2Name := names[pairIDs[1]]
	an := league.NotifAdminDateCancelled(league.DateCancelledParams{
		MatchID: id, PlayerName: playerName, Reason: reason, CompName: compName,
		Pair1Name: pair1Name, Pair2Name: pair2Name, Urgency: urgency,
	})
	if err := h.notifier.NotifyAdmins(an); err != nil {
		slog.Error("notify admins cancel date", "match", id, "err", err)
	}
}

var errDateNotScheduled = errors.New("match has no confirmed date")

// cancelDateTx cancels the confirmed date on a fresh copy of the match: the
// handler's gate read the match before the transaction, and a concurrent cancel,
// result or admin decision may have moved it since. It retires the accepted date
// proposals, clears the date and reminders, writes the timeline entry, and
// returns the saved match and whether the date was less than 24h away.
func cancelDateTx(txApp core.App, matchID, userID, reason string) (*core.Record, bool, error) {
	match, err := txApp.FindRecordById("matches", matchID)
	if err != nil {
		return nil, false, err
	}
	if match.GetString("status") != league.StatusScheduled {
		return nil, false, errDateNotScheduled
	}
	within24h := false
	if start, ok := league.MatchStart(match); ok {
		within24h = time.Until(start) < 24*time.Hour
	}
	accepted, err := txApp.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'",
		"", 0, 0, map[string]any{"mid": matchID})
	if err != nil {
		return nil, false, err
	}
	for _, sp := range accepted {
		sp.Set("proposal_status", "superseded")
		if err := txApp.Save(sp); err != nil {
			return nil, false, err
		}
	}
	match.Set("date", "")
	match.Set("time", "")
	match.Set("club", "")
	match.Set("status", league.StatusPending)
	match.Set("last_warn_level", 0)
	if err := txApp.Save(match); err != nil {
		return nil, false, err
	}
	league.ClearMatchReminders(txApp, matchID)
	detail := "canceló la fecha del partido: " + reason
	if within24h {
		detail = "canceló la fecha del partido (con menos de 24h): " + reason
	}
	addTimelineEntry(txApp, timelineEntry{
		MatchID: matchID, ActorID: userID,
		Kind: "scheduling_response", Detail: detail,
	})
	return match, within24h, nil
}

// playerActionGate validates that userID can perform a score action on match:
// they must be a participant and not withdrawn.
// Returns (team, nil) on success, or a sentinel error for the caller to map.
func playerActionGate(app core.App, userID string, match *core.Record) (int, error) {
	team, err := league.PlayerTeam(app, userID, match)
	if err != nil {
		return 0, errNotParticipant
	}
	pairID := match.GetString("pair1")
	if team == 2 {
		pairID = match.GetString("pair2")
	}
	withdrawn, err := league.IsWithdrawn(app, pairID, match.GetString("competition"))
	if err != nil {
		return 0, err
	}
	if withdrawn {
		return 0, errWithdrawn
	}
	return team, nil
}

// mapActionGateError translates playerActionGate errors to UI alert responses.
func mapActionGateError(e *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, errNotParticipant):
		return alertError(e, "No eres participante de este partido")
	case errors.Is(err, errWithdrawn):
		return alertError(e, "Tu pareja se ha retirado de esta competición")
	default:
		return alertError(e, "Error interno")
	}
}

func buildShareText(app core.App, match *core.Record, baseURL, matchPath string) (text, shareURL string) {
	if match.GetString("status") != league.StatusFinal {
		return "", ""
	}
	fullURL := baseURL + matchPath
	line := NewMatchCard(app, match, PlayerFull, "").SummaryLine()
	return url.QueryEscape(line + "\n" + fullURL), fullURL
}
