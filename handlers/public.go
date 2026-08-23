package handlers

import (
	"sort"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

type PublicHandler struct {
	app        core.App
	leagueSvc  *league.Service
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPublicHandler(app core.App, leagueSvc *league.Service, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PublicHandler {
	return &PublicHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage}
}

type PendingMatchDetail struct {
	MatchID     string
	Opponent    string
	RoundNumber int
}

type HomeCompetition struct {
	Competition    *core.Record
	PendingMatches int
	PendingDetails []PendingMatchDetail
}

type NextMatch struct {
	MatchID         string
	Opponent        string
	CompetitionName string
	RoundNumber     int
	ScheduleStatus  string // "unscheduled", "proposed", "confirmed"
	ProposedDate    string
	ProposedVenue   string
}

type PendingAction struct {
	MatchID     string
	Opponent    string
	ActionType  string // "confirm_score", "respond_proposal"
	Description string
}

type RecentResult struct {
	MatchID         string
	Pair1Name       string
	Pair2Name       string
	Score           string
	WinnerName      string
	CompetitionName string
	UpdatedAt       string
}

func (h *PublicHandler) Home(e *core.RequestEvent) error {
	userID := e.Auth.Id

	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = true
	}

	allComps, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "name", 0, 0, nil)

	var comps []HomeCompetition
	var nextMatch *NextMatch
	var pendingActions []PendingAction
	var recentResults []RecentResult

	for _, c := range allComps {
		if !h.playerInCompetition(c, playerPairIDs) {
			continue
		}
		hc, nm, actions, results := h.buildHomeCompetition(c, playerPairIDs, nextMatch == nil)
		comps = append(comps, hc)
		if nextMatch == nil && nm != nil {
			nextMatch = nm
		}
		pendingActions = append(pendingActions, actions...)
		recentResults = append(recentResults, results...)
	}

	sort.Slice(recentResults, func(i, j int) bool {
		return recentResults[i].UpdatedAt > recentResults[j].UpdatedAt
	})
	if len(recentResults) > 5 {
		recentResults = recentResults[:5]
	}

	return h.renderPage(e, "home.html", map[string]any{
		"Competitions":   comps,
		"NextMatch":      nextMatch,
		"PendingActions": pendingActions,
		"RecentResults":  recentResults,
	})
}

func (h *PublicHandler) playerInCompetition(c *core.Record, playerPairIDs map[string]bool) bool {
	for _, pid := range c.GetStringSlice("pairs") {
		if playerPairIDs[pid] {
			return true
		}
	}
	return false
}

func (h *PublicHandler) opponentName(m *core.Record, playerPairIDs map[string]bool) string {
	opponent := m.GetString("pair1")
	if playerPairIDs[opponent] {
		opponent = m.GetString("pair2")
	}
	if pair, err := h.app.FindRecordById("pairs", opponent); err == nil {
		return pair.GetString("name")
	}
	return "?"
}

func (h *PublicHandler) buildHomeCompetition(c *core.Record, playerPairIDs map[string]bool, needNext bool) (HomeCompetition, *NextMatch, []PendingAction, []RecentResult) {
	pending := 0
	var pendingDetails []PendingMatchDetail
	var nextMatch *NextMatch
	var actions []PendingAction

	pendingMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'pending'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})

	for _, m := range pendingMatches {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		if !playerPairIDs[p1] && !playerPairIDs[p2] {
			continue
		}
		pending++

		if len(pendingDetails) < 5 {
			pendingDetails = append(pendingDetails, PendingMatchDetail{
				MatchID:     m.Id,
				Opponent:    h.opponentName(m, playerPairIDs),
				RoundNumber: int(m.GetFloat("round_number")),
			})
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

	hc := HomeCompetition{
		Competition:    c,
		PendingMatches: pending,
		PendingDetails: pendingDetails,
	}
	return hc, nextMatch, actions, results
}

func (h *PublicHandler) buildNextMatch(m *core.Record, c *core.Record, playerPairIDs map[string]bool) *NextMatch {
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
		prop := proposals[0]
		pd := ParseProposalData(prop.GetString("proposal_data"))
		if prop.GetString("proposal_status") == "accepted" {
			nm.ScheduleStatus = "confirmed"
		} else {
			nm.ScheduleStatus = "proposed"
		}
		if pd != nil {
			nm.ProposedDate = pd.Date + " " + pd.Time
			if pd.VenueName != "" {
				nm.ProposedVenue = pd.VenueName
			} else if pd.VenueText != "" {
				nm.ProposedVenue = pd.VenueText
			}
		}
	}
	return nm
}

func (h *PublicHandler) checkPendingProposal(m *core.Record, playerPairIDs map[string]bool) *PendingAction {
	proposals, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
		"-created", 1, 0, map[string]any{"mid": m.Id})
	if len(proposals) == 0 {
		return nil
	}
	prop := proposals[0]
	proposerTeam, _ := league.PlayerTeam(h.app, prop.GetString("author"), m)
	playerTeam := 1
	if playerPairIDs[m.GetString("pair2")] {
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

func (h *PublicHandler) findUnconfirmedScores(c *core.Record, playerPairIDs map[string]bool) []PendingAction {
	confirmed, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'confirmed'",
		"", 0, 0, map[string]any{"cid": c.Id})
	var actions []PendingAction
	for _, m := range confirmed {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		if !playerPairIDs[p1] && !playerPairIDs[p2] {
			continue
		}
		submitterTeam, _ := league.PlayerTeam(h.app, m.GetString("submitted_by"), m)
		playerTeam := 1
		if playerPairIDs[p2] {
			playerTeam = 2
		}
		if submitterTeam == playerTeam {
			continue
		}
		actions = append(actions, PendingAction{
			MatchID:     m.Id,
			Opponent:    h.opponentName(m, playerPairIDs),
			ActionType:  "confirm_score",
			Description: "Confirmar resultado: " + m.GetString("scores"),
		})
	}
	return actions
}

func (h *PublicHandler) findRecentResults(c *core.Record) []RecentResult {
	finals, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'",
		"-updated", 5, 0, map[string]any{"cid": c.Id})
	var results []RecentResult
	for _, m := range finals {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		p1Name, p2Name := "?", "?"
		if pair, err := h.app.FindRecordById("pairs", p1); err == nil {
			p1Name = pair.GetString("name")
		}
		if pair, err := h.app.FindRecordById("pairs", p2); err == nil {
			p2Name = pair.GetString("name")
		}
		winnerName := ""
		switch m.GetString("winner") {
		case p1:
			winnerName = p1Name
		case p2:
			winnerName = p2Name
		}
		results = append(results, RecentResult{
			MatchID:         m.Id,
			Pair1Name:       p1Name,
			Pair2Name:       p2Name,
			Score:           m.GetString("scores"),
			WinnerName:      winnerName,
			CompetitionName: c.GetString("name"),
			UpdatedAt:       m.GetString("updated"),
		})
	}
	return results
}

type RoundView struct {
	RoundNumber int
	Matches     []RoundMatchView
}

type RoundMatchView struct {
	Match     *core.Record
	Pair1     string
	Pair2     string
	IsMyMatch bool
}

func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = true
	}

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}",
		"", 0, 0,
		map[string]any{"cid": id})

	allPairIDs := make(map[string]bool)
	for _, m := range matches {
		allPairIDs[m.GetString("pair1")] = true
		allPairIDs[m.GetString("pair2")] = true
	}

	pairIDSlice := make([]string, 0, len(allPairIDs))
	for pid := range allPairIDs {
		pairIDSlice = append(pairIDSlice, pid)
	}
	pairNames := league.PairNames(h.app, pairIDSlice)

	roundMap := map[int][]RoundMatchView{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		roundMap[rn] = append(roundMap[rn], RoundMatchView{
			Match:     m,
			Pair1:     pairNames[p1],
			Pair2:     pairNames[p2],
			IsMyMatch: playerPairIDs[p1] || playerPairIDs[p2],
		})
	}

	for rn, matches := range roundMap {
		sort.SliceStable(matches, func(i, j int) bool {
			return matches[i].IsMyMatch && !matches[j].IsMyMatch
		})
		roundMap[rn] = matches
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

	autoExpandRound := 0
	for _, rv := range rounds {
		for _, mv := range rv.Matches {
			if mv.Match.GetString("status") != league.StatusFinal {
				autoExpandRound = rv.RoundNumber
				break
			}
		}
		if autoExpandRound > 0 {
			break
		}
	}

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

	return h.renderPage(e, "competition.html", map[string]any{
		"Competition":     comp,
		"Rounds":          rounds,
		"Standings":       standings,
		"Awards":          awards,
		"IsArchived":      !comp.GetBool("active"),
		"AutoExpandRound": autoExpandRound,
		"HasPenalties":    hasPenalties,
	})
}
