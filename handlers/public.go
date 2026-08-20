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

func (h *PublicHandler) Home(e *core.RequestEvent) error {
	competitions, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "", 0, 0, nil)

	return h.renderPage(e, "home.html", map[string]any{
		"Competitions": competitions,
	})
}

type RoundView struct {
	RoundNumber int
	Matches     []RoundMatchView
}

type RoundMatchView struct {
	Match *core.Record
	Pair1 string
	Pair2 string
}

func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.Redirect(302, "/")
	}

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}",
		"round_number", 0, 0,
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
		roundMap[rn] = append(roundMap[rn], RoundMatchView{
			Match: m,
			Pair1: pairNames[m.GetString("pair1")],
			Pair2: pairNames[m.GetString("pair2")],
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
