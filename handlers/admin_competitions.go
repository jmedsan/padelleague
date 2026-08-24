package handlers

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// AdminIssue represents a problem detected in competition state for the admin dashboard.
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

// CompetitionHandler handles admin CRUD and management operations for competitions.
type CompetitionHandler struct {
	app        core.App
	leagueSvc  *league.Service
	renderPage func(e *core.RequestEvent, page string, data map[string]any) error
}

// NewCompetitionHandler creates a CompetitionHandler with the given dependencies.
func NewCompetitionHandler(app core.App, leagueSvc *league.Service, renderPage func(e *core.RequestEvent, page string, data map[string]any) error) *CompetitionHandler {
	return &CompetitionHandler{app: app, leagueSvc: leagueSvc, renderPage: renderPage}
}

// CompetitionSummary holds aggregate stats for a competition on the dashboard.
type CompetitionSummary struct {
	Competition   *core.Record
	PairsCount    int
	TotalMatches  int
	PlayedMatches int
	DisputeCount  int
}

// Dashboard renders the admin competitions overview with active/inactive lists and issues.
func (h *CompetitionHandler) Dashboard(e *core.RequestEvent) error {
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0, nil)

	var active, inactive []CompetitionSummary
	for _, comp := range allComps {
		summary := h.buildCompetitionSummary(comp)
		if comp.GetBool("active") {
			active = append(active, summary)
		} else {
			inactive = append(inactive, summary)
		}
	}

	totalDisputes, _ := h.app.FindRecordsByFilter("matches",
		"status = 'disputed'", "", 0, 0, nil)

	issues := h.buildAdminIssues(active)

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Active":       active,
		"Inactive":     inactive,
		"DisputeCount": len(totalDisputes),
		"Issues":       issues,
		"IssueCount":   len(issues),
	})
}

func (h *CompetitionHandler) buildCompetitionSummary(comp *core.Record) CompetitionSummary {
	allMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": comp.Id})
	playedMatches := 0
	disputeCount := 0
	for _, m := range allMatches {
		if m.GetString("status") == league.StatusFinal {
			playedMatches++
		}
		if m.GetString("status") == league.StatusDisputed {
			disputeCount++
		}
	}
	return CompetitionSummary{
		Competition:   comp,
		PairsCount:    len(comp.GetStringSlice("pairs")),
		TotalMatches:  len(allMatches),
		PlayedMatches: playedMatches,
		DisputeCount:  disputeCount,
	}
}

func (h *CompetitionHandler) buildAdminIssues(active []CompetitionSummary) []AdminIssue {
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
		pairNames := league.PairNames(h.app, pairIDs)

		for _, m := range matches {
			issues = append(issues, h.classifyMatchIssues(m, compName, quorumHours, pairNames, now)...)
		}
	}
	return issues
}

func (h *CompetitionHandler) classifyMatchIssues(m *core.Record, compName string, quorumHours float64, pairNames map[string]string, now time.Time) []AdminIssue {
	status := m.GetString("status")
	base := AdminIssue{
		CompetitionName: compName,
		Pair1Name:       pairNames[m.GetString("pair1")],
		Pair2Name:       pairNames[m.GetString("pair2")],
		MatchID:         m.Id,
	}

	switch status {
	case league.StatusDisputed:
		issue := base
		issue.Type = "dispute"
		issue.TypeLabel = "Disputa"
		issue.BadgeClass = "badge-error"
		issue.Detail = "pendiente de resolución"
		return []AdminIssue{issue}
	case league.StatusConfirmed:
		return h.checkQuorumIssue(m, base, quorumHours, now)
	case league.StatusPending:
		return h.checkPendingIssues(m, base, now)
	}
	return nil
}

func (h *CompetitionHandler) checkQuorumIssue(m *core.Record, base AdminIssue, quorumHours float64, now time.Time) []AdminIssue {
	if quorumHours <= 0 {
		return nil
	}
	sa := m.GetString("submitted_at")
	if sa == "" {
		return nil
	}
	dt, err := types.ParseDateTime(sa)
	if err != nil {
		return nil
	}
	elapsed := now.Sub(dt.Time())
	if elapsed <= time.Duration(quorumHours)*time.Hour {
		return nil
	}
	days := int(elapsed.Hours() / 24)
	detail := fmt.Sprintf("enviado hace %d dias", days)
	if days == 0 {
		detail = fmt.Sprintf("enviado hace %d horas", int(elapsed.Hours()))
	}
	issue := base
	issue.Type = "quorum"
	issue.TypeLabel = "Quorum"
	issue.BadgeClass = "badge-warning"
	issue.Detail = detail
	return []AdminIssue{issue}
}

func (h *CompetitionHandler) checkPendingIssues(m *core.Record, base AdminIssue, now time.Time) []AdminIssue {
	var issues []AdminIssue
	if d := m.GetString("date"); d != "" {
		if dt, err := types.ParseDateTime(d); err == nil && dt.Time().Before(now) {
			issue := base
			issue.Type = "overdue"
			issue.TypeLabel = "Vencido"
			issue.BadgeClass = "badge-ghost"
			issue.Detail = "fecha: " + d
			issues = append(issues, issue)
		}
	}
	lastMsg, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "-created", 1, 0,
		map[string]any{"mid": m.Id})
	if len(lastMsg) == 0 {
		return issues
	}
	created := lastMsg[0].GetString("created")
	t, err := time.Parse("2006-01-02 15:04:05.000Z", created)
	if err != nil {
		return issues
	}
	if now.Sub(t) > 14*24*time.Hour {
		days := int(now.Sub(t).Hours() / 24)
		issue := base
		issue.Type = "stale"
		issue.TypeLabel = "Inactivo"
		issue.BadgeClass = "badge-info"
		issue.Detail = fmt.Sprintf("sin actividad en %d dias", days)
		issues = append(issues, issue)
	}
	return issues
}

// Detail renders the admin detail page for a single competition.
func (h *CompetitionHandler) Detail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pairIDs := comp.GetStringSlice("pairs")
	seeding := h.getSeeding(comp)
	paymentStatus := h.getPaymentStatus(comp)
	penaltyMap := h.getPenaltyMap(comp)

	pairEntries := h.buildPairEntries(pairIDs, seeding, paymentStatus)
	allPairs := h.availablePairs(pairIDs)
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "", 0, 0, map[string]any{"cid": id})

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": id})

	pairNameMap := league.PairNames(h.app, pairIDs)

	rounds := h.buildRoundGroups(matches, pairNameMap)
	disputes := h.buildDisputeViews(matches, pairNameMap)

	var standings []league.StandingRowFull
	if comp.GetString("type") == "league" {
		standings, _ = h.leagueSvc.ComputeStandings(id)
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

type pairEntry struct {
	PairID   string
	PairName string
	Seed     int
	Paid     bool
}

func (h *CompetitionHandler) buildPairEntries(pairIDs []string, seeding map[string]int, paymentStatus map[string]bool) []pairEntry {
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
			Paid:     paymentStatus[pid],
		})
	}
	return entries
}

func (h *CompetitionHandler) availablePairs(enrolledIDs []string) []*core.Record {
	allPairsRaw, _ := h.app.FindAllRecords("pairs")
	enrolled := map[string]bool{}
	for _, pid := range enrolledIDs {
		enrolled[pid] = true
	}
	var available []*core.Record
	for _, p := range allPairsRaw {
		if !enrolled[p.Id] {
			available = append(available, p)
		}
	}
	return available
}

type matchEntry struct {
	Match     *core.Record
	Pair1Name string
	Pair2Name string
	RoundNum  int
}

type roundGroup struct {
	Number  int
	Matches []matchEntry
}

func (h *CompetitionHandler) buildRoundGroups(matches []*core.Record, pairNames map[string]string) []roundGroup {
	roundMap := map[int][]matchEntry{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], matchEntry{
			Match:     m,
			Pair1Name: pairNames[m.GetString("pair1")],
			Pair2Name: pairNames[m.GetString("pair2")],
			RoundNum:  rn,
		})
	}
	var rounds []roundGroup
	for rn, ms := range roundMap {
		rounds = append(rounds, roundGroup{Number: rn, Matches: ms})
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i].Number < rounds[j].Number
	})
	return rounds
}

func (h *CompetitionHandler) buildDisputeViews(matches []*core.Record, pairNames map[string]string) []DisputeView {
	var disputes []DisputeView
	for _, m := range matches {
		if m.GetString("status") != league.StatusDisputed {
			continue
		}
		disputes = append(disputes, DisputeView{
			Match:        m,
			Pair1Name:    pairNames[m.GetString("pair1")],
			Pair2Name:    pairNames[m.GetString("pair2")],
			SubmittedBy:  league.PlayerName(h.app, m.GetString("submitted_by")),
			DisputedBy:   league.PlayerName(h.app, m.GetString("disputed_by")),
			DisputeNotes: m.GetString("dispute_notes"),
		})
	}
	return disputes
}

// Create handles POST to create a new competition.
func (h *CompetitionHandler) Create(e *core.RequestEvent) error {
	col, err := h.app.FindCollectionByNameOrId("competitions")
	if err != nil {
		return alertError(e, "Error interno")
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
		slog.Error("create competition failed", "err", err)
		return alertError(e, "Error al crear la competición")
	}

	return redirectHX(e, "/admin/competitions")
}

// Update handles POST to modify an existing competition's settings.
func (h *CompetitionHandler) Update(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
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
		slog.Error("update competition failed", "err", err)
		return alertError(e, "Error al guardar la competición")
	}

	return redirectHX(e, "/admin/competitions")
}

// ApplyPenalty adds or removes a point penalty for a pair in a competition.
func (h *CompetitionHandler) ApplyPenalty(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
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
		return alertError(e, "Error al guardar")
	}

	return redirectHX(e, "/admin/competitions/"+id)
}

func (h *CompetitionHandler) getPenaltyMap(comp *core.Record) map[string]float64 {
	penalties := make(map[string]float64)
	if err := comp.UnmarshalJSONField("penalty_points", &penalties); err != nil {
		slog.Warn("unmarshal penalty_points", "err", err)
	}
	return penalties
}

// Toggle switches a competition between active and inactive states.
func (h *CompetitionHandler) Toggle(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	record.Set("active", !record.GetBool("active"))
	if err := h.app.Save(record); err != nil {
		slog.Error("toggle competition active failed", "err", err)
		return alertError(e, "Error al cambiar el estado")
	}

	return redirectHX(e, "/admin/competitions")
}

// AddPair enrolls a pair in a competition, validating player uniqueness.
func (h *CompetitionHandler) AddPair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair")
	seedStr := e.Request.FormValue("seed")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return alertError(e, "Pareja no encontrada")
	}

	existingPairIDs := comp.GetStringSlice("pairs")
	if err := h.validatePlayerUniqueness(existingPairIDs, pair, ""); err != nil {
		slog.Error("player uniqueness validation failed", "pair", pairID, "err", err)
		return alertError(e, "Esta pareja tiene jugadores duplicados en la competición")
	}

	for _, pid := range existingPairIDs {
		if pid == pairID {
			return alertError(e, "Esta pareja ya está en la competición")
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
		slog.Error("add pair failed", "competition", compID, "err", err)
		return alertError(e, "Error al añadir la pareja")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

// RemovePair removes a pair from a competition and deletes its pending matches.
func (h *CompetitionHandler) RemovePair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
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
		slog.Error("remove pair failed", "competition", compID, "err", err)
		return alertError(e, "Error al eliminar la pareja")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

// CopyPairs imports pairs from a source competition into the target.
func (h *CompetitionHandler) CopyPairs(e *core.RequestEvent) error {
	targetID := e.Request.PathValue("id")
	sourceID := e.Request.FormValue("source_competition")

	if sourceID == "" {
		return alertError(e, "Selecciona una competición de origen")
	}

	source, err := h.app.FindRecordById("competitions", sourceID)
	if err != nil {
		return alertError(e, "Competición origen no encontrada")
	}

	target, err := h.app.FindRecordById("competitions", targetID)
	if err != nil {
		return alertError(e, "Competición destino no encontrada")
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
	isPlayoff := target.GetString("type") == "playoff"
	for _, pairID := range sourcePairIDs {
		if !h.canCopyPair(pairID, existingSet, existingPairIDs) {
			skipped++
			continue
		}
		existingPairIDs = append(existingPairIDs, pairID)
		existingSet[pairID] = true
		if isPlayoff {
			if s, ok := sourceSeeding[pairID]; ok {
				targetSeeding[pairID] = s
			}
		}
		copied++
	}

	target.Set("pairs", existingPairIDs)
	target.Set("seeding", targetSeeding)

	if err := h.app.Save(target); err != nil {
		slog.Error("copy pairs failed", "err", err)
		return alertError(e, "Error al copiar parejas")
	}

	return alertSuccess(e, fmt.Sprintf("%d parejas copiadas, %d omitidas", copied, skipped))
}

func (h *CompetitionHandler) canCopyPair(pairID string, existingSet map[string]bool, existingPairIDs []string) bool {
	if existingSet[pairID] {
		return false
	}
	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return false
	}
	return h.validatePlayerUniqueness(existingPairIDs, pair, "") == nil
}

// TogglePayment marks a single pair's payment status as paid or unpaid.
func (h *CompetitionHandler) TogglePayment(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	paymentStatus := h.getPaymentStatus(comp)
	paymentStatus[pairID] = !paymentStatus[pairID]
	comp.Set("payment_status", paymentStatus)

	if err := h.app.Save(comp); err != nil {
		slog.Error("toggle payment failed", "err", err)
		return alertError(e, "Error al cambiar el estado de pago")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

// TogglePaymentAll sets all pairs in a competition to paid or unpaid.
func (h *CompetitionHandler) TogglePaymentAll(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pairIDs := comp.GetStringSlice("pairs")
	status := map[string]bool{}
	for _, pid := range pairIDs {
		status[pid] = true
	}

	comp.Set("payment_status", status)
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al guardar")
	}

	return redirectHX(e, "/admin/competitions/"+id)
}

func (h *CompetitionHandler) getPaymentStatus(comp *core.Record) map[string]bool {
	status := make(map[string]bool)
	if err := comp.UnmarshalJSONField("payment_status", &status); err != nil {
		slog.Warn("unmarshal payment_status", "err", err)
	}
	return status
}

func (h *CompetitionHandler) getSeeding(comp *core.Record) map[string]int {
	seeding := make(map[string]int)
	if err := comp.UnmarshalJSONField("seeding", &seeding); err != nil {
		slog.Warn("unmarshal seeding", "err", err)
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
			return fmt.Errorf("un jugador ya participa en otra pareja de esta competición")
		}
	}
	return nil
}
