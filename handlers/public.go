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
	CompetitionName string
	RoundNumber     int
	ScheduleStatus  string // "unscheduled", "proposed", "confirmed"
	ProposedDate    string
	ProposedVenue   string
}

// PendingAction represents an action the player needs to take on a match.
type PendingAction struct {
	MatchID     string
	Opponent    string
	ActionType  string // "confirm_score", "respond_proposal"
	Description string
}

// Home renders the player's dashboard with competitions, next match, and actions.
func (h *PublicHandler) Home(e *core.RequestEvent) error {
	data := map[string]any{}

	activeComps, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "name", 0, 0, nil)

	if render.AdminView(e) {
		h.addAdminHomeData(data, activeComps)
		return h.renderPage(e, "home.html", data)
	}

	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}

	var comps []CompetitionView
	var nextMatch *NextMatch
	var pendingActions []PendingAction
	var recentResults []MatchCard

	for _, c := range activeComps {
		if !h.playerInCompetition(c, playerPairIDs) {
			continue
		}
		parts := h.buildHomeCompetition(c, playerPairIDs, nextMatch == nil)
		comps = append(comps, parts.Comp)
		if nextMatch == nil && parts.Next != nil {
			nextMatch = parts.Next
		}
		pendingActions = append(pendingActions, parts.Pending...)
		recentResults = append(recentResults, parts.Recent...)
	}

	sort.Slice(recentResults, func(i, j int) bool {
		return recentResults[i].Match.GetString("created") > recentResults[j].Match.GetString("created")
	})
	if len(recentResults) > 5 {
		recentResults = recentResults[:5]
	}

	urgentTasks, _ := league.PlayerTasks(h.app, userID, time.Now())

	data["Competitions"] = comps
	data["NextMatch"] = nextMatch
	data["PendingActions"] = pendingActions
	data["RecentResults"] = recentResults
	data["UrgentTasks"] = urgentTasks

	if slices.Contains(e.Auth.GetStringSlice("roles"), "player") {
		if steps := h.onboardingSteps(e.Auth, activeComps); len(steps) > 0 {
			data["OnboardSteps"] = steps
		}
	}

	return h.renderPage(e, "home.html", data)
}

func (h *PublicHandler) addAdminHomeData(data map[string]any, activeComps []*core.Record) {
	setups, alerts, _ := league.AdminDashboard(h.app, time.Now())
	data["AdminSetups"] = setups
	data["AdminCards"], data["AdminAlerts"] = h.splitAdminAlerts(alerts)

	existing, _ := h.app.FindRecordsByFilter("competitions", "", "", 1, 0, nil)
	data["AdminBootstrap"] = len(existing) == 0
	data["PlayoffPrompts"] = league.PlayoffPrompts(h.app, activeComps, time.Now())
}

// onboardingSteps returns the player onboarding checklist, or nil when every
// actionable step is done (so the template hides the card).
func (h *PublicHandler) onboardingSteps(user *core.Record, activeComps []*core.Record) []OnboardStep {
	profileDone := user.GetString("display_name") != ""

	reglamentoDone := true
	reglamentoURL := "/"
	for _, c := range activeComps {
		if pending := league.UnacknowledgedMandatory(h.app, c, user.Id); len(pending) > 0 {
			reglamentoDone = false
			reglamentoURL = fmt.Sprintf("/competition/%s#documentos", c.Id)
			break
		}
	}

	if profileDone && reglamentoDone {
		return nil
	}

	return []OnboardStep{
		{Label: "Completa tu perfil", URL: "/profile/complete", Done: profileDone},
		{Label: "Lee el reglamento", URL: reglamentoURL, Done: reglamentoDone},
	}
}

func (h *PublicHandler) splitAdminAlerts(alerts []league.AdminAlert) ([]MatchCard, []league.AdminAlert) {
	var cards []MatchCard
	var otherAlerts []league.AdminAlert
	for _, alert := range alerts {
		if alert.Kind == "dispute" || alert.Kind == "walkover" {
			if match, err := h.app.FindRecordById("matches", alert.MatchID); err == nil {
				cards = append(cards, NewMatchCard(h.app, match, AdminSummary, ""))
				continue
			}
		}
		otherAlerts = append(otherAlerts, alert)
	}
	return cards, otherAlerts
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
	Comp    CompetitionView
	Next    *NextMatch
	Pending []PendingAction
	Recent  []MatchCard
}

func (h *PublicHandler) buildHomeCompetition(c *core.Record, playerPairIDs map[string]struct{}, needNext bool) homeCompetitionParts {
	pending := 0
	var pendingDetails []MatchCard
	var nextMatch *NextMatch
	var actions []PendingAction

	pendingMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && (status = 'pending' || status = 'scheduled')",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})

	pairNames := collectPairNames(h.app, pendingMatches)

	for _, m := range pendingMatches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		_, hasP1 := playerPairIDs[p1]
		_, hasP2 := playerPairIDs[p2]
		if !hasP1 && !hasP2 {
			continue
		}
		pending++

		if len(pendingDetails) < 5 {
			pendingDetails = append(pendingDetails, NewMatchRow(m, pairNames, playerPairIDs))
		}

		if needNext && nextMatch == nil {
			nextMatch = h.buildNextMatch(m, c, playerPairIDs)
		}

		if pa := h.checkPendingProposal(m, playerPairIDs); pa != nil {
			actions = append(actions, *pa)
		}
	}

	actions = append(actions, h.findUnconfirmedScores(c, playerPairIDs)...)
	results := h.findRecentResults(c)

	return homeCompetitionParts{
		Comp:    NewHomeCompetitionView(c, pending, pendingDetails),
		Next:    nextMatch,
		Pending: actions,
		Recent:  results,
	}
}

func (h *PublicHandler) buildNextMatch(m *core.Record, c *core.Record, playerPairIDs map[string]struct{}) *NextMatch {
	nm := &NextMatch{
		MatchID:         m.Id,
		Opponent:        h.opponentName(m, playerPairIDs),
		CompetitionName: c.GetString("name"),
		RoundNumber:     int(m.GetFloat("round_number")),
		ScheduleStatus:  "unscheduled",
	}
	proposals, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status != 'rejected' && proposal_status != 'superseded'",
		"-created", 1, 0, map[string]any{"mid": m.Id})
	if len(proposals) > 0 {
		applyProposalToNextMatch(nm, proposals[0])
	}
	return nm
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
	proposals, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
		"-created", 1, 0, map[string]any{"mid": m.Id})
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
	confirmed, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'confirmed'",
		"-created", 0, 0, map[string]any{"cid": c.Id})
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
	proposals, _ := h.app.FindRecordsByFilter("match_messages",
		"type = 'result_submission' && proposal_status = 'pending'",
		"-created", 0, 0, nil)
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

func (h *PublicHandler) findRecentResults(c *core.Record) []MatchCard {
	finals, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"-created", 5, 0, map[string]any{"cid": c.Id})
	pairNames := collectPairNames(h.app, finals)
	empty := map[string]struct{}{}
	var results []MatchCard
	for _, m := range finals {
		mc := NewMatchRow(m, pairNames, empty)
		mc.CompetitionName = c.GetString("name")
		results = append(results, mc)
	}
	return results
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

	if league.IsParticipant(comp, playerPairIDs) {
		if pending := league.UnacknowledgedMandatory(h.app, comp, userID); len(pending) > 0 {
			allDocs := league.AttachedDocuments(h.app, comp)
			mandatoryIDs := make([]string, len(pending))
			for i, d := range pending {
				mandatoryIDs[i] = d.Id
			}
			docViews := make([]DocumentView, len(allDocs))
			for i, d := range allDocs {
				docViews[i] = NewDocumentView(d, PlayerRow)
			}
			return h.renderPage(e, "competition-docs-gate.html", map[string]any{
				"Competition":   comp,
				"DocumentViews": docViews,
				"MandatoryIDs":  mandatoryIDs,
			})
		}
	}

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}",
		"round_number,created", 0, 0,
		map[string]any{"cid": id})

	pairNames := collectPairNames(h.app, matches)

	rounds := buildRounds(matches, pairNames, playerPairIDs)
	autoExpandRound := firstIncompleteRound(rounds)

	data := h.buildCompetitionData(comp, rounds, pairNames, autoExpandRound)
	data["PlayerPairIDs"] = playerPairIDs
	docs := league.AttachedDocuments(h.app, comp)
	if len(docs) > 0 {
		docViews := make([]DocumentView, len(docs))
		for i, d := range docs {
			docViews[i] = NewDocumentView(d, PlayerRow)
		}
		data["DocumentViews"] = docViews
	}
	return h.renderPage(e, "competition.html", data)
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

func (h *PublicHandler) buildCompetitionData(comp *core.Record, rounds []RoundView, pairNames map[string]string, autoExpandRound int) map[string]any {
	id := comp.Id
	var standings []league.StandingRowFull
	hasPenalties := false
	if comp.GetString("type") == "league" {
		standings, _ = h.leagueSvc.ComputeStandings(id)
		for _, s := range standings {
			if s.Penalty > 0 {
				hasPenalties = true
				break
			}
		}
	}

	var awards []league.Award
	if !comp.GetBool("active") {
		awards = h.leagueSvc.Awards(id)
	}

	compPairs := buildCompPairs(standings, pairNames)

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
		"CompPairs":       compPairs,
		"IsPlayoff":       isPlayoff,
		"Bracket":         bracket,
	}
}

type pairOption struct {
	ID   string
	Name string
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

func buildCompPairs(standings []league.StandingRowFull, pairNames map[string]string) []pairOption {
	if len(standings) > 0 {
		pairs := make([]pairOption, 0, len(standings))
		for _, s := range standings {
			pairs = append(pairs, pairOption{ID: s.PairID, Name: s.PairName})
		}
		return pairs
	}
	var pairs []pairOption
	for id, name := range pairNames {
		if id != "" {
			pairs = append(pairs, pairOption{ID: id, Name: name})
		}
	}
	return pairs
}

func buildRounds(matches []*core.Record, pairNames map[string]string, playerPairIDs map[string]struct{}) []RoundView {
	roundMap := map[int][]MatchCard{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], NewMatchRow(m, pairNames, playerPairIDs))
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
