package handlers

import (
	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// ThreadData is the view model produced by buildThreadData, consumed by
// thread.html and thread-messages.html.
type ThreadData struct {
	Timeline       []TimelineEntryVM // read-only history lines, oldest→newest
	SchedProposals []SchedProposalVM // non-rejected scheduling proposals, newest first
	ResultPanel    ResultPanelVM     // live result proposal(s) or the final result
}

// TimelineEntryVM is one read-only history line in the timeline.
type TimelineEntryVM struct {
	Kind        string // "proposal" | "response" | "event" | "chat"
	AuthorName  string
	IsMyTeam    bool
	Content     string
	Note        string // rejection reason, shown below the dateBox/resultBox
	CreatedAt   string // render.FmtShortTime — DD/MM HH:MM
	Score       string // result_submission only
	Pair1Name   string // result_submission only, for the winner-name calculation
	Pair2Name   string // result_submission only, for the winner-name calculation
	Date        string // scheduling_proposal only (stored "2006-01-02")
	Time        string // scheduling_proposal only
	Place       string // scheduling_proposal venue name
	Status      string // proposal_status; scheduling_proposal/result_submission only
	StatusLabel string // Spanish label for Status
	StatusClass string // DaisyUI badge class for Status
}

// SchedProposalVM is one non-rejected scheduling proposal in the details panel.
type SchedProposalVM struct {
	RecordID          string
	MatchID           string
	AuthorLabel       string
	Data              *ProposalData
	Status            string // "pending" | "accepted" | "superseded"
	IsAccepted        bool
	CanRespond        bool
	CanChangeDecision bool
	CreatedAt         string
}

// ResultPanelVM is the result box: the final result once quorum is reached,
// otherwise the live result proposal(s) awaiting a response.
type ResultPanelVM struct {
	HasFinal   bool
	FinalScore string
	WinnerName string
	Live       []ResultProposalVM
}

// ResultProposalVM is one live result proposal in the result box.
type ResultProposalVM struct {
	RecordID     string
	MatchID      string
	PairLabel    string
	Score        string
	AwaitingMe   bool
	CanRespond   bool
	ScoreCounter ScoreInputVM
}

type threadBuildCtx struct {
	app            core.App
	match          *core.Record
	matchID        string
	matchStatus    string
	myTeam         int
	compModifiable bool
	pairNames      map[string]string
	pair1Players   []string
	pair2Players   []string
}

func (h *ThreadHandler) buildThreadData(match *core.Record, matchID string, myTeam int, compModifiable bool) ThreadData {
	bc := threadBuildCtx{
		app:            h.app,
		match:          match,
		matchID:        matchID,
		matchStatus:    match.GetString("status"),
		myTeam:         myTeam,
		compModifiable: compModifiable,
		pairNames:      league.PairNames(h.app, []string{match.GetString("pair1"), match.GetString("pair2")}),
		pair1Players:   league.PlayersForPair(h.app, match.GetString("pair1")),
		pair2Players:   league.PlayersForPair(h.app, match.GetString("pair2")),
	}
	messages, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "created", 0, 0,
		map[string]any{"mid": matchID})
	nameCache := make(map[string]string)
	var td ThreadData
	if bc.matchStatus == league.StatusFinal {
		td.ResultPanel.HasFinal = true
		td.ResultPanel.FinalScore = match.GetString("scores")
		if name, ok := bc.pairNames[match.GetString("winner")]; ok {
			td.ResultPanel.WinnerName = name
		}
	}
	for _, msg := range messages {
		authorID := msg.GetString("author")
		if _, ok := nameCache[authorID]; !ok {
			nameCache[authorID] = league.PlayerName(h.app, authorID)
		}
		bc.processMessage(msg, authorID, nameCache[authorID], &td)
	}
	for i, j := 0, len(td.SchedProposals)-1; i < j; i, j = i+1, j-1 {
		td.SchedProposals[i], td.SchedProposals[j] = td.SchedProposals[j], td.SchedProposals[i]
	}
	return td
}

// msgCtx bundles the per-message values derived once in processMessage and
// shared by the timeline/panel builders below.
type msgCtx struct {
	msg        *core.Record
	msgType    string
	authorName string
	authorTeam int
	created    string
}

func (bc *threadBuildCtx) processMessage(msg *core.Record, authorID, cachedName string, td *ThreadData) {
	mc := msgCtx{
		msg:        msg,
		msgType:    msg.GetString("type"),
		authorName: cachedName,
		authorTeam: playerTeamOf(authorID, bc.pair1Players, bc.pair2Players),
		created:    render.FmtShortTime(msg.GetDateTime("created").Time()),
	}
	if isProposalOrResponse(mc.msgType) {
		mc.authorName = pairPlayerLabel(bc.app, authorID, bc.match)
	}
	td.Timeline = append(td.Timeline, bc.timelineEntry(mc))
	bc.appendToPanel(mc, td)
}

// timelineEntry builds the frozen-snapshot timeline line for one message.
// The action text and status shown are frozen at insertion time — they
// reflect what happened at THIS event (from its own message type and, for a
// response, its own recorded Action), not proposal_status (which is the
// proposal's current, possibly later-superseded, state).
func (bc *threadBuildCtx) timelineEntry(mc msgCtx) TimelineEntryVM {
	pd := ParseProposalData(mc.msg.GetString("proposal_data"))
	entry := TimelineEntryVM{
		Kind:       timelineKind(mc.msgType),
		AuthorName: mc.authorName,
		IsMyTeam:   bc.myTeam != 0 && mc.authorTeam == bc.myTeam,
		CreatedAt:  mc.created,
	}
	fillTimelineEntryData(&entry, pd, mc.msgType)
	if mc.msgType == "result_submission" || mc.msgType == "result_response" {
		entry.Pair1Name = bc.pairNames[bc.match.GetString("pair1")]
		entry.Pair2Name = bc.pairNames[bc.match.GetString("pair2")]
	}
	action := ""
	if pd != nil {
		action = pd.Action
	}
	entry.Content, entry.StatusLabel, entry.StatusClass = timelineEntryText(mc.msgType, action, mc.msg.GetString("content"))
	entry.Note = bc.timelineNote(mc, action)
	return entry
}

func (bc *threadBuildCtx) timelineNote(mc msgCtx, action string) string {
	if mc.msgType == "result_response" && action == "reject" && bc.matchStatus == league.StatusDisputed {
		return bc.match.GetString("dispute_notes")
	}
	if mc.msgType == "scheduling_response" && action == "reject" {
		if text := mc.msg.GetString("rejection_text"); text != "" {
			return text
		}
		return mc.msg.GetString("rejection_reason")
	}
	return ""
}

// appendToPanel adds pending scheduling/result proposals to the details
// panel. Only PENDING scheduling proposals belong there — an accepted date
// is a settled fact shown once in the match header; superseded/rejected are
// history (timeline only). No repeats.
func (bc *threadBuildCtx) appendToPanel(mc msgCtx, td *ThreadData) {
	status := mc.msg.GetString("proposal_status")
	sameTeam := mc.authorTeam == bc.myTeam || bc.myTeam == 0
	if mc.msgType == "scheduling_proposal" && status == "pending" {
		td.SchedProposals = append(td.SchedProposals, bc.schedProposal(mc.msg, mc.authorName, mc.created, sameTeam))
	}
	if mc.msgType == "result_submission" && status == "pending" && !td.ResultPanel.HasFinal {
		td.ResultPanel.Live = append(td.ResultPanel.Live, bc.resultProposal(mc.msg, mc.authorName, mc.authorTeam, sameTeam))
	}
}

func (bc *threadBuildCtx) schedProposal(msg *core.Record, authorName, created string, sameTeam bool) SchedProposalVM {
	status := msg.GetString("proposal_status")
	canRespond, canChange := proposalActions("scheduling_proposal", bc.matchStatus, sameTeam, status)
	return SchedProposalVM{
		RecordID:          msg.Id,
		MatchID:           bc.matchID,
		AuthorLabel:       authorName,
		Data:              ParseProposalData(msg.GetString("proposal_data")),
		Status:            status,
		IsAccepted:        status == "accepted",
		CanRespond:        canRespond && bc.compModifiable,
		CanChangeDecision: canChange && bc.compModifiable,
		CreatedAt:         created,
	}
}

func (bc *threadBuildCtx) resultProposal(msg *core.Record, authorName string, authorTeam int, sameTeam bool) ResultProposalVM {
	canRespond, _ := proposalActions("result_submission", bc.matchStatus, sameTeam, msg.GetString("proposal_status"))
	canRespond = canRespond && bc.compModifiable
	rp := ResultProposalVM{
		RecordID:   msg.Id,
		MatchID:    bc.matchID,
		PairLabel:  authorName,
		Score:      msg.GetString("content"),
		AwaitingMe: bc.myTeam != 0 && authorTeam != bc.myTeam,
		CanRespond: canRespond,
	}
	if canRespond {
		rp.ScoreCounter = ScoreInputVM{
			FieldName: "counter_scores",
			IDSuffix:  bc.matchID + "-counter-" + msg.Id,
			Pair1Name: bc.pairNames[bc.match.GetString("pair1")],
			Pair2Name: bc.pairNames[bc.match.GetString("pair2")],
		}
	}
	return rp
}

func timelineKind(msgType string) string {
	switch msgType {
	case "scheduling_proposal", "result_submission":
		return "proposal"
	case "scheduling_response", "result_response":
		return "response"
	case "result_event", "admin_action":
		return "event"
	default:
		return "chat"
	}
}

// fillTimelineEntryData sets the score/date fields the timeline template
// renders through resultBox/dateBox, based on the message type. Response
// entries (accept/reject) carry the same proposal data as their parent
// proposal, so they render the identical component with a different badge.
func fillTimelineEntryData(entry *TimelineEntryVM, pd *ProposalData, msgType string) {
	if pd == nil {
		return
	}
	switch msgType {
	case "result_submission", "result_response":
		entry.Score = pd.Scores
	case "scheduling_proposal", "scheduling_response":
		entry.Date = pd.Date
		entry.Time = pd.Time
		entry.Place = pd.VenueName
	}
}

// timelineEntryText returns the action verb phrase, status label, and status
// badge class for a timeline entry. The status is frozen at insertion time —
// derived from the message TYPE and, for a response, its own recorded Action
// ("accept"/"reject") — never from the proposal's current, possibly
// later-superseded proposal_status.
//
// Scheduling proposals and result submissions carry their date/score in
// Date/Time/Place/Score instead of inline text — the template renders those
// through dateBox/resultBox, so the verb here stays value-free.
func timelineEntryText(msgType, action, content string) (verb, statusLabel, statusClass string) {
	switch msgType {
	case "scheduling_proposal":
		return "propuso fecha y lugar", "Propuesta", "badge-warning"
	case "result_submission":
		return "propuso resultado", "Propuesta", "badge-warning"
	case "scheduling_response":
		if action == "accept" {
			return "confirmó fecha y lugar", "Confirmada", "badge-success"
		}
		return "rechazó fecha y lugar", "Rechazada", "badge-error"
	case "result_response":
		if action == "accept" {
			return "confirmó resultado", "Confirmado", "badge-success"
		}
		return "rechazó resultado", "Rechazado", "badge-error"
	default: // result_event, admin_action, chat
		return content, "", ""
	}
}

func isProposalOrResponse(msgType string) bool {
	return msgType == "scheduling_proposal" || msgType == "result_submission" ||
		msgType == "scheduling_response" || msgType == "result_response"
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
