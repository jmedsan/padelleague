package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type AdminIssue struct {
	Type            string
	TypeLabel       string
	BadgeClass      string
	CompetitionName string
	Pair1Name       string
	Pair2Name       string
	MatchID         string
	Detail          string
}

type CompetitionHandler struct {
	app        core.App
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

func NewCompetitionHandler(app core.App, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *CompetitionHandler {
	return &CompetitionHandler{app: app, renderPage: renderPage}
}

type CompetitionSummary struct {
	Competition   *core.Record
	PairsCount    int
	TotalMatches  int
	PlayedMatches int
	DisputeCount  int
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

	var issues []AdminIssue
	now := time.Now().UTC()
	for _, cs := range active {
		comp := cs.Competition
		compName := comp.GetString("name")
		quorumHours := comp.GetFloat("quorum_timeout_hours")

		matches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": comp.Id})
		pairIDs := make([]string, 0)
		for _, m := range matches {
			pairIDs = append(pairIDs, m.GetString("pair1"), m.GetString("pair2"))
		}
		pairNames, _ := expandPairNames(h.app, pairIDs)

		for _, m := range matches {
			status := m.GetString("status")
			p1 := pairNames[m.GetString("pair1")]
			p2 := pairNames[m.GetString("pair2")]

			switch status {
			case "disputed":
				issues = append(issues, AdminIssue{
					Type: "dispute", TypeLabel: "Disputa", BadgeClass: "badge-error",
					CompetitionName: compName, Pair1Name: p1, Pair2Name: p2,
					MatchID: m.Id, Detail: "pendiente de resolucion",
				})
			case "confirmed":
				if quorumHours > 0 {
					if sa := m.GetString("submitted_at"); sa != "" {
						if t, err := time.Parse(time.RFC3339, sa); err == nil {
							elapsed := now.Sub(t)
							if elapsed > time.Duration(quorumHours)*time.Hour {
								days := int(elapsed.Hours() / 24)
								detail := fmt.Sprintf("enviado hace %d dias", days)
								if days == 0 {
									detail = fmt.Sprintf("enviado hace %d horas", int(elapsed.Hours()))
								}
								issues = append(issues, AdminIssue{
									Type: "quorum", TypeLabel: "Quorum", BadgeClass: "badge-warning",
									CompetitionName: compName, Pair1Name: p1, Pair2Name: p2,
									MatchID: m.Id, Detail: detail,
								})
							}
						}
					}
				}
			case "pending":
				if d := m.GetString("date"); d != "" {
					if matchDate, err := time.Parse("2006-01-02", d); err == nil {
						if matchDate.Before(now) {
							issues = append(issues, AdminIssue{
								Type: "overdue", TypeLabel: "Vencido", BadgeClass: "badge-ghost",
								CompetitionName: compName, Pair1Name: p1, Pair2Name: p2,
								MatchID: m.Id, Detail: "fecha: " + d,
							})
						}
					}
				}
				lastMsg, _ := h.app.FindRecordsByFilter("match_messages",
					"match = {:mid}", "-created", 1, 0,
					map[string]any{"mid": m.Id})
				if len(lastMsg) > 0 {
					created := lastMsg[0].GetString("created")
					if t, err := time.Parse("2006-01-02 15:04:05.000Z", created); err == nil {
						if now.Sub(t) > 14*24*time.Hour {
							days := int(now.Sub(t).Hours() / 24)
							issues = append(issues, AdminIssue{
								Type: "stale", TypeLabel: "Inactivo", BadgeClass: "badge-info",
								CompetitionName: compName, Pair1Name: p1, Pair2Name: p2,
								MatchID: m.Id, Detail: fmt.Sprintf("sin actividad en %d dias", days),
							})
						}
					}
				}
			}
		}
	}

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Active":       active,
		"Inactive":     inactive,
		"DisputeCount": len(totalDisputes),
		"Issues":       issues,
		"IssueCount":   len(issues),
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
	paymentStatus := h.getPaymentStatus(comp)
	penaltyMap := h.getPenaltyMap(comp)

	type pairEntry struct {
		PairID   string
		PairName string
		Seed     int
		Paid     bool
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
			Paid:     paymentStatus[pid],
		})
	}

	allPairsRaw, _ := h.app.FindAllRecords("pairs")
	enrolledSet := map[string]bool{}
	for _, pid := range pairIDs {
		enrolledSet[pid] = true
	}
	var allPairs []*core.Record
	for _, p := range allPairsRaw {
		if !enrolledSet[p.Id] {
			allPairs = append(allPairs, p)
		}
	}
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "", 0, 0, map[string]any{"cid": id})

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": id})

	pairNameMap, _ := expandPairNames(h.app, pairIDs)

	type matchEntry struct {
		Match     *core.Record
		Pair1Name string
		Pair2Name string
		RoundNum  int
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

	allUsers, _ := h.app.FindRecordsByFilter("users", "role = 'player'", "", 0, 0, nil)

	hasUnpaid := false
	for _, pe := range pairEntries {
		if !pe.Paid {
			hasUnpaid = true
			break
		}
	}

	return h.renderPage(e, "admin/competition-detail.html", map[string]any{
		"Competition":     comp,
		"Entries":         pairEntries,
		"AllPairs":        allPairs,
		"AllCompetitions": allComps,
		"AllUsers":        allUsers,
		"Rounds":          rounds,
		"Disputes":        disputes,
		"Standings":       standings,
		"PenaltyMap":      penaltyMap,
		"IsLeague":        comp.GetString("type") == "league",
		"HasFixtures":     len(matches) > 0,
		"HasUnpaid":       hasUnpaid,
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

	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, _ := strconv.Atoi(v)
		record.Set("quorum_timeout_hours", hours)
	}

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

	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, _ := strconv.Atoi(v)
		record.Set("quorum_timeout_hours", hours)
	}

	if dp := e.Request.FormValue("default_penalty"); dp != "" {
		if v, err := strconv.Atoi(dp); err == nil {
			record.Set("default_penalty", v)
		}
	}

	if err := h.app.Save(record); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions")
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) ApplyPenalty(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competicion no encontrada</div>`)
	}

	pairID := e.Request.FormValue("pair_id")
	action := e.Request.FormValue("action")

	penalties := h.getPenaltyMap(comp)

	if action == "apply" {
		amount := comp.GetFloat("default_penalty")
		if amount == 0 {
			amount = 3
		}
		penalties[pairID] = amount
	} else {
		delete(penalties, pairID)
	}

	comp.Set("penalty_points", penalties)
	if err := h.app.Save(comp); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) getPenaltyMap(comp *core.Record) map[string]float64 {
	penalties := make(map[string]float64)
	raw := comp.Get("penalty_points")
	if raw == nil {
		return penalties
	}
	switch v := raw.(type) {
	case string:
		if v != "" {
			json.Unmarshal([]byte(v), &penalties)
		}
	case map[string]any:
		for k, val := range v {
			if f, ok := val.(float64); ok {
				penalties[k] = f
			}
		}
	}
	return penalties
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

	paymentStatus := h.getPaymentStatus(comp)
	delete(paymentStatus, pairID)
	comp.Set("payment_status", paymentStatus)

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

func (h *CompetitionHandler) TogglePayment(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competicion no encontrada</div>`)
	}

	paymentStatus := h.getPaymentStatus(comp)
	paymentStatus[pairID] = !paymentStatus[pairID]
	comp.Set("payment_status", paymentStatus)

	if err := h.app.Save(comp); err != nil {
		return e.HTML(http.StatusOK, fmt.Sprintf(`<div class="alert alert-error">Error: %s</div>`, err.Error()))
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+compID)
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) TogglePaymentAll(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Competicion no encontrada</div>`)
	}

	pairIDs := comp.GetStringSlice("pairs")
	status := map[string]bool{}
	for _, pid := range pairIDs {
		status[pid] = true
	}

	comp.Set("payment_status", status)
	if err := h.app.Save(comp); err != nil {
		return e.HTML(http.StatusOK, `<div class="alert alert-error">Error al guardar</div>`)
	}

	e.Response.Header().Set("HX-Redirect", "/admin/competitions/"+id)
	return e.NoContent(http.StatusNoContent)
}

func (h *CompetitionHandler) getPaymentStatus(comp *core.Record) map[string]bool {
	status := make(map[string]bool)
	raw := comp.Get("payment_status")
	if raw == nil {
		return status
	}
	switch v := raw.(type) {
	case string:
		if v != "" {
			json.Unmarshal([]byte(v), &status)
		}
	case map[string]any:
		for k, val := range v {
			if b, ok := val.(bool); ok {
				status[k] = b
			}
		}
	}
	return status
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
