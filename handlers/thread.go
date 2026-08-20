package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

type ThreadHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewThreadHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *ThreadHandler {
	return &ThreadHandler{app: app, renderPage: renderPage}
}

type ThreadMessage struct {
	Record          *core.Record
	AuthorName      string
	AuthorTeam      int
	IsMyTeam        bool
	Type            string
	Content         string
	ProposalData    *ProposalData
	Status          string
	RejectionReason string
	RejectionText   string
	CanRespond      bool
	CreatedAt       string
}

type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
}

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
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &pd); err != nil {
			return nil
		}
	default:
		return nil
	}
	return &pd
}

func (h *ThreadHandler) Thread(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return e.HTML(http.StatusOK, `<div class="text-center py-6 opacity-60">Parejas pendientes de asignacion</div>`)
	}

	isAdmin := e.Auth.GetString("role") == "admin"
	myTeam, teamErr := getPlayerTeam(h.app, e.Auth.Id, match)
	if teamErr != nil && !isAdmin {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No tienes acceso a este hilo</div>`)
	}

	messages, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "", 0, 0,
		map[string]any{"mid": matchID})

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].GetDateTime("created").Time().Before(
			messages[j].GetDateTime("created").Time())
	})

	venues, _ := h.app.FindRecordsByFilter("venues",
		"id != ''", "name", 0, 0, nil)

	pair1Players := getPlayersForPair(h.app, pair1ID)
	pair2Players := getPlayersForPair(h.app, pair2ID)
	nameCache := make(map[string]string)

	authorTeamOf := func(uid string) int {
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

	cachedName := func(uid string) string {
		if n, ok := nameCache[uid]; ok {
			return n
		}
		n := resolvePlayerName(h.app, uid)
		nameCache[uid] = n
		return n
	}

	var threadMessages []ThreadMessage
	for _, msg := range messages {
		authorID := msg.GetString("author")
		authorTeam := authorTeamOf(authorID)

		msgType := msg.GetString("type")
		var pd *ProposalData
		if msgType == "scheduling_proposal" {
			pd = ParseProposalData(msg.Get("proposal_data"))
		}

		status := msg.GetString("proposal_status")
		canRespond := msgType == "scheduling_proposal" &&
			status == "pending" &&
			match.GetString("status") == "pending" &&
			!isAdmin &&
			myTeam != 0 &&
			authorTeam != myTeam

		threadMessages = append(threadMessages, ThreadMessage{
			Record:          msg,
			AuthorName:      cachedName(authorID),
			AuthorTeam:      authorTeam,
			IsMyTeam:        myTeam != 0 && authorTeam == myTeam,
			Type:            msgType,
			Content:         msg.GetString("content"),
			ProposalData:    pd,
			Status:          status,
			RejectionReason: msg.GetString("rejection_reason"),
			RejectionText:   msg.GetString("rejection_text"),
			CanRespond:      canRespond,
			CreatedAt:       msg.GetDateTime("created").Time().Format("02/01 15:04"),
		})
	}

	canPost := myTeam != 0 && !isAdmin
	canPropose := canPost && match.GetString("status") == "pending"

	return h.renderPage(e, "thread.html", map[string]any{
		"MatchID":    matchID,
		"Messages":   threadMessages,
		"Venues":     venues,
		"CanPost":    canPost,
		"CanPropose": canPropose,
		"IsAdmin":    isAdmin,
	})
}

func (h *ThreadHandler) PostMessage(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	myTeam, err := getPlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	content := e.Request.FormValue("content")
	if content == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">El mensaje no puede estar vacio</div>`)
	}

	msgType := e.Request.FormValue("type")
	if msgType != "chat" && msgType != "score_discussion" {
		msgType = "chat"
	}

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", msgType)
	record.Set("content", content)

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al enviar mensaje</div>`)
	}

	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := getPlayersForPair(h.app, rivalPairID)
	authorName := resolvePlayerName(h.app, e.Auth.Id)
	notifyPlayers(h.app, rivalPlayers, "general",
		"Nuevo mensaje",
		fmt.Sprintf("%s escribio: %s", authorName, truncate(content, 60)),
		matchID)

	e.Response.Header().Set("HX-Redirect", "/match/"+matchID)
	return e.NoContent(http.StatusNoContent)
}

func (h *ThreadHandler) PostProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Solo se pueden proponer fechas para partidos pendientes</div>`)
	}

	pair1ID := match.GetString("pair1")
	pair2ID := match.GetString("pair2")
	if pair1ID == "" || pair2ID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido aun no tiene parejas asignadas</div>`)
	}

	myTeam, err := getPlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	date := e.Request.FormValue("date")
	time := e.Request.FormValue("time")
	venueID := e.Request.FormValue("venue_id")
	venueText := e.Request.FormValue("venue_text")

	if date == "" || time == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Fecha y hora son obligatorias</div>`)
	}

	venueName := venueText
	if venueID != "" && venueID != "otro" {
		venue, err := h.app.FindRecordById("venues", venueID)
		if err != nil {
			venueID = ""
			venueName = venueText
		} else {
			venueName = venue.GetString("name")
		}
	} else {
		venueID = ""
	}

	pd := ProposalData{
		Date:      date,
		Time:      time,
		VenueID:   venueID,
		VenueName: venueName,
		VenueText: venueText,
	}
	pdJSON, _ := json.Marshal(pd)

	col, err := h.app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", e.Auth.Id)
	record.Set("type", "scheduling_proposal")
	record.Set("proposal_data", string(pdJSON))
	record.Set("proposal_status", "pending")

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al crear propuesta</div>`)
	}

	rivalPairID := match.GetString("pair1")
	if myTeam == 1 {
		rivalPairID = match.GetString("pair2")
	}
	rivalPlayers := getPlayersForPair(h.app, rivalPairID)
	authorName := resolvePlayerName(h.app, e.Auth.Id)
	notifyPlayers(h.app, rivalPlayers, "scheduling",
		"Propuesta de fecha",
		fmt.Sprintf("%s propone jugar el %s a las %s en %s", authorName, date, time, venueName),
		matchID)

	e.Response.Header().Set("HX-Redirect", "/match/"+matchID)
	return e.NoContent(http.StatusNoContent)
}

func (h *ThreadHandler) RespondProposal(e *core.RequestEvent) error {
	matchID := e.Request.PathValue("id")
	msgID := e.Request.PathValue("msgId")

	match, err := h.app.FindRecordById("matches", matchID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Partido no encontrado</div>`)
	}

	if match.GetString("status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Este partido ya no acepta propuestas</div>`)
	}

	myTeam, err := getPlayerTeam(h.app, e.Auth.Id, match)
	if err != nil || myTeam == 0 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No eres participante de este partido</div>`)
	}

	msg, err := h.app.FindRecordById("match_messages", msgID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Propuesta no encontrada</div>`)
	}

	if msg.GetString("match") != matchID {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Propuesta no pertenece a este partido</div>`)
	}

	if msg.GetString("proposal_status") != "pending" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Esta propuesta ya fue respondida</div>`)
	}

	authorTeam, _ := getPlayerTeam(h.app, msg.GetString("author"), match)
	if authorTeam == myTeam {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">No puedes responder a tu propia propuesta</div>`)
	}

	proposerPairID := match.GetString("pair1")
	if authorTeam == 2 {
		proposerPairID = match.GetString("pair2")
	}

	action := e.Request.FormValue("action")

	if action == "accept" {
		existing, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && proposal_status = 'accepted'",
			"", 0, 1,
			map[string]any{"mid": matchID})
		if len(existing) > 0 {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Ya hay una propuesta aceptada para este partido</div>`)
		}

		pd := ParseProposalData(msg.Get("proposal_data"))
		if pd == nil {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al leer los datos de la propuesta</div>`)
		}

		match.Set("date", pd.Date)
		match.Set("time", pd.Time)
		match.Set("club", pd.VenueName)
		if err := h.app.Save(match); err != nil {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al actualizar el partido</div>`)
		}

		msg.Set("proposal_status", "accepted")
		if err := h.app.Save(msg); err != nil {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al marcar la propuesta como aceptada</div>`)
		}

		otherPending, _ := h.app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending' && id != {:msgid}",
			"", 0, 0,
			map[string]any{"mid": matchID, "msgid": msgID})
		for _, other := range otherPending {
			other.Set("proposal_status", "superseded")
			h.app.Save(other)
		}

		proposerPlayers := getPlayersForPair(h.app, proposerPairID)
		responderName := resolvePlayerName(h.app, e.Auth.Id)
		notifyPlayers(h.app, proposerPlayers, "scheduling",
			"Propuesta aceptada",
			fmt.Sprintf("%s acepto tu propuesta para el %s a las %s", responderName, pd.Date, pd.Time),
			matchID)
	} else if action == "reject" {
		reason := e.Request.FormValue("rejection_reason")
		text := e.Request.FormValue("rejection_text")

		msg.Set("proposal_status", "rejected")
		msg.Set("rejection_reason", reason)
		msg.Set("rejection_text", text)
		if err := h.app.Save(msg); err != nil {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al rechazar la propuesta</div>`)
		}

		proposerPlayers := getPlayersForPair(h.app, proposerPairID)
		responderName := resolvePlayerName(h.app, e.Auth.Id)
		notifyPlayers(h.app, proposerPlayers, "scheduling",
			"Propuesta rechazada",
			fmt.Sprintf("%s rechazo tu propuesta: %s", responderName, reason),
			matchID)
	} else {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Accion no valida</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/match/"+matchID)
	return e.NoContent(http.StatusNoContent)
}
