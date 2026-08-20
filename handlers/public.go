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
		matches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid} && status = 'pending'",
			"", 0, 0,
			map[string]any{"cid": c.Id})
		for _, m := range matches {
			p1 := m.GetString("pair1")
			p2 := m.GetString("pair2")
			if playerPairIDs[p1] || playerPairIDs[p2] {
				pending++
			}
		}

		comps = append(comps, HomeCompetition{
			Competition:    c,
			PendingMatches: pending,
		})
	}

	return h.renderPage(e, "home.html", map[string]any{
		"Competitions": comps,
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
