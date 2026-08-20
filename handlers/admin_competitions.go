package handlers

import (
	"fmt"
	"net/http"

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

	cpRecords, _ := h.app.FindRecordsByFilter("competition_pairs",
		"competition = {:cid}", "seed", 0, 0,
		map[string]any{"cid": id})

	type pairEntry struct {
		CompPairID string
		PairName   string
		Seed       int
	}
	var entries []pairEntry
	for _, cp := range cpRecords {
		pair, err := h.app.FindRecordById("pairs", cp.GetString("pair"))
		if err != nil {
			continue
		}
		entries = append(entries, pairEntry{
			CompPairID: cp.Id,
			PairName:   pair.GetString("name"),
			Seed:       int(cp.GetFloat("seed")),
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

	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Pareja no encontrada</div>`)
	}

	if err := h.validatePlayerUniqueness(compID, pair, ""); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">%s</div>`, err.Error()))
	}

	col, err := h.app.FindCollectionByNameOrId("competition_pairs")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	cp := core.NewRecord(col)
	cp.Set("competition", compID)
	cp.Set("pair", pairID)
	if seedStr != "" {
		cp.Set("seed", seedStr)
	}

	if err := h.app.Save(cp); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID+"/pairs")
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) RemovePair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	cpID := e.Request.FormValue("cp_id")

	cp, err := h.app.FindRecordById("competition_pairs", cpID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Registro no encontrado</div>`)
	}

	if err := h.app.Delete(cp); err != nil {
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

	sourcePairs, _ := h.app.FindRecordsByFilter("competition_pairs",
		"competition = {:sid}", "", 0, 0,
		map[string]any{"sid": sourceID})

	target, err := h.app.FindRecordById("competitions", targetID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competición destino no encontrada</div>`)
	}

	col, err := h.app.FindCollectionByNameOrId("competition_pairs")
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error interno</div>`)
	}

	copied := 0
	skipped := 0
	for _, sp := range sourcePairs {
		pairID := sp.GetString("pair")

		existing, _ := h.app.FindRecordsByFilter("competition_pairs",
			"competition = {:cid} && pair = {:pid}", "", 1, 0,
			map[string]any{"cid": targetID, "pid": pairID})
		if len(existing) > 0 {
			skipped++
			continue
		}

		pair, err := h.app.FindRecordById("pairs", pairID)
		if err != nil {
			skipped++
			continue
		}
		if err := h.validatePlayerUniqueness(targetID, pair, ""); err != nil {
			skipped++
			continue
		}

		cp := core.NewRecord(col)
		cp.Set("competition", targetID)
		cp.Set("pair", pairID)
		if target.GetString("type") == "playoff" {
			cp.Set("seed", sp.GetFloat("seed"))
		}
		if err := h.app.Save(cp); err != nil {
			skipped++
			continue
		}
		copied++
	}

	return e.HTML(http.StatusOK, fmt.Sprintf(
		`<div class="alert alert-success">%d parejas copiadas, %d omitidas</div>`, copied, skipped))
}

func (h *CompetitionHandler) validatePlayerUniqueness(compID string, pair *core.Record, excludeCPID string) error {
	p1 := pair.GetString("player1")
	p2 := pair.GetString("player2")

	existingCPs, _ := h.app.FindRecordsByFilter("competition_pairs",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": compID})

	for _, cp := range existingCPs {
		if excludeCPID != "" && cp.Id == excludeCPID {
			continue
		}
		otherPair, err := h.app.FindRecordById("pairs", cp.GetString("pair"))
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
