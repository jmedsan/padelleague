package handlers

import (
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

type PublicHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewPublicHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *PublicHandler {
	return &PublicHandler{app: app, renderPage: renderPage}
}

type HomeCompetition struct {
	Competition    *core.Record
	PendingMatches int
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

	pairs, _ := findPairsForPlayer(h.app, userID)

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
		compPairIDs := c.GetStringSlice("pairs")
		inComp := false
		for _, pid := range compPairIDs {
			if playerPairIDs[pid] {
				inComp = true
				break
			}
		}
		if !inComp {
			continue
		}

		pending := 0
		pendingMatches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid} && status = 'pending'",
			"", 0, 0,
			map[string]any{"cid": c.Id})
		for _, m := range pendingMatches {
			p1 := m.GetString("pair1")
			p2 := m.GetString("pair2")
			if !playerPairIDs[p1] && !playerPairIDs[p2] {
				continue
			}
			pending++

			if nextMatch == nil {
				opponent := p1
				if playerPairIDs[p1] {
					opponent = p2
				}
				opponentName := "?"
				if pair, err := h.app.FindRecordById("pairs", opponent); err == nil {
					opponentName = pair.GetString("name")
				}
				nm := &NextMatch{
					MatchID:         m.Id,
					Opponent:        opponentName,
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
				nextMatch = nm
			}

			// Check for unanswered proposals from rival team
			proposals, _ := h.app.FindRecordsByFilter("match_messages",
				"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
				"-created", 1, 0, map[string]any{"mid": m.Id})
			if len(proposals) > 0 {
				prop := proposals[0]
				proposerID := prop.GetString("author")
				proposerTeam, _ := getPlayerTeam(h.app, proposerID, m)
				playerTeam := 1
				if playerPairIDs[p2] {
					playerTeam = 2
				}
				if proposerTeam != playerTeam {
					opponent := p1
					if playerPairIDs[p1] {
						opponent = p2
					}
					opName := "?"
					if pair, err := h.app.FindRecordById("pairs", opponent); err == nil {
						opName = pair.GetString("name")
					}
					pendingActions = append(pendingActions, PendingAction{
						MatchID:     m.Id,
						Opponent:    opName,
						ActionType:  "respond_proposal",
						Description: "Propuesta de horario pendiente",
					})
				}
			}
		}

		// Unconfirmed scores
		confirmed, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid} && status = 'confirmed'",
			"", 0, 0, map[string]any{"cid": c.Id})
		for _, m := range confirmed {
			p1 := m.GetString("pair1")
			p2 := m.GetString("pair2")
			if !playerPairIDs[p1] && !playerPairIDs[p2] {
				continue
			}
			submittedBy := m.GetString("submitted_by")
			submitterTeam, _ := getPlayerTeam(h.app, submittedBy, m)
			playerTeam := 1
			if playerPairIDs[p2] {
				playerTeam = 2
			}
			if submitterTeam == playerTeam {
				continue
			}
			opponent := p1
			if playerPairIDs[p1] {
				opponent = p2
			}
			opName := "?"
			if pair, err := h.app.FindRecordById("pairs", opponent); err == nil {
				opName = pair.GetString("name")
			}
			pendingActions = append(pendingActions, PendingAction{
				MatchID:     m.Id,
				Opponent:    opName,
				ActionType:  "confirm_score",
				Description: "Confirmar resultado: " + m.GetString("scores"),
			})
		}

		// Recent results
		finals, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid} && status = 'final'",
			"-updated", 5, 0, map[string]any{"cid": c.Id})
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
			winner := m.GetString("winner")
			if winner == p1 {
				winnerName = p1Name
			} else if winner == p2 {
				winnerName = p2Name
			}
			recentResults = append(recentResults, RecentResult{
				MatchID:         m.Id,
				Pair1Name:       p1Name,
				Pair2Name:       p2Name,
				Score:           m.GetString("scores"),
				WinnerName:      winnerName,
				CompetitionName: c.GetString("name"),
				UpdatedAt:       m.GetString("updated"),
			})
		}

		comps = append(comps, HomeCompetition{
			Competition:    c,
			PendingMatches: pending,
		})
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

type RoundView struct {
	RoundNumber int
	Matches     []RoundMatchView
}

type RoundMatchView struct {
	Match      *core.Record
	Pair1      string
	Pair2      string
	IsMyMatch  bool
}

func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	userID := e.Auth.Id
	pairs, _ := findPairsForPlayer(h.app, userID)
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
	pairNames, _ := expandPairNames(h.app, pairIDSlice)

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

	roundNums := make([]int, 0, len(roundMap))
	for rn := range roundMap {
		roundNums = append(roundNums, rn)
	}
	sort.Ints(roundNums)

	rounds := make([]RoundView, 0, len(roundNums))
	for _, rn := range roundNums {
		rounds = append(rounds, RoundView{RoundNumber: rn, Matches: roundMap[rn]})
	}

	var standings []StandingRowFull
	if comp.GetString("type") == "league" {
		standings, _ = ComputeStandings(h.app, id)
	}

	var awards []Award
	if !comp.GetBool("active") {
		awards = computeAwards(h.app, id)
	}

	return h.renderPage(e, "competition.html", map[string]any{
		"Competition": comp,
		"Rounds":      rounds,
		"Standings":   standings,
		"Awards":      awards,
		"IsArchived":  !comp.GetBool("active"),
	})
}
