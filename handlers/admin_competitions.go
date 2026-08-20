package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

type CompetitionHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewCompetitionHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *CompetitionHandler {
	return &CompetitionHandler{app: app, renderPage: renderPage}
}

type CompetitionSummary struct {
	Competition  *core.Record
	PairsCount   int
	TotalMatches int
	PlayedMatches int
	DisputeCount int
}

func (h *CompetitionHandler) Dashboard(e *core.RequestEvent) error {
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0, nil)

	var active, inactive []CompetitionSummary
	for _, comp := range allComps {
		pairsCount := len(comp.GetStringSlice("pairs"))

		allMatches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": comp.Id})
		playedMatches := 0
		disputeCount := 0
		for _, m := range allMatches {
			if m.GetString("status") == "final" {
				playedMatches++
			}
			if m.GetString("status") == "disputed" {
				disputeCount++
			}
		}

		summary := CompetitionSummary{
			Competition:   comp,
			PairsCount:    pairsCount,
			TotalMatches:  len(allMatches),
			PlayedMatches: playedMatches,
			DisputeCount:  disputeCount,
		}

		if comp.GetBool("active") {
			active = append(active, summary)
		} else {
			inactive = append(inactive, summary)
		}
	}

	totalDisputes, _ := h.app.FindRecordsByFilter("matches",
		"status = 'disputed'", "", 0, 0, nil)

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Active":       active,
		"Inactive":     inactive,
		"DisputeCount": len(totalDisputes),
	})
}

func (h *CompetitionHandler) Detail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	pairIDs := comp.GetStringSlice("pairs")
	seeding := h.getSeeding(comp)

	type pairEntry struct {
		PairID   string
		PairName string
		Seed     int
	}
	var pairEntries []pairEntry
	for _, pid := range pairIDs {
		pair, err := h.app.FindRecordById("pairs", pid)
		if err != nil {
			continue
		}
		pairEntries = append(pairEntries, pairEntry{
			PairID:   pid,
			PairName: pair.GetString("name"),
			Seed:     seeding[pid],
		})
	}

	allPairs, _ := h.app.FindAllRecords("pairs")
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "", 0, 0, map[string]any{"cid": id})

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "round_number", 0, 0,
		map[string]any{"cid": id})

	pairNameMap, _ := expandPairNames(h.app, pairIDs)

	type matchEntry struct {
		Match      *core.Record
		Pair1Name  string
		Pair2Name  string
		RoundNum   int
	}
	roundMap := map[int][]matchEntry{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], matchEntry{
			Match:     m,
			Pair1Name: pairNameMap[m.GetString("pair1")],
			Pair2Name: pairNameMap[m.GetString("pair2")],
			RoundNum:  rn,
		})
	}

	type roundGroup struct {
		Number  int
		Matches []matchEntry
	}
	var rounds []roundGroup
	for rn, ms := range roundMap {
		rounds = append(rounds, roundGroup{Number: rn, Matches: ms})
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i].Number < rounds[j].Number
	})

	var disputes []DisputeView
	for _, m := range matches {
		if m.GetString("status") != "disputed" {
			continue
		}
		disputes = append(disputes, DisputeView{
			Match:        m,
			Pair1Name:    pairNameMap[m.GetString("pair1")],
			Pair2Name:    pairNameMap[m.GetString("pair2")],
			SubmittedBy:  resolvePlayerName(h.app, m.GetString("submitted_by")),
			DisputedBy:   resolvePlayerName(h.app, m.GetString("disputed_by")),
			DisputeNotes: m.GetString("dispute_notes"),
		})
	}

	var standings []StandingRowFull
	if comp.GetString("type") == "league" {
		standings, _ = ComputeStandings(h.app, id)
	}

	allUsers, _ := h.app.FindRecordsByFilter("users", "role = 'player'", "display_name", 0, 0, nil)

	return h.renderPage(e, "admin/competition-detail.html", map[string]any{
		"Competition":     comp,
		"Entries":         pairEntries,
		"AllPairs":        allPairs,
		"AllCompetitions": allComps,
		"AllUsers":        allUsers,
		"Rounds":          rounds,
		"Disputes":        disputes,
		"Standings":       standings,
		"IsLeague":        comp.GetString("type") == "league",
		"HasFixtures":     len(matches) > 0,
	})
}

func (h *CompetitionHandler) List(e *core.RequestEvent) error {
	return h.Dashboard(e)
}

func (h *CompetitionHandler) Create(e *core.RequestEvent) error {
	col, err := h.app.FindCollectionByNameOrId("competitions")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	record := core.NewRecord(col)
	record.Set("name", e.Request.FormValue("name"))
	record.Set("type", e.Request.FormValue("type"))
	record.Set("category", e.Request.FormValue("category"))
	record.Set("active", e.Request.FormValue("active") == "on")
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions")
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) Update(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("type", e.Request.FormValue("type"))
	record.Set("category", e.Request.FormValue("category"))
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions")
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) Toggle(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	record.Set("active", !record.GetBool("active"))
	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions")
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) ListPairs(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	pairIDs := comp.GetStringSlice("pairs")
	seeding := h.getSeeding(comp)

	type pairEntry struct {
		PairID   string
		PairName string
		Seed     int
	}
	var entries []pairEntry
	for _, pid := range pairIDs {
		pair, err := h.app.FindRecordById("pairs", pid)
		if err != nil {
			continue
		}
		entries = append(entries, pairEntry{
			PairID:   pid,
			PairName: pair.GetString("name"),
			Seed:     seeding[pid],
		})
	}

	allPairs, _ := h.app.FindAllRecords("pairs")
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "", 0, 0, map[string]any{"cid": id})

	return h.renderPage(e, "admin/competition-pairs.html", map[string]any{
		"Competition":     comp,
		"Entries":         entries,
		"AllPairs":        allPairs,
		"AllCompetitions": allComps,
	})
}

func (h *CompetitionHandler) AddPair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair")
	seedStr := e.Request.FormValue("seed")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Pareja no encontrada</div>`)
	}

	existingPairIDs := comp.GetStringSlice("pairs")
	if err := h.validatePlayerUniqueness(existingPairIDs, pair, ""); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">%s</div>`, err.Error()))
	}

	for _, pid := range existingPairIDs {
		if pid == pairID {
			return e.HTML(http.StatusOK, `<div class="alert alert-error">Esta pareja ya está en la competición</div>`)
		}
	}

	comp.Set("pairs", append(existingPairIDs, pairID))

	if seedStr != "" {
		seed, _ := strconv.Atoi(seedStr)
		if seed > 0 {
			seeding := h.getSeeding(comp)
			seeding[pairID] = seed
			comp.Set("seeding", seeding)
		}
	}

	if err := h.app.Save(comp); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID)
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) RemovePair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición no encontrada</div>`)
	}

	existingPairIDs := comp.GetStringSlice("pairs")
	var updated []string
	for _, pid := range existingPairIDs {
		if pid != pairID {
			updated = append(updated, pid)
		}
	}
	comp.Set("pairs", updated)

	seeding := h.getSeeding(comp)
	delete(seeding, pairID)
	comp.Set("seeding", seeding)

	if err := h.app.Save(comp); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID)
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) CopyPairs(e *core.RequestEvent) error {
	targetID := e.Request.PathValue("id")
	sourceID := e.Request.FormValue("source_competition")

	if sourceID == "" {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Selecciona una competición de origen</div>`)
	}

	source, err := h.app.FindRecordById("competitions", sourceID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición origen no encontrada</div>`)
	}

	target, err := h.app.FindRecordById("competitions", targetID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición destino no encontrada</div>`)
	}

	sourcePairIDs := source.GetStringSlice("pairs")
	sourceSeeding := h.getSeeding(source)
	existingPairIDs := target.GetStringSlice("pairs")
	targetSeeding := h.getSeeding(target)

	existingSet := make(map[string]bool, len(existingPairIDs))
	for _, pid := range existingPairIDs {
		existingSet[pid] = true
	}

	copied := 0
	skipped := 0
	for _, pairID := range sourcePairIDs {
		if existingSet[pairID] {
			skipped++
			continue
		}

		pair, err := h.app.FindRecordById("pairs", pairID)
		if err != nil {
			skipped++
			continue
		}
		if err := h.validatePlayerUniqueness(existingPairIDs, pair, ""); err != nil {
			skipped++
			continue
		}

		existingPairIDs = append(existingPairIDs, pairID)
		existingSet[pairID] = true
		if target.GetString("type") == "playoff" {
			if s, ok := sourceSeeding[pairID]; ok {
				targetSeeding[pairID] = s
			}
		}
		copied++
	}

	target.Set("pairs", existingPairIDs)
	target.Set("seeding", targetSeeding)

	if err := h.app.Save(target); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	return e.HTML(http.StatusOK, fmt.Sprintf(
		`<div class="alert alert-success">%d parejas copiadas, %d omitidas</div>`, copied, skipped))
}

func (h *CompetitionHandler) getSeeding(comp *core.Record) map[string]int {
	seeding := make(map[string]int)
	raw := comp.Get("seeding")
	if raw == nil {
		return seeding
	}
	switch v := raw.(type) {
	case string:
		if v != "" {
			json.Unmarshal([]byte(v), &seeding)
		}
	case map[string]any:
		for k, val := range v {
			switch n := val.(type) {
			case float64:
				seeding[k] = int(n)
			case int:
				seeding[k] = n
			}
		}
	}
	return seeding
}

func (h *CompetitionHandler) validatePlayerUniqueness(existingPairIDs []string, pair *core.Record, excludePairID string) error {
	p1 := pair.GetString("player1")
	p2 := pair.GetString("player2")

	for _, pid := range existingPairIDs {
		if excludePairID != "" && pid == excludePairID {
			continue
		}
		otherPair, err := h.app.FindRecordById("pairs", pid)
		if err != nil {
			continue
		}
		op1 := otherPair.GetString("player1")
		op2 := otherPair.GetString("player2")
		if p1 == op1 || p1 == op2 || p2 == op1 || p2 == op2 {
			return fmt.Errorf("Un jugador ya participa en otra pareja de esta competición")
		}
	}
	return nil
}
