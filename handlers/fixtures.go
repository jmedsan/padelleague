package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

type Round struct {
	Number  int
	Matches []RoundMatch
}

type RoundMatch struct {
	Home string
	Away string
}

func generateRoundRobin(pairIDs []string, double bool) []Round {
	n := len(pairIDs)
	if n < 2 {
		return nil
	}

	pairs := make([]string, len(pairIDs))
	copy(pairs, pairIDs)

	if n%2 == 1 {
		pairs = append(pairs, "")
		n++
	}

	rounds := make([]Round, 0, n-1)
	for r := 0; r < n-1; r++ {
		var matches []RoundMatch
		for i := 0; i < n/2; i++ {
			home := pairs[i]
			away := pairs[n-1-i]
			if home != "" && away != "" {
				matches = append(matches, RoundMatch{Home: home, Away: away})
			}
		}
		rounds = append(rounds, Round{Number: r + 1, Matches: matches})
		last := pairs[n-1]
		copy(pairs[2:], pairs[1:n-1])
		pairs[1] = last
	}

	if double {
		half := len(rounds)
		for i := 0; i < half; i++ {
			var swapped []RoundMatch
			for _, m := range rounds[i].Matches {
				swapped = append(swapped, RoundMatch{Home: m.Away, Away: m.Home})
			}
			rounds = append(rounds, Round{Number: half + i + 1, Matches: swapped})
		}
	}

	return rounds
}

type FixtureHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewFixtureHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *FixtureHandler {
	return &FixtureHandler{app: app, renderPage: renderPage}
}

func (h *FixtureHandler) GenerateFixtures(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	confirm := e.Request.URL.Query().Get("confirm") == "true"

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	existingMatchdays, _ := h.app.FindRecordsByFilter("matchdays",
		"competition = {:id}", "-round_number", 0, 0,
		map[string]any{"id": compID})

	if len(existingMatchdays) > 0 && !confirm {
		return e.HTML(http.StatusOK, fmt.Sprintf(`
			<div class="alert alert-warning">
				<span>Ya existen %d jornadas. ¿Desea regenerar? Esto eliminará los partidos existentes.</span>
				<button hx-post="/admin/competitions/%s/generate?confirm=true" hx-target="#generate-result" class="btn btn-sm btn-warning">Confirmar</button>
			</div>`, len(existingMatchdays), compID))
	}

	cpRecords, _ := h.app.FindRecordsByFilter("competition_pairs",
		"competition = {:id}", "seed", 0, 0,
		map[string]any{"id": compID})

	if len(cpRecords) < 2 {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Se necesitan al menos 2 parejas</div>`)
	}

	pairIDs := make([]string, len(cpRecords))
	for i, cp := range cpRecords {
		pairIDs[i] = cp.GetString("pair")
	}

	compType := comp.GetString("type")

	err = h.app.RunInTransaction(func(txApp core.App) error {
		for _, md := range existingMatchdays {
			matches, _ := txApp.FindRecordsByFilter("matches",
				"matchday = {:mdid}", "", 0, 0,
				map[string]any{"mdid": md.Id})
			for _, m := range matches {
				if err := txApp.Delete(m); err != nil {
					return err
				}
			}
			if err := txApp.Delete(md); err != nil {
				return err
			}
		}

		if compType == "league" {
			return h.generateLeague(txApp, compID, pairIDs, comp.GetBool("play_twice"))
		}
		return h.generatePlayoff(txApp, compID, cpRecords)
	})

	if err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	var allPlayerIDs []string
	seen := map[string]bool{}
	for _, pid := range pairIDs {
		for _, uid := range getPlayersForPair(h.app, pid) {
			if !seen[uid] {
				seen[uid] = true
				allPlayerIDs = append(allPlayerIDs, uid)
			}
		}
	}
	notifyPlayers(h.app, allPlayerIDs, "match_assigned", "Calendario generado",
		fmt.Sprintf("Se ha generado el calendario de la competición."), "")
	emailNotifyPlayers(h.app, allPlayerIDs, "Calendario generado",
		"Se ha generado el calendario de la competición.", "")

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID+"/pairs")
	return e.NoContent(http.StatusNoContent)
}

func (h *FixtureHandler) generateLeague(txApp core.App, compID string, pairIDs []string, double bool) error {
	rounds := generateRoundRobin(pairIDs, double)

	matchdayCol, err := txApp.FindCollectionByNameOrId("matchdays")
	if err != nil {
		return err
	}
	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}

	for _, round := range rounds {
		md := core.NewRecord(matchdayCol)
		md.Set("competition", compID)
		md.Set("round_number", round.Number)
		md.Set("matches_to_win", 1)
		if err := txApp.Save(md); err != nil {
			return err
		}

		for _, m := range round.Matches {
			match := core.NewRecord(matchCol)
			match.Set("matchday", md.Id)
			match.Set("pair1", m.Home)
			match.Set("pair2", m.Away)
			match.Set("status", "pending")
			if err := txApp.Save(match); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *FixtureHandler) generatePlayoff(txApp core.App, compID string, cpRecords []*core.Record) error {
	n := len(cpRecords)
	numRounds := int(math.Ceil(math.Log2(float64(n))))
	bracketSize := 1 << numRounds

	sort.Slice(cpRecords, func(i, j int) bool {
		si := cpRecords[i].GetFloat("seed")
		sj := cpRecords[j].GetFloat("seed")
		if si == 0 && sj == 0 {
			return i < j
		}
		if si == 0 {
			return false
		}
		if sj == 0 {
			return true
		}
		return si < sj
	})

	// Slot pairs into bracket positions: top seed vs bottom, 2nd vs 2nd-bottom, etc.
	slots := make([]string, bracketSize)
	for i, cp := range cpRecords {
		slots[i] = cp.GetString("pair")
	}

	matchdayCol, err := txApp.FindCollectionByNameOrId("matchdays")
	if err != nil {
		return err
	}
	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}

	var matchdayIDs []string
	for r := 1; r <= numRounds; r++ {
		md := core.NewRecord(matchdayCol)
		md.Set("competition", compID)
		md.Set("round_number", r)
		md.Set("matches_to_win", 1)
		if err := txApp.Save(md); err != nil {
			return err
		}
		matchdayIDs = append(matchdayIDs, md.Id)
	}

	type bracketSlot struct {
		pair1 string
		pair2 string
	}

	firstRound := make([]bracketSlot, bracketSize/2)
	for i := 0; i < bracketSize/2; i++ {
		firstRound[i] = bracketSlot{
			pair1: slots[i],
			pair2: slots[bracketSize-1-i],
		}
	}

	advancers := make([]string, bracketSize/2)
	for i, bs := range firstRound {
		if bs.pair1 == "" && bs.pair2 == "" {
			continue
		}
		// Bye: one side empty, opponent auto-advances
		if bs.pair2 == "" {
			advancers[i] = bs.pair1
			continue
		}
		if bs.pair1 == "" {
			advancers[i] = bs.pair2
			continue
		}
		match := core.NewRecord(matchCol)
		match.Set("matchday", matchdayIDs[0])
		match.Set("pair1", bs.pair1)
		match.Set("pair2", bs.pair2)
		match.Set("status", "pending")
		if err := txApp.Save(match); err != nil {
			return err
		}
	}

	currentAdvancers := advancers
	for r := 1; r < numRounds; r++ {
		numMatches := len(currentAdvancers) / 2
		nextAdvancers := make([]string, numMatches)
		for i := 0; i < numMatches; i++ {
			p1 := currentAdvancers[i*2]
			p2 := currentAdvancers[i*2+1]

			// Both known from byes — create match directly
			if p1 != "" && p2 != "" {
				match := core.NewRecord(matchCol)
				match.Set("matchday", matchdayIDs[r])
				match.Set("pair1", p1)
				match.Set("pair2", p2)
				match.Set("status", "pending")
				if err := txApp.Save(match); err != nil {
					return err
				}
				continue
			}

			// At least one slot TBD — create placeholder match
			match := core.NewRecord(matchCol)
			match.Set("matchday", matchdayIDs[r])
			if p1 != "" {
				match.Set("pair1", p1)
			}
			if p2 != "" {
				match.Set("pair2", p2)
			}
			match.Set("status", "pending")
			if err := txApp.Save(match); err != nil {
				return err
			}
		}
		currentAdvancers = nextAdvancers
	}

	return nil
}

// AutoAdvancePlayoff checks if all matches in the current playoff round are
// final, and if so populates the next round with the winners.
func AutoAdvancePlayoff(app core.App, matchRecord *core.Record) error {
	matchday, err := app.FindRecordById("matchdays", matchRecord.GetString("matchday"))
	if err != nil {
		return nil
	}

	comp, err := app.FindRecordById("competitions", matchday.GetString("competition"))
	if err != nil || comp.GetString("type") != "playoff" {
		return nil
	}

	currentRound := int(matchday.GetFloat("round_number"))

	roundMatches, _ := app.FindRecordsByFilter("matches",
		"matchday = {:mdid}", "created", 0, 0,
		map[string]any{"mdid": matchday.Id})

	for _, m := range roundMatches {
		if m.GetString("status") != "final" {
			return nil
		}
	}

	nextMatchdays, _ := app.FindRecordsByFilter("matchdays",
		"competition = {:cid} && round_number = {:rn}", "", 1, 0,
		map[string]any{"cid": comp.Id, "rn": nextRound(currentRound)})

	if len(nextMatchdays) == 0 {
		return nil
	}

	var roundWinners []string
	for _, m := range roundMatches {
		roundWinners = append(roundWinners, m.GetString("winner"))
	}

	nextMatches, _ := app.FindRecordsByFilter("matches",
		"matchday = {:mdid}", "created", 0, 0,
		map[string]any{"mdid": nextMatchdays[0].Id})

	for i, nm := range nextMatches {
		p1Idx := i * 2
		p2Idx := i*2 + 1
		if p1Idx < len(roundWinners) && roundWinners[p1Idx] != "" {
			nm.Set("pair1", roundWinners[p1Idx])
		}
		if p2Idx < len(roundWinners) && roundWinners[p2Idx] != "" {
			nm.Set("pair2", roundWinners[p2Idx])
		}
		if err := app.Save(nm); err != nil {
			return err
		}
	}

	return nil
}

func nextRound(current int) int {
	return current + 1
}
