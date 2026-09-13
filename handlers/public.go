package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// HomeAction is a unified to-do entry on the player dashboard.
type HomeAction struct {
	Kind     string // "dispute" | "confirm" | "respond" | "docs" | "organize"
	MatchID  string
	Title    string
	Detail   string
	URL      string
	SortKey  string
	Accent   string
	Recovery bool
}

var actionKindPriority = map[string]int{
	"dispute": 0, "confirm": 1, "respond": 1, "docs": 1,
	"organize": 2,
}

// PublicRenderers bundles the render functions a PublicHandler needs.
type PublicRenderers struct {
	Page      RenderFunc
	ErrorPage RenderErrorFunc
}

// PublicHandler serves player-facing pages like the dashboard and competition views.
type PublicHandler struct {
	app       core.App
	leagueSvc *league.Service
	render    PublicRenderers
}

// NewPublicHandler creates a PublicHandler with the given dependencies.
func NewPublicHandler(app core.App, leagueSvc *league.Service, render PublicRenderers) *PublicHandler {
	return &PublicHandler{app: app, leagueSvc: leagueSvc, render: render}
}

// NextMatch holds the player's next upcoming match details for the dashboard.
type NextMatch struct {
	MatchID         string
	Opponent        string
	CompetitionID   string
	CompetitionName string
	CompetitionLogo string
	RoundNumber     int
	ScheduleStatus  string // "unscheduled", "proposed", "confirmed"
	ProposedDate    string
	ProposedVenue   string
	IsPlayoff       bool
	EffectiveDate   time.Time // resolved: match date → round date → competition end
	DisplayDate     string    // formatted for the template
}

// PendingAction represents an action the player needs to take on a match.
type PendingAction struct {
	MatchID     string
	Opponent    string
	ActionType  string // "confirm_score", "respond_proposal"
	Description string
}

// DocsAction flags an active competition where the player still has
// unacknowledged mandatory documents gating their participation.
type DocsAction struct {
	CompID   string
	CompName string
}

// Home renders the player's dashboard with competitions, next match, and
// actions. Admins are redirected to /admin/competitions, the single admin
// landing page (bootstrap prompt, playoff prompts, urgent alerts, and the
// setup checklist inline on each inactive competition's card).
func (h *PublicHandler) Home(e *core.RequestEvent) error {
	if render.AdminView(e) {
		return e.Redirect(http.StatusFound, "/admin/competitions")
	}

	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}

	agg := h.aggregateHomeData(userID, playerPairIDs)

	urgentTasks, _ := league.PlayerTasks(h.app, userID, time.Now())
	actions := buildHomeActions(urgentTasks, agg.pending, agg.next, agg.docs)

	data := map[string]any{
		"PageTitle":       "Inicio",
		"Competitions":    agg.comps,
		"CompCount":       len(agg.comps),
		"Actions":         actions,
		"UpcomingMatches": agg.upcoming,
		"RecentResults":   agg.recent,
	}

	return h.render.Page(e, "home.html", data)
}

type homeAggregation struct {
	comps    []CompetitionView
	next     *NextMatch
	upcoming []NextMatch
	pending  []PendingAction
	recent   []MatchCard
	docs     []DocsAction
}

func (h *PublicHandler) aggregateHomeData(userID string, playerPairIDs map[string]struct{}) homeAggregation {
	activeComps := findRecordsLogged(h.app, "home: find active competitions", RecordQuery{Collection: "competitions", Filter: "active = true", Sort: "name"})

	var agg homeAggregation
	for _, c := range activeComps {
		if !h.playerInCompetition(c, playerPairIDs) {
			continue
		}
		parts := h.buildHomeCompetition(c, playerPairIDs, agg.next == nil)
		agg.comps = append(agg.comps, parts.Comp)
		if agg.next == nil && parts.Next != nil {
			agg.next = parts.Next
		}
		agg.upcoming = append(agg.upcoming, parts.Upcoming...)
		agg.pending = append(agg.pending, parts.Pending...)
		agg.recent = append(agg.recent, parts.Recent...)
		if len(league.UnacknowledgedMandatory(h.app, c, userID)) > 0 {
			agg.docs = append(agg.docs, DocsAction{CompID: c.Id, CompName: c.GetString("name")})
		}
	}

	agg.upcoming = filterAndSortUpcoming(agg.upcoming, time.Now(), 3)
	sort.Slice(agg.recent, func(i, j int) bool {
		return agg.recent[i].Match.GetString("date") > agg.recent[j].Match.GetString("date")
	})
	if len(agg.recent) > 5 {
		agg.recent = agg.recent[:5]
	}
	return agg
}

func (h *PublicHandler) playerInCompetition(c *core.Record, playerPairIDs map[string]struct{}) bool {
	for _, pid := range c.GetStringSlice("pairs") {
		if _, ok := playerPairIDs[pid]; ok {
			return true
		}
	}
	return false
}

func (h *PublicHandler) opponentName(m *core.Record, playerPairIDs map[string]struct{}) string {
	opponent := m.GetString("pair1")
	if _, ok := playerPairIDs[opponent]; ok {
		opponent = m.GetString("pair2")
	}
	if pair, err := h.app.FindRecordById("pairs", opponent); err == nil {
		return pair.GetString("name")
	}
	return "?"
}

type homeCompetitionParts struct {
	Comp     CompetitionView
	Next     *NextMatch
	Upcoming []NextMatch
	Pending  []PendingAction
	Recent   []MatchCard
}

func (h *PublicHandler) buildHomeCompetition(c *core.Record, playerPairIDs map[string]struct{}, needNext bool) homeCompetitionParts {
	// The competition card itself always shows (a player must be able to see
	// they're registered), but every match-derived section — next match,
	// pending actions, recent results, standing — waits for the calendar to
	// be published.
	if c.GetString("calendar_status") != "published" {
		return homeCompetitionParts{Comp: NewHomeCompetitionView(h.leagueSvc, c, 0, nil)}
	}

	pending := 0
	var nextMatch *NextMatch
	var upcoming []NextMatch
	var actions []PendingAction

	pendingMatches := findRecordsLogged(h.app, "buildHomeCompetition: find pending matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid} && (status = 'pending' || status = 'scheduled')",
		Sort: "round_number", Params: map[string]any{"cid": c.Id},
	})

	for _, m := range pendingMatches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		_, hasP1 := playerPairIDs[p1]
		_, hasP2 := playerPairIDs[p2]
		if !hasP1 && !hasP2 {
			continue
		}
		pending++

		nm := h.buildNextMatch(m, c, playerPairIDs)
		upcoming = append(upcoming, *nm)
		if needNext && nextMatch == nil {
			nextMatch = nm
		}

		if pa := h.checkPendingProposal(m, playerPairIDs); pa != nil {
			actions = append(actions, *pa)
		}
	}

	actions = append(actions, h.findUnconfirmedScores(c, playerPairIDs)...)
	results := h.findRecentResults(c, playerPairIDs)

	return homeCompetitionParts{
		Comp:     NewHomeCompetitionView(h.leagueSvc, c, pending, playerPairIDs),
		Next:     nextMatch,
		Upcoming: upcoming,
		Pending:  actions,
		Recent:   results,
	}
}

func (h *PublicHandler) buildNextMatch(m *core.Record, c *core.Record, playerPairIDs map[string]struct{}) *NextMatch {
	nm := &NextMatch{
		MatchID:         m.Id,
		Opponent:        h.opponentName(m, playerPairIDs),
		CompetitionID:   c.Id,
		CompetitionName: c.GetString("name"),
		CompetitionLogo: league.CompetitionLogoURL(c.Id, c.GetString("logo")),
		RoundNumber:     int(m.GetFloat("round_number")),
		ScheduleStatus:  "unscheduled",
		IsPlayoff:       league.IsPlayoff(c),
	}
	proposals := findRecordsLogged(h.app, "buildNextMatch: find scheduling proposal", RecordQuery{
		Collection: "match_messages",
		Filter:     "match = {:mid} && type = 'scheduling_proposal' && (proposal_status = 'pending' || proposal_status = 'accepted')",
		Sort:       "-created", Limit: 1, Params: map[string]any{"mid": m.Id},
	})
	if len(proposals) > 0 {
		applyProposalToNextMatch(nm, proposals[0])
	}
	nm.EffectiveDate = resolveMatchDate(m, c)
	if !nm.EffectiveDate.IsZero() {
		nm.DisplayDate = nm.EffectiveDate.Format(time.RFC3339)
	}
	return nm
}

func resolveMatchDate(m *core.Record, c *core.Record) time.Time {
	if d := m.GetDateTime("date").Time(); !d.IsZero() {
		return d
	}
	roundNum := int(m.GetFloat("round_number"))
	if t, ok := league.RoundArrangeDate(c, roundNum); ok && !t.IsZero() {
		return t
	}
	if end := c.GetDateTime("end_date").Time(); !end.IsZero() {
		return end
	}
	return time.Time{}
}

func applyProposalToNextMatch(nm *NextMatch, prop *core.Record) {
	pd := ParseProposalData(prop.GetString("proposal_data"))
	if prop.GetString("proposal_status") == "accepted" {
		nm.ScheduleStatus = "confirmed"
	} else {
		nm.ScheduleStatus = "proposed"
	}
	if pd == nil {
		return
	}
	nm.ProposedDate = pd.Date + " " + pd.Time
	if pd.VenueName != "" {
		nm.ProposedVenue = pd.VenueName
	} else if pd.VenueText != "" {
		nm.ProposedVenue = pd.VenueText
	}
}

func (h *PublicHandler) checkPendingProposal(m *core.Record, playerPairIDs map[string]struct{}) *PendingAction {
	proposals := findRecordsLogged(h.app, "checkPendingProposal: find scheduling proposals", RecordQuery{
		Collection: "match_messages", Filter: "match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
		Sort: "-created", Params: map[string]any{"mid": m.Id},
	})
	if len(proposals) == 0 {
		return nil
	}
	playerTeam := 1
	if _, ok := playerPairIDs[m.GetString("pair2")]; ok {
		playerTeam = 2
	}
	for _, prop := range proposals {
		proposerTeam, _ := league.PlayerTeam(h.app, prop.GetString("author"), m)
		if proposerTeam != playerTeam {
			return &PendingAction{
				MatchID:     m.Id,
				Opponent:    h.opponentName(m, playerPairIDs),
				ActionType:  "respond_proposal",
				Description: "Propuesta de horario pendiente",
			}
		}
	}
	return nil
}

func (h *PublicHandler) findUnconfirmedScores(c *core.Record, playerPairIDs map[string]struct{}) []PendingAction {
	actions := h.findLegacyConfirmed(c, playerPairIDs)
	actions = append(actions, h.findPendingProposals(c, playerPairIDs)...)
	return actions
}

func (h *PublicHandler) findLegacyConfirmed(c *core.Record, playerPairIDs map[string]struct{}) []PendingAction {
	var actions []PendingAction
	confirmed := findRecordsLogged(h.app, "findLegacyConfirmed: find confirmed matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid} && status = 'confirmed'",
		Sort: "-created", Params: map[string]any{"cid": c.Id},
	})
	for _, m := range confirmed {
		if !isRivalAction(h.app, m, m.GetString("submitted_by"), playerPairIDs) {
			continue
		}
		actions = append(actions, PendingAction{
			MatchID:     m.Id,
			Opponent:    h.opponentName(m, playerPairIDs),
			ActionType:  "confirm_score",
			Description: "Responder resultado: " + m.GetString("scores"),
		})
	}
	return actions
}

func (h *PublicHandler) findPendingProposals(c *core.Record, playerPairIDs map[string]struct{}) []PendingAction {
	var actions []PendingAction
	proposals := findRecordsLogged(h.app, "findPendingProposals: find result proposals", RecordQuery{
		Collection: "match_messages", Filter: "type = 'result_submission' && proposal_status = 'pending'", Sort: "-created",
	})
	for _, p := range proposals {
		m, err := h.app.FindRecordById("matches", p.GetString("match"))
		if err != nil || m.GetString("competition") != c.Id {
			continue
		}
		if !isRivalAction(h.app, m, p.GetString("author"), playerPairIDs) {
			continue
		}
		scores := m.GetString("scores")
		if scores == "" {
			scores = "pendiente"
		}
		actions = append(actions, PendingAction{
			MatchID:     m.Id,
			Opponent:    h.opponentName(m, playerPairIDs),
			ActionType:  "respond_result",
			Description: "Responder resultado: " + scores,
		})
	}
	return actions
}

func isRivalAction(app core.App, m *core.Record, authorID string, playerPairIDs map[string]struct{}) bool {
	_, hasP1 := playerPairIDs[m.GetString("pair1")]
	_, hasP2 := playerPairIDs[m.GetString("pair2")]
	if !hasP1 && !hasP2 {
		return false
	}
	authorTeam, _ := league.PlayerTeam(app, authorID, m)
	playerTeam := 1
	if hasP2 {
		playerTeam = 2
	}
	return authorTeam != playerTeam
}

func (h *PublicHandler) findRecentResults(c *core.Record, playerPairIDs map[string]struct{}) []MatchCard {
	finals := findRecordsLogged(h.app, "findRecentResults: find final matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid} && status = 'final'",
		Sort: "-date,-created", Limit: 20, Params: map[string]any{"cid": c.Id},
	})
	pairNames := collectPairNames(h.app, finals)
	// No IsMyMatch accent here: every row is already filtered to the
	// player's own pairs below, so the left border would be noise on
	// every row rather than a distinguishing signal.
	noAccent := map[string]struct{}{}
	var results []MatchCard
	for _, m := range finals {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		_, hasP1 := playerPairIDs[p1]
		_, hasP2 := playerPairIDs[p2]
		if !hasP1 && !hasP2 {
			continue
		}
		mc := NewMatchRow(m, pairNames, noAccent)
		mc.CompetitionName = c.GetString("name")
		mc.CompetitionLogo = league.CompetitionLogoURL(c.Id, c.GetString("logo"))
		if hasP1 {
			mc.Opponent = pairNames[p2]
			mc.Won = m.GetString("winner") == p1
		} else {
			mc.Opponent = pairNames[p1]
			mc.Won = m.GetString("winner") == p2
		}
		results = append(results, mc)
		if len(results) >= 5 {
			break
		}
	}
	return results
}

func filterAndSortUpcoming(upcoming []NextMatch, now time.Time, maxCount int) []NextMatch {
	twoWeeks := now.Add(14 * 24 * time.Hour)
	var dated, undated []NextMatch
	for _, u := range upcoming {
		if u.EffectiveDate.IsZero() {
			u.DisplayDate = "Fecha por confirmar"
			undated = append(undated, u)
		} else if !u.EffectiveDate.After(twoWeeks) {
			dated = append(dated, u)
		}
	}
	sort.Slice(dated, func(i, j int) bool {
		return dated[i].EffectiveDate.Before(dated[j].EffectiveDate)
	})
	result := append(dated, undated...)
	if len(result) > maxCount {
		result = result[:maxCount]
	}
	return result
}

func buildHomeActions(tasks []league.PlayerTask, pending []PendingAction, next *NextMatch, docs []DocsAction) []HomeAction {
	seen := map[string]HomeAction{}
	for _, t := range tasks {
		mergeAction(seen, taskToAction(t))
	}
	for _, p := range pending {
		mergeAction(seen, pendingToAction(p))
	}
	if next != nil && next.ScheduleStatus == "unscheduled" && !next.IsPlayoff {
		if _, exists := seen[next.MatchID]; !exists {
			mergeAction(seen, nextMatchAction(next))
		}
	}
	actions := make([]HomeAction, 0, len(seen)+len(docs))
	for _, a := range seen {
		actions = append(actions, a)
	}
	for _, d := range docs {
		actions = append(actions, docsToAction(d))
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].SortKey < actions[j].SortKey
	})
	return actions
}

func taskToAction(t league.PlayerTask) HomeAction {
	a := HomeAction{MatchID: t.MatchID, URL: "/match/" + t.MatchID, Recovery: t.Recovery}
	switch t.Kind {
	case league.TaskDispute:
		a.Kind = "dispute"
		a.Title = "Disputa abierta"
		a.Detail = fmt.Sprintf("vs %s · %s", t.Opponent, t.CompetitionName)
		a.Accent = "error"
		a.SortKey = fmt.Sprintf("0-%d", t.RoundNumber)
	case league.TaskOrganize:
		a.Kind = "organize"
		a.Title = t.Description
		a.Detail = fmt.Sprintf("vs %s · %s", t.Opponent, t.CompetitionName)
		a.Accent = warningAccent(t.Warning)
		a.SortKey = fmt.Sprintf("2-%d", t.Warning)
	case league.TaskPlay:
		return HomeAction{}
	}
	return a
}

func pendingToAction(p PendingAction) HomeAction {
	a := HomeAction{MatchID: p.MatchID, URL: "/match/" + p.MatchID, Accent: "warning"}
	switch p.ActionType {
	case "confirm_score":
		a.Kind = "confirm"
		a.Title = "Confirmar resultado"
		a.Detail = fmt.Sprintf("vs %s · %s", p.Opponent, p.Description)
		a.SortKey = "1-confirm"
	case "respond_result":
		a.Kind = "confirm"
		a.Title = "Responder resultado"
		a.Detail = fmt.Sprintf("vs %s · %s", p.Opponent, p.Description)
		a.SortKey = "1-respond-result"
	case "respond_proposal":
		a.Kind = "respond"
		a.Title = "Responder propuesta"
		a.Detail = fmt.Sprintf("vs %s · %s", p.Opponent, p.Description)
		a.SortKey = "1-respond-proposal"
	}
	return a
}

func docsToAction(d DocsAction) HomeAction {
	return HomeAction{
		Kind:    "docs",
		Title:   "Lee los documentos",
		Detail:  d.CompName,
		URL:     "/competition/" + d.CompID,
		Accent:  "warning",
		SortKey: "1z-docs-" + d.CompID,
	}
}

func nextMatchAction(next *NextMatch) HomeAction {
	a := HomeAction{
		MatchID: next.MatchID,
		URL:     "/match/" + next.MatchID,
		Kind:    "organize",
		Title:   "Propón una fecha",
		Detail:  fmt.Sprintf("vs %s · J%d · %s", next.Opponent, next.RoundNumber, next.CompetitionName),
		Accent:  "info",
		SortKey: "2-0",
	}
	return a
}

func mergeAction(seen map[string]HomeAction, a HomeAction) {
	if a.MatchID == "" || a.Kind == "" {
		return
	}
	existing, exists := seen[a.MatchID]
	if !exists || actionKindPriority[a.Kind] < actionKindPriority[existing.Kind] {
		seen[a.MatchID] = a
	}
}

func warningAccent(w league.Warning) string {
	switch {
	case w >= league.WarnOverdue:
		return "error"
	case w >= league.WarnUrgent:
		return "warning"
	default:
		return "info"
	}
}
