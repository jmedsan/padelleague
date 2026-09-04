package handlers

import (
	"fmt"
	"net/http"
	"slices"
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

// PublicHandler serves player-facing pages like the dashboard and competition views.
type PublicHandler struct {
	app             core.App
	leagueSvc       *league.Service
	renderPage      RenderFunc
	renderErrorPage RenderErrorFunc
}

// NewPublicHandler creates a PublicHandler with the given dependencies.
func NewPublicHandler(app core.App, leagueSvc *league.Service, renderPage RenderFunc, renderErrorPage RenderErrorFunc) *PublicHandler {
	return &PublicHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage, renderErrorPage: renderErrorPage}
}

// OnboardStep is one item in the player onboarding checklist.
type OnboardStep struct {
	Label string
	URL   string
	Done  bool
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

	if slices.Contains(e.Auth.GetStringSlice("roles"), "player") {
		if steps := h.onboardingSteps(e.Auth); len(steps) > 0 {
			data["OnboardSteps"] = steps
		}
	}

	return h.renderPage(e, "home.html", data)
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

// onboardingSteps returns the player onboarding checklist, or nil when every
// actionable step is done (so the template hides the card).
func (h *PublicHandler) onboardingSteps(user *core.Record) []OnboardStep {
	profileDone := user.GetString("display_name") != ""

	if profileDone {
		return nil
	}

	return []OnboardStep{
		{Label: "Completa tu perfil", URL: "/profile/complete", Done: profileDone},
	}
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
		Filter:     "match = {:mid} && type = 'scheduling_proposal' && proposal_status != 'rejected' && proposal_status != 'superseded'",
		Sort:       "-created", Limit: 1, Params: map[string]any{"mid": m.Id},
	})
	if len(proposals) > 0 {
		applyProposalToNextMatch(nm, proposals[0])
	}
	nm.EffectiveDate = resolveMatchDate(m, c)
	if !nm.EffectiveDate.IsZero() {
		nm.DisplayDate = nm.EffectiveDate.Format("02/01/2006")
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
	proposals := findRecordsLogged(h.app, "checkPendingProposal: find scheduling proposal", RecordQuery{
		Collection: "match_messages", Filter: "match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
		Sort: "-created", Limit: 1, Params: map[string]any{"mid": m.Id},
	})
	if len(proposals) == 0 {
		return nil
	}
	prop := proposals[0]
	proposerTeam, _ := league.PlayerTeam(h.app, prop.GetString("author"), m)
	playerTeam := 1
	if _, ok := playerPairIDs[m.GetString("pair2")]; ok {
		playerTeam = 2
	}
	if proposerTeam == playerTeam {
		return nil
	}
	return &PendingAction{
		MatchID:     m.Id,
		Opponent:    h.opponentName(m, playerPairIDs),
		ActionType:  "respond_proposal",
		Description: "Propuesta de horario pendiente",
	}
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
		Detail:  fmt.Sprintf("vs %s · %s", next.Opponent, next.CompetitionName),
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

// RoundView groups matches by round number for the competition page.
type RoundView struct {
	RoundNumber int
	Matches     []MatchCard
}

// BracketRound holds one round of a single-elimination bracket for display.
type BracketRound struct {
	Name    string
	Matches []MatchCard
}

func buildBracket(rounds []RoundView, maxRound int) []BracketRound {
	var bracket []BracketRound
	for _, r := range rounds {
		bracket = append(bracket, BracketRound{
			Name:    bracketRoundName(r.RoundNumber, maxRound),
			Matches: r.Matches,
		})
	}
	return bracket
}

func bracketRoundName(round, maxRound int) string {
	remaining := maxRound - round
	switch remaining {
	case 0:
		return "Final"
	case 1:
		return "Semifinal"
	case 2:
		return "Cuartos"
	case 3:
		return "Octavos"
	default:
		return fmt.Sprintf("Ronda %d", round)
	}
}

// Competition renders the public competition page with standings and fixtures.
func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Competición no encontrada")
	}

	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}

	if gated, err := h.docsGate(e, comp, userID, playerPairIDs); gated || err != nil {
		return err
	}

	matches := findRecordsLogged(h.app, "Competition: find matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid}",
		Sort: "round_number,created", Params: map[string]any{"cid": id},
	})

	pairNames := collectPairNames(h.app, matches)

	showAll := e.Request.URL.Query().Get("all") == "1"
	isPlayoff := league.IsPlayoff(comp)
	if isPlayoff {
		showAll = true
	}
	rounds := buildRounds(matches, pairNames, playerPairIDs, showAll)
	for i := range rounds {
		enrichWithPendingResults(h.app, rounds[i].Matches)
	}
	autoExpandRound := firstIncompleteRound(rounds)

	data := h.buildCompetitionData(comp, rounds, autoExpandRound)
	data["PageTitle"] = comp.GetString("name")
	data["PlayerPairIDs"] = playerPairIDs
	data["ShowAll"] = showAll
	data["Mode"] = PlayerSummary
	data["OGImage"] = league.CompetitionLogoURL(comp.Id, comp.GetString("logo"))
	data["FooterCompetitionID"] = comp.Id
	h.addCompetitionDocViews(data, comp, userID)
	return h.renderPage(e, "competition.html", data)
}

// docsGate renders the mandatory-documents gate page and reports gated=true
// when the player has unacknowledged mandatory documents; the caller must
// stop processing the request in that case regardless of err.
func (h *PublicHandler) docsGate(e *core.RequestEvent, comp *core.Record, userID string, playerPairIDs map[string]struct{}) (gated bool, err error) {
	if !league.IsParticipant(comp, playerPairIDs) {
		return false, nil
	}
	pending := league.UnacknowledgedMandatory(h.app, comp, userID)
	if len(pending) == 0 {
		return false, nil
	}
	allDocs := league.AttachedDocuments(h.app, comp)
	mandatoryIDs := make([]string, len(pending))
	for i, d := range pending {
		mandatoryIDs[i] = d.Id
	}
	docViews := make([]DocumentView, len(allDocs))
	for i, d := range allDocs {
		docViews[i] = NewDocumentView(d, PlayerSummary)
	}
	err = h.renderPage(e, "competition-docs-gate.html", map[string]any{
		"PageTitle":           "Documentos",
		"Competition":         comp,
		"DocumentViews":       docViews,
		"MandatoryIDs":        mandatoryIDs,
		"Mode":                PlayerRow,
		"FooterCompetitionID": comp.Id,
	})
	return true, err
}

// addCompetitionDocViews sets DocumentView entries on data when the
// competition has attached documents, marking which ones userID has
// acknowledged.
func (h *PublicHandler) addCompetitionDocViews(data map[string]any, comp *core.Record, userID string) {
	docs := league.AttachedDocuments(h.app, comp)
	if len(docs) == 0 {
		return
	}
	ackedSlice := league.AckedDocIDs(h.app, comp.Id, userID)
	ackedSet := make(map[string]struct{}, len(ackedSlice))
	for _, id := range ackedSlice {
		ackedSet[id] = struct{}{}
	}
	docViews := make([]DocumentView, len(docs))
	for i, d := range docs {
		docViews[i] = NewDocumentViewWithAck(d, PlayerSummary, ackedSet)
	}
	data["DocumentViews"] = docViews
}

// AcceptDocs records that the player has read the competition's mandatory documents.
func (h *PublicHandler) AcceptDocs(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return h.renderErrorPage(e, http.StatusNotFound, "Competición no encontrada")
	}
	mandatoryIDs := league.MandatoryDocIDs(h.app, comp)
	ack, err := league.FindOrNewAck(h.app, comp.Id, e.Auth.Id)
	if err != nil {
		return alertError(e, "Error al registrar la lectura")
	}
	ack.Set("documents", mandatoryIDs)
	if err := h.app.Save(ack); err != nil {
		return alertError(e, "Error al registrar la lectura")
	}
	return redirectHX(e, "/competition/"+comp.Id)
}

func (h *PublicHandler) buildCompetitionData(comp *core.Record, rounds []RoundView, autoExpandRound int) map[string]any {
	id := comp.Id
	var standings []league.StandingRowFull
	hasPenalties := false
	if comp.GetString("type") == "league" {
		rows, _ := h.leagueSvc.ComputeStandings(id)
		hasPlayed := false
		for _, s := range rows {
			if s.Played > 0 {
				hasPlayed = true
			}
			if s.Penalty > 0 {
				hasPenalties = true
			}
		}
		if len(rows) >= 2 && hasPlayed {
			standings = rows
		}
	}

	var awards []league.Award
	if !comp.GetBool("active") {
		awards = h.leagueSvc.Awards(id)
	}

	isPlayoff := league.IsPlayoff(comp)
	var bracket []BracketRound
	if isPlayoff && len(rounds) > 0 {
		maxRound := rounds[len(rounds)-1].RoundNumber
		bracket = buildBracket(rounds, maxRound)
	}

	return map[string]any{
		"Competition":     comp,
		"Rounds":          rounds,
		"Standings":       standings,
		"Awards":          awards,
		"IsArchived":      !comp.GetBool("active"),
		"AutoExpandRound": autoExpandRound,
		"HasPenalties":    hasPenalties,
		"IsPlayoff":       isPlayoff,
		"Bracket":         bracket,
	}
}

func collectPairNames(app core.App, matches []*core.Record) map[string]string {
	ids := make(map[string]struct{})
	for _, m := range matches {
		ids[m.GetString("pair1")] = struct{}{}
		ids[m.GetString("pair2")] = struct{}{}
	}
	slice := make([]string, 0, len(ids))
	for pid := range ids {
		slice = append(slice, pid)
	}
	return league.PairNames(app, slice)
}

func buildRounds(matches []*core.Record, pairNames map[string]string, playerPairIDs map[string]struct{}, showAll bool) []RoundView {
	roundMap := map[int][]MatchCard{}
	for _, m := range matches {
		mc := NewMatchRow(m, pairNames, playerPairIDs)
		if !showAll && !mc.IsMyMatch {
			continue
		}
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], mc)
	}
	for rn, ms := range roundMap {
		sort.SliceStable(ms, func(i, j int) bool {
			return ms[i].IsMyMatch && !ms[j].IsMyMatch
		})
		roundMap[rn] = ms
	}
	roundNums := make([]int, 0, len(roundMap))
	for rn := range roundMap {
		roundNums = append(roundNums, rn)
	}
	sort.Ints(roundNums)
	rounds := make([]RoundView, 0, len(roundNums))
	for _, rn := range roundNums {
		rounds = append(rounds, RoundView{RoundNumber: rn, Matches: roundMap[rn]})
	}
	for ri := 1; ri < len(rounds); ri++ {
		prevRound := rounds[ri-1].RoundNumber
		for mi := range rounds[ri].Matches {
			rounds[ri].Matches[mi].PopulateFeeder(prevRound, mi)
		}
	}
	return rounds
}

func firstIncompleteRound(rounds []RoundView) int {
	for _, rv := range rounds {
		for _, mv := range rv.Matches {
			if mv.Match.GetString("status") != league.StatusFinal {
				return rv.RoundNumber
			}
		}
	}
	return 0
}
