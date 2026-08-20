package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func (h *CompetitionHandler) List(e *core.RequestEvent) error {
	filter := e.Request.URL.Query().Get("filter")
	var competitions []*core.Record
	var err error

	switch filter {
	case "active":
		competitions, err = h.app.FindRecordsByFilter("competitions", "active = true", "-created", 0, 0, nil)
	case "inactive":
		competitions, err = h.app.FindRecordsByFilter("competitions", "active = false", "-created", 0, 0, nil)
	default:
		competitions, err = h.app.FindRecordsByFilter("competitions", "", "-created", 0, 0, nil)
	}
	if err != nil {
		competitions = []*core.Record{}
	}

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Competitions": competitions,
		"Filter":       filter,
	})
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
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "-created", 0, 0, map[string]any{"cid": id})

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

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID+"/pairs")
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

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID+"/pairs")
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
