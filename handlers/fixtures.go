package handlers

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

type FixtureHandler struct {
	app        core.App
	leagueSvc  *league.Service
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewFixtureHandler(app core.App, leagueSvc *league.Service, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *FixtureHandler {
	return &FixtureHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage}
}

func (h *FixtureHandler) GenerateFixtures(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	confirm := e.Request.URL.Query().Get("confirm") == "true"

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	existingMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:id}", "", 0, 0,
		map[string]any{"id": compID})

	if len(existingMatches) > 0 && !confirm {
		return e.HTML(http.StatusOK, fmt.Sprintf(`
			<div class="alert alert-warning">
				<span>Ya existen %d partidos. ¿Desea regenerar? Esto eliminará los partidos existentes.</span>
				<button hx-post="/admin/competitions/%s/generate?confirm=true" hx-target="#generate-result" class="btn btn-sm btn-warning">Confirmar</button>
			</div>`, len(existingMatches), compID))
	}

	pairIDs := comp.GetStringSlice("pairs")

	if len(pairIDs) < 2 {
		return alertError(e, "Se necesitan al menos 2 parejas")
	}

	compType := comp.GetString("type")

	err = h.app.RunInTransaction(func(txApp core.App) error {
		for _, m := range existingMatches {
			if err := txApp.Delete(m); err != nil {
				return err
			}
		}

		if compType == "league" {
			return h.generateLeague(txApp, compID, pairIDs, comp.GetBool("play_twice"))
		}
		return h.generatePlayoff(txApp, compID, pairIDs, comp)
	})

	if err != nil {
		slog.Error("generate fixtures failed", "competition", compID, "err", err)
		return alertError(e, "Error al generar partidos")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

func (h *FixtureHandler) generateLeague(txApp core.App, compID string, pairIDs []string, double bool) error {
	rounds := league.RoundRobin(pairIDs, double)

	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}

	for _, round := range rounds {
		for _, m := range round.Matches {
			match := core.NewRecord(matchCol)
			match.Set("competition", compID)
			match.Set("round_number", round.Number)
			match.Set("matches_to_win", 1)
			match.Set("pair1", m.Home)
			match.Set("pair2", m.Away)
			match.Set("status", StatusPending)
			if err := txApp.Save(match); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *FixtureHandler) generatePlayoff(txApp core.App, compID string, pairIDs []string, comp *core.Record) error {
	n := len(pairIDs)
	numRounds := int(math.Ceil(math.Log2(float64(n))))
	bracketSize := 1 << numRounds

	seeding := NewCompetitionHandler(h.app, h.leagueSvc, h.renderPage).getSeeding(comp)

	type seededPair struct {
		id   string
		seed int
	}
	sp := make([]seededPair, len(pairIDs))
	for i, pid := range pairIDs {
		sp[i] = seededPair{id: pid, seed: seeding[pid]}
	}
	sort.Slice(sp, func(i, j int) bool {
		si, sj := sp[i].seed, sp[j].seed
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

	slots := make([]string, bracketSize)
	for i, p := range sp {
		slots[i] = p.id
	}

	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
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
		if bs.pair2 == "" {
			advancers[i] = bs.pair1
			continue
		}
		if bs.pair1 == "" {
			advancers[i] = bs.pair2
			continue
		}
		match := core.NewRecord(matchCol)
		match.Set("competition", compID)
		match.Set("round_number", 1)
		match.Set("matches_to_win", 1)
		match.Set("pair1", bs.pair1)
		match.Set("pair2", bs.pair2)
		match.Set("status", StatusPending)
		if err := txApp.Save(match); err != nil {
			return err
		}
	}

	currentAdvancers := advancers
	for r := 2; r <= numRounds; r++ {
		numMatches := len(currentAdvancers) / 2
		nextAdvancers := make([]string, numMatches)
		for i := 0; i < numMatches; i++ {
			p1 := currentAdvancers[i*2]
			p2 := currentAdvancers[i*2+1]

			match := core.NewRecord(matchCol)
			match.Set("competition", compID)
			match.Set("round_number", r)
			match.Set("matches_to_win", 1)
			if p1 != "" {
				match.Set("pair1", p1)
			}
			if p2 != "" {
				match.Set("pair2", p2)
			}
			match.Set("status", StatusPending)
			if err := txApp.Save(match); err != nil {
				return err
			}
		}
		currentAdvancers = nextAdvancers
	}

	return nil
}
