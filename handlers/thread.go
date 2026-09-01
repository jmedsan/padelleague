package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// ThreadHandler handles the match thread: messages, scheduling proposals, and responses.
type ThreadHandler struct {
	app           core.App
	notifier      *notify.Notifier
	renderPage    RenderFunc
	renderPartial RenderFunc
}

// NewThreadHandler creates a ThreadHandler with the given dependencies.
func NewThreadHandler(app core.App, notifier *notify.Notifier, renderPage RenderFunc, renderPartial RenderFunc) *ThreadHandler {
	return &ThreadHandler{app: app, notifier: notifier, renderPage: renderPage, renderPartial: renderPartial}
}

// ThreadMessage holds a thread message record with display-ready fields.
type ThreadMessage struct {
	Record            *core.Record
	MatchID           string
	AuthorName        string
	AuthorTeam        int
	IsMyTeam          bool
	Type              string
	Content           string
	ProposalData      *ProposalData
	Status            string
	RejectionReason   string
	RejectionText     string
	CanRespond        bool
	CanChangeDecision bool
	DataType          string
	CreatedAt         string
	ParentID          string
	Responses         []ThreadMessage
	ScoreCounter      ScoreInputVM
}

// ProposalData holds parsed scheduling proposal details from a thread message.
type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
	Scores    string `json:"scores,omitempty"`
}

// ParseProposalData decodes a proposal from a raw JSON field value.
func ParseProposalData(raw any) *ProposalData {
	if raw == nil {
		return nil
	}
	var pd ProposalData
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(v), &pd); err != nil {
			return nil
		}
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &pd); err != nil {
			return nil
		}
	}
	return &pd
}

func (h *ThreadHandler) buildThreadMessages(match *core.Record, matchID string, myTeam int, compModifiable bool) []ThreadMessage {
	messages, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "created", 0, 0,
		map[string]any{"mid": matchID})

	pair1Players := league.PlayersForPair(h.app, match.GetString("pair1"))
	pair2Players := league.PlayersForPair(h.app, match.GetString("pair2"))
	pairNames := league.PairNames(h.app, []string{match.GetString("pair1"), match.GetString("pair2")})
	nameCache := make(map[string]string)

	bctx := threadBuildCtx{
		match: match, matchID: matchID, myTeam: myTeam, compModifiable: compModifiable,
		pair1Players: pair1Players, pair2Players: pair2Players,
		pair1Name: pairNames[match.GetString("pair1")], pair2Name: pairNames[match.GetString("pair2")],
		nameCache: nameCache,
	}
	byID := make(map[string]*ThreadMessage)
	var allMessages []ThreadMessage
	for _, msg := range messages {
		tm := h.toThreadMessage(msg, bctx)
		allMessages = append(allMessages, tm)
		byID[msg.Id] = &allMessages[len(allMessages)-1]
	}

	var topLevel []ThreadMessage
	for i := range allMessages {
		tm := &allMessages[i]
		if tm.ParentID != "" {
			if parent, ok := byID[tm.ParentID]; ok {
				parent.Responses = append(parent.Responses, *tm)
				continue
			}
		}
		topLevel = append(topLevel, *tm)
	}
	return topLevel
}

type threadBuildCtx struct {
	match                      *core.Record
	matchID                    string
	myTeam                     int
	compModifiable             bool
	pair1Players, pair2Players []string
	pair1Name, pair2Name       string
	nameCache                  map[string]string
}

func (h *ThreadHandler) toThreadMessage(msg *core.Record, ctx threadBuildCtx) ThreadMessage {
	authorID := msg.GetString("author")
	authorTeam := playerTeamOf(authorID, ctx.pair1Players, ctx.pair2Players)

	if _, ok := ctx.nameCache[authorID]; !ok {
		ctx.nameCache[authorID] = league.PlayerName(h.app, authorID)
	}

	msgType := msg.GetString("type")
	var pd *ProposalData
	if msgType == "scheduling_proposal" || msgType == "result_submission" {
		pd = ParseProposalData(msg.GetString("proposal_data"))
	}

	authorName := ctx.nameCache[authorID]
	if msgType == "scheduling_proposal" || msgType == "result_submission" || msgType == "scheduling_response" || msgType == "result_response" {
		authorName = pairPlayerLabel(h.app, authorID, ctx.match)
	}

	status := msg.GetString("proposal_status")
	canRespond, canChangeDecision := proposalActions(msgType, ctx.match.GetString("status"), authorTeam == ctx.myTeam || ctx.myTeam == 0, status)
	canRespond = canRespond && ctx.compModifiable
	canChangeDecision = canChangeDecision && ctx.compModifiable

	tm := ThreadMessage{
		Record:            msg,
		MatchID:           ctx.matchID,
		AuthorName:        authorName,
		AuthorTeam:        authorTeam,
		IsMyTeam:          ctx.myTeam != 0 && authorTeam == ctx.myTeam,
		Type:              msgType,
		Content:           msg.GetString("content"),
		ProposalData:      pd,
		Status:            status,
		RejectionReason:   msg.GetString("rejection_reason"),
		RejectionText:     msg.GetString("rejection_text"),
		CanRespond:        canRespond,
		CanChangeDecision: canChangeDecision,
		DataType:          dataTypeFor(msgType),
		CreatedAt:         msg.GetDateTime("created").Time().Format("02/01 15:04"),
		ParentID:          msg.GetString("parent"),
	}
	if canRespond && msgType == "result_submission" {
		tm.ScoreCounter = ScoreInputVM{FieldName: "counter_scores", IDSuffix: ctx.matchID + "-counter-" + msg.Id, Pair1Name: ctx.pair1Name, Pair2Name: ctx.pair2Name}
	}
	return tm
}

func dataTypeFor(msgType string) string {
	switch msgType {
	case "scheduling_proposal", "scheduling_response", "availability":
		return "schedule"
	case "result_submission", "result_response", "result_event", "admin_action":
		return "result"
	default:
		return "message"
	}
}

func playerTeamOf(uid string, pair1Players, pair2Players []string) int {
	for _, p := range pair1Players {
		if p == uid {
			return 1
		}
	}
	for _, p := range pair2Players {
		if p == uid {
			return 2
		}
	}
	return 0
}

func proposalActions(msgType, matchStatus string, sameTeamOrOutsider bool, proposalStatus string) (canRespond, canChange bool) {
	isProposal := msgType == "scheduling_proposal" || msgType == "result_submission"
	canAct := isProposal &&
		league.IsPreScore(matchStatus) &&
		!sameTeamOrOutsider
	return canAct && proposalStatus == "pending",
		canAct && (proposalStatus == "accepted" || proposalStatus == "rejected")
}

// Thread renders the full match thread page with messages and proposals.
func (h *ThreadHandler) Thread(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return e.HTML(http.StatusOK, `<div class="text-center py-6 opacity-60">Parejas pendientes de asignacion</div>`)
	}

	isAdmin := isEffectiveAdmin(e)
	myTeam, teamErr := league.PlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No tienes acceso a este hilo")
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	isParticipant := myTeam != 0
	canPost := isParticipant || isAdmin
	isPlayoff := false
	compModifiable := true
	if comp, err := h.app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		isPlayoff = league.IsPlayoff(comp)
		compModifiable = isAdmin || league.PlayerCanModify(comp, time.Now())
	}

	threadMessages := h.buildThreadMessages(match, matchID, myTeam, compModifiable)

	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)
	canPropose := isParticipant && league.IsPreScore(match.GetString("status")) && !isPlayoff && compModifiable

	var unpaidWarning string
	if canPropose {
		unpaidWarning = h.checkUnpaid(match, pair1ID, pair2ID)
	}

	return h.renderPartial(e, "thread.html", map[string]any{
		"MatchID":              matchID,
		"Messages":             threadMessages,
		"Venues":               venues,
		"CanPost":              canPost,
		"CanPropose":           canPropose,
		"IsAdmin":              isAdmin,
		"IsParticipant":        isParticipant,
		"IsPlayoff":            isPlayoff,
		"CompModifiable":       compModifiable,
		"IsScheduled":          match.GetString("status") == league.StatusScheduled,
		"HasDateAndPlace":      match.GetString("date") != "" && match.GetString("club") != "",
		"Match":                match,
		"UnpaidWarning":        unpaidWarning,
		"ProposalDefaultVenue": "",
		"ProposalDefaultTime":  "20:00",
	})
}

func (h *ThreadHandler) checkUnpaid(match *core.Record, pair1ID, pair2ID string) string {
	comp, err := h.app.FindRecordById("competitions", match.GetString("competition"))
	if err != nil {
		return ""
	}
	ps := make(map[string]bool)
	_ = comp.UnmarshalJSONField("payment_status", &ps)
	if !ps[pair1ID] || !ps[pair2ID] {
		return "Una o ambas parejas no han pagado la inscripción"
	}
	return ""
}

// ThreadMessages returns the HTMX partial with updated thread messages.
func (h *ThreadHandler) ThreadMessages(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	isAdmin := isEffectiveAdmin(e)
	myTeam, teamErr := league.PlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return alertError(e, "No tienes acceso a este hilo")
	}

	compModifiable := true
	if comp, err := h.app.FindRecordById("competitions", match.GetString("competition")); err == nil {
		compModifiable = isAdmin || league.PlayerCanModify(comp, time.Now())
	}

	threadMessages := h.buildThreadMessages(match, matchID, myTeam, compModifiable)

	return h.renderPartial(e, "thread-messages.html", map[string]any{
		"MatchID":  matchID,
		"Messages": threadMessages,
	})
}

// PostMessage handles POST to add a new chat message to the match thread.
func (h *ThreadHandler) PostMessage(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	myTeam, _ := league.PlayerTeam(h.app, e.Auth.Id, match)
	isAdmin := isEffectiveAdmin(e)
	if myTeam == 0 && !isAdmin {
		return alertError(e, "No eres participante de este partido")
	}

	content := e.Request.FormValue("content")
	if content == "" {
		return alertError(e, "El mensaje no puede estar vacío")
	}

	msgType := e.Request.FormValue("type")
	if msgType != "chat" && msgType != "score_discussion" {
		msgType = "chat"
	}

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", msgType)
	record.Set("content", content)

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al enviar mensaje")
	}

	// A player notifies the rival team; an admin (not on either team) notifies both.
	var recipients []string
	if myTeam == 0 {
		recipients = append(league.PlayersForPair(h.app, match.GetString("pair1")),
			league.PlayersForPair(h.app, match.GetString("pair2"))...)
	} else {
		rivalPairID := match.GetString("pair1")
		if myTeam == 1 {
			rivalPairID = match.GetString("pair2")
		}
		recipients = league.PlayersForPair(h.app, rivalPairID)
	}
	authorName := league.PlayerName(h.app, e.Auth.Id)
	h.notifier.NotifyPlayers(recipients, league.NotifNewMessage(matchID, authorName, content))

	return redirectHX(e, "/match/"+matchID)
}

// PostProposal creates a scheduling proposal message in the match thread.
func (h *ThreadHandler) PostProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Solo se pueden proponer fechas para partidos pendientes")
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return alertError(e, "Este partido aun no tiene parejas asignadas")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	pd, err := h.parseProposalForm(e)
	if err != nil {
		return err
	}
	pdJSON, _ := json.Marshal(pd)

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", "scheduling_proposal")
	record.Set("proposal_data", string(pdJSON))
	record.Set("proposal_status", "pending")

	if err := h.app.Save(record); err != nil {
		return alertError(e, "Error al crear propuesta")
	}

	h.notifyProposal(match, myTeam, proposalNotice{AuthorID: e.Auth.Id, Date: pd.Date, Time: pd.Time, VenueName: pd.VenueName})
	return redirectHX(e, "/match/"+matchID)
}

type proposalNotice struct {
	AuthorID, Date, Time, VenueName string
}

func (h *ThreadHandler) notifyProposal(match *core.Record, myTeam int, n proposalNotice) {
	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := league.PlayersForPair(h.app, rivalPairID)
	authorName := league.PlayerName(h.app, n.AuthorID)
	notif := league.NotifProposal(league.ProposalParams{
		MatchID: match.Id, AuthorName: authorName, Date: n.Date, Time: n.Time, VenueName: n.VenueName,
	})
	h.notifier.NotifyPlayers(rivalPlayers, notif)
	h.notifier.EmailPlayers(rivalPlayers, notif.Title, notif.Body, "/match/"+match.Id)
}

func (h *ThreadHandler) parseProposalForm(e *core.RequestEvent) (ProposalData, error) {
	date := e.Request.FormValue("date")
	timeVal := e.Request.FormValue("time")
	venueID := e.Request.FormValue("venue_id")
	venueText := e.Request.FormValue("venue_text")

	if date == "" || timeVal == "" {
		return ProposalData{}, alertError(e, "Fecha y hora son obligatorias")
	}

	venueID, venueName := h.resolveVenue(venueID, venueText)
	return ProposalData{
		Date:      date,
		Time:      timeVal,
		VenueID:   venueID,
		VenueName: venueName,
		VenueText: venueText,
	}, nil
}

// RespondProposal records a player's accept or reject on a scheduling proposal.
func (h *ThreadHandler) RespondProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return err
	}

	if err := checkDocGate(h.app, e, match); err != nil {
		return err
	}

	if err := checkCompModifiable(h.app, e, match); err != nil {
		return err
	}

	if !league.IsPreScore(match.GetString("status")) {
		return alertError(e, "Este partido ya no acepta propuestas")
	}

	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return alertError(e, "No eres participante de este partido")
	}

	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return alertError(e, "Propuesta no encontrada")
	}

	if msg.GetString("match") != matchID {
		return alertError(e, "Propuesta no pertenece a este partido")
	}

	proposalStatus := msg.GetString("proposal_status")
	if proposalStatus == "accepted" {
		return redirectHX(e, "/match/"+matchID)
	}
	if proposalStatus != "pending" {
		return alertError(e, "Esta propuesta ya fue respondida")
	}

	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return alertError(e, "No puedes responder a tu propia propuesta")
	}

	if err := h.dispatchProposalAction(e, match, msg, authorTeam); err != nil {
		return err
	}
	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) dispatchProposalAction(e *core.RequestEvent, match, msg *core.Record, authorTeam int) error {
	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}
	action := e.Request.FormValue("action")

	msgType := msg.GetString("type")
	if msgType == "result_submission" {
		switch action {
		case "accept":
			return h.acceptResultProposal(e, match, msg, proposerPairID)
		case "reject":
			return h.rejectResultProposal(e, match, msg, proposerPairID)
		default:
			return alertError(e, "Acción no válida")
		}
	}

	switch action {
	case "accept":
		return h.acceptProposal(e, match, msg, proposerPairID)
	case "reject":
		return h.rejectProposal(e, msg, match, proposerPairID)
	default:
		return alertError(e, "Acción no válida")
	}
}

func (h *ThreadHandler) acceptProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	existing, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && proposal_status = 'accepted'",
		"", 0, 0,
		map[string]any{"mid": match.Id})
	if len(existing) > 0 && match.GetString("status") != league.StatusScheduled {
		return alertError(e, "Ya hay una propuesta aceptada para este partido")
	}
	for _, old := range existing {
		old.Set("proposal_status", "superseded")
		if err := h.app.Save(old); err != nil {
			_ = h.notifier.NotifyAdmins(league.Notification{
				Type: "admin_message", Title: "Error al reemplazar propuesta",
				Body: "No se pudo marcar la propuesta anterior como reemplazada", MatchID: match.Id,
			})
		}
	}

	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}

	match.Set("date", pd.Date)
	match.Set("time", pd.Time)
	match.Set("club", pd.VenueName)
	match.Set("status", league.StatusScheduled)
	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al actualizar el partido")
	}

	msg.Set("proposal_status", "accepted")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al marcar la propuesta como aceptada")
	}

	h.supersedePendingAndNotify(match.Id, msg.Id)

	responderLabel := pairPlayerLabel(h.app, e.Auth.Id, match)
	proposerName := league.PlayerName(h.app, msg.GetString("author"))
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind:     "scheduling_response",
		Detail:   responderLabel + " aceptó la propuesta de " + proposerName + " (" + pd.Date + ", " + pd.Time + ", " + pd.VenueName + ")",
		ParentID: msg.Id,
	})

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	notif := league.NotifProposalAccepted(match.Id, league.PlayerName(h.app, e.Auth.Id), pd.Date, pd.Time)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifier.EmailPlayers(proposerPlayers, notif.Title, notif.Body, "/match/"+match.Id)
	return nil
}

func (h *ThreadHandler) rejectProposal(e *core.RequestEvent, msg *core.Record, match *core.Record, proposerPairID string) error {
	reason := e.Request.FormValue("rejection_reason")
	text := e.Request.FormValue("rejection_text")

	msg.Set("proposal_status", "rejected")
	msg.Set("rejection_reason", reason)
	msg.Set("rejection_text", text)
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al rechazar la propuesta")
	}

	responderLabel := pairPlayerLabel(h.app, e.Auth.Id, match)
	proposerName := league.PlayerName(h.app, msg.GetString("author"))
	detail := responderLabel + " rechazó la propuesta de " + proposerName
	if text != "" {
		detail += ": " + text
	} else if reason != "" {
		detail += ": " + reason
	}
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "scheduling_response", Detail: detail,
		ParentID: msg.Id,
	})

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	notif := league.NotifProposalRejected(match.Id, league.PlayerName(h.app, e.Auth.Id), reason)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifier.EmailPlayers(proposerPlayers, notif.Title, notif.Body, "/match/"+match.Id)
	return nil
}

func (h *ThreadHandler) acceptResultProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil || pd.Scores == "" {
		return alertError(e, "Error al leer los datos de la propuesta")
	}

	winner, err := league.DetermineWinner(match, pd.Scores)
	if err != nil {
		return alertError(e, "Error al determinar el ganador")
	}

	match.Set("scores", pd.Scores)
	match.Set("winner", winner)
	match.Set("status", league.StatusFinal)
	if err := h.app.Save(match); err != nil {
		return alertError(e, "Error al finalizar el partido")
	}

	msg.Set("proposal_status", "accepted")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al marcar la propuesta como aceptada")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "result_response", Detail: "✓ Resultado aceptado: " + pd.Scores,
		ParentID: msg.Id,
	})

	h.supersedePendingResults(match.Id, msg.Id)

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	n := league.NotifResultConfirmed(match.Id)
	h.notifier.NotifyPlayers(proposerPlayers, n)
	h.notifier.EmailPlayers(proposerPlayers, n.Title, n.Body, "/match/"+match.Id)
	return nil
}

func (h *ThreadHandler) supersedePendingResults(matchID, excludeMsgID string) {
	pending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending' && id != {:eid}",
		"", 0, 0,
		map[string]any{"mid": matchID, "eid": excludeMsgID})
	for _, p := range pending {
		p.Set("proposal_status", "superseded")
		if err := h.app.Save(p); err != nil {
			slog.Error("supersede result proposal", "id", p.Id, "err", err)
		}
	}
}

func (h *ThreadHandler) rejectResultProposal(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	counterScores := e.Request.FormValue("counter_scores")
	if counterScores == "" {
		return alertError(e, "Debes proponer un marcador alternativo")
	}
	if _, err := league.ParseScore(counterScores); err != nil {
		return alertError(e, "Marcador no válido")
	}

	msg.Set("proposal_status", "superseded")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al rechazar la propuesta")
	}

	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "result_response", Detail: "✗ Resultado rechazado: " + msg.GetString("content"),
		ParentID: msg.Id,
	})

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return alertError(e, "Error interno")
	}
	pdJSON, _ := json.Marshal(ProposalData{Scores: counterScores})
	counter := core.NewRecord(col)
	counter.Set("match", match.Id)
	counter.Set("author", e.Auth.Id)
	counter.Set("type", "result_submission")
	counter.Set("content", counterScores)
	counter.Set("proposal_status", "pending")
	counter.Set("proposal_data", string(pdJSON))
	if err := h.app.Save(counter); err != nil {
		return alertError(e, "Error al crear la contrapropuesta")
	}

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	notif := league.NotifResultCountered(match.Id)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifier.EmailPlayers(proposerPlayers, notif.Title, notif.Body, "/match/"+match.Id)
	return nil
}

func (h *ThreadHandler) resolveVenue(venueID, venueText string) (string, string) {
	if venueID == "" || venueID == "otro" {
		return "", venueText
	}
	venue, err := h.app.FindRecordById("venues", venueID)
	if err != nil {
		return "", venueText
	}
	return venueID, venue.GetString("name")
}

func (h *ThreadHandler) supersedePending(matchID, excludeMsgID string) error {
	otherPending, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
		"", 0, 0,
		map[string]any{"mid": matchID, "msgid": excludeMsgID})
	var failedIDs []string
	for _, other := range otherPending {
		other.Set("proposal_status", "superseded")
		if err := h.app.Save(other); err != nil {
			slog.Error("supersede proposal", "id", other.Id, "err", err)
			failedIDs = append(failedIDs, other.Id)
		}
	}
	if len(failedIDs) > 0 {
		return fmt.Errorf("failed to supersede proposals: %v", failedIDs)
	}
	return nil
}

func (h *ThreadHandler) supersedePendingAndNotify(matchID, excludeMsgID string) {
	if err := h.supersedePending(matchID, excludeMsgID); err != nil {
		slog.Error("supersede pending proposals", "match", matchID, "err", err)
		n := league.NotifAdminSupersedeFailed(matchID)
		_ = h.notifier.NotifyAdmins(n)
	}
}

func (h *ThreadHandler) revokeAcceptance(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	msg.Set("proposal_status", "rejected")
	msg.Set("rejection_reason", "Decisión cambiada")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al cambiar la decisión")
	}
	match.Set("date", "")
	match.Set("time", "")
	match.Set("club", "")
	match.Set("status", league.StatusPending)
	if err := h.app.Save(match); err != nil {
		slog.Error("save match after rejection", "match", match.Id, "err", err)
		return alertError(e, "Error al actualizar el partido")
	}

	responderLabel := pairPlayerLabel(h.app, e.Auth.Id, match)
	proposerName := league.PlayerName(h.app, msg.GetString("author"))
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind: "scheduling_response", Detail: responderLabel + " revocó la aceptación de la propuesta de " + proposerName,
		ParentID: msg.Id,
	})

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	notif := league.NotifDecisionChangedToRejected(match.Id, league.PlayerName(h.app, e.Auth.Id))
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifier.EmailPlayers(proposerPlayers, notif.Title, notif.Body, "/match/"+match.Id)
	return nil
}

func (h *ThreadHandler) changeToAccepted(e *core.RequestEvent, match, msg *core.Record, proposerPairID string) error {
	existing, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && proposal_status = 'accepted'",
		"", 1, 0,
		map[string]any{"mid": match.Id})
	if len(existing) > 0 {
		return alertError(e, "Ya hay otra propuesta aceptada")
	}
	pd := ParseProposalData(msg.Get("proposal_data"))
	if pd == nil {
		return alertError(e, "Error al leer los datos de la propuesta")
	}
	msg.Set("proposal_status", "accepted")
	msg.Set("rejection_reason", "")
	msg.Set("rejection_text", "")
	if err := h.app.Save(msg); err != nil {
		return alertError(e, "Error al cambiar la decisión")
	}
	match.Set("date", pd.Date)
	match.Set("time", pd.Time)
	match.Set("club", pd.VenueName)
	match.Set("status", league.StatusScheduled)
	if err := h.app.Save(match); err != nil {
		slog.Error("save match after acceptance", "match", match.Id, "err", err)
		return alertError(e, "Error al actualizar el partido")
	}
	h.supersedePendingAndNotify(match.Id, msg.Id)

	responderLabel := pairPlayerLabel(h.app, e.Auth.Id, match)
	proposerName := league.PlayerName(h.app, msg.GetString("author"))
	addTimelineEntry(h.app, timelineEntry{
		MatchID: match.Id, ActorID: e.Auth.Id,
		Kind:     "scheduling_response",
		Detail:   responderLabel + " aceptó la propuesta de " + proposerName + " (" + pd.Date + ", " + pd.Time + ", " + pd.VenueName + ")",
		ParentID: msg.Id,
	})

	proposerPlayers := league.PlayersForPair(h.app, proposerPairID)
	notif := league.NotifDecisionChangedToAccepted(match.Id, league.PlayerName(h.app, e.Auth.Id), pd.Date, pd.Time)
	h.notifier.NotifyPlayers(proposerPlayers, notif)
	h.notifier.EmailPlayers(proposerPlayers, notif.Title, notif.Body, "/match/"+match.Id)
	return nil
}

// ProposalChangeDecision lets a player revoke or change their proposal response.
func (h *ThreadHandler) ProposalChangeDecision(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, msg, authorTeam, err := h.validateChangeDecision(e, matchID, msgID)
	if err != nil {
		return err
	}

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	if msg.GetString("proposal_status") == "accepted" {
		if err := h.revokeAcceptance(e, match, msg, proposerPairID); err != nil {
			return err
		}
	} else {
		if err := h.changeToAccepted(e, match, msg, proposerPairID); err != nil {
			return err
		}
	}

	return redirectHX(e, "/match/"+matchID)
}

func (h *ThreadHandler) validateChangeDecision(e *core.RequestEvent, matchID, msgID string) (*core.Record, *core.Record, int, error) {
	match, err := findMatchOr404(h.app, e, matchID)
	if err != nil {
		return nil, nil, 0, err
	}
	if err := checkCompModifiable(h.app, e, match); err != nil {
		return nil, nil, 0, err
	}
	if !league.IsPreScore(match.GetString("status")) {
		return nil, nil, 0, alertError(e, "Este partido ya no acepta cambios")
	}
	myTeam, err := league.PlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return nil, nil, 0, alertError(e, "No eres participante de este partido")
	}
	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return nil, nil, 0, alertError(e, "Propuesta no encontrada")
	}
	if msg.GetString("match") != matchID {
		return nil, nil, 0, alertError(e, "Propuesta no pertenece a este partido")
	}
	authorTeam, _ := league.PlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return nil, nil, 0, alertError(e, "No puedes cambiar la decisión de tu propia propuesta")
	}
	currentStatus := msg.GetString("proposal_status")
	if currentStatus != "accepted" && currentStatus != "rejected" {
		return nil, nil, 0, alertError(e, "Solo se pueden cambiar decisiones de propuestas aceptadas o rechazadas")
	}
	return match, msg, authorTeam, nil
}
