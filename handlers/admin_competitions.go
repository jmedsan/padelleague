package handlers

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// CompetitionHandler handles admin CRUD and management operations for competitions.
type CompetitionHandler struct {
	app        core.App
	leagueSvc  *league.Service
	notifier   *notify.Notifier
	renderPage RenderFunc
}

// NewCompetitionHandler creates a CompetitionHandler with the given dependencies.
func NewCompetitionHandler(app core.App, leagueSvc *league.Service, notifier *notify.Notifier, renderPage RenderFunc) *CompetitionHandler {
	return &CompetitionHandler{app: app, leagueSvc: leagueSvc, notifier: notifier, renderPage: renderPage}
}

// Detail renders the admin detail page for a single competition.
func (h *CompetitionHandler) Detail(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pairIDs := comp.GetStringSlice("pairs")
	seeding := getSeeding(comp)
	paymentStatus := getPaymentStatus(comp)
	penaltyRows := h.getPenaltyRows(id)

	pairEntries := buildPairEntries(h.app, pairIDs, seeding, paymentStatus)
	allPairs := availablePairs(h.app, pairIDs)
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != {:cid}", "name", 0, 0, map[string]any{"cid": id})

	matches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "round_number,created", 0, 0,
		map[string]any{"cid": id})

	pairNameMap := league.PairNames(h.app, pairIDs)

	rounds := h.buildRoundGroups(matches, pairNameMap)
	disputes := h.buildDisputeCards(matches)
	allUsers, _ := h.app.FindRecordsByFilter("users", "roles ~ 'player'", "display_name", 0, 0, nil)
	isLeague := comp.GetString("type") == "league"

	data := map[string]any{
		"Competition":     comp,
		"Entries":         pairEntries,
		"AllPairs":        allPairs,
		"AllCompetitions": allComps,
		"AllUsers":        allUsers,
		"Rounds":          rounds,
		"Disputes":        disputes,
		"PenaltyRows":     penaltyRows,
		"IsLeague":        isLeague,
		"HasFixtures":     len(matches) > 0,
		"HasUnpaid":       anyUnpaid(pairEntries),
		"UnpaidCount":     countUnpaid(pairEntries),
		"Phase":           league.PhaseOf(comp, time.Now()),
		"Categories":      league.Categories(),
	}
	h.addDetailExtras(data, comp, matches)
	return h.renderPage(e, "admin/competition-detail.html", data)
}

func (h *CompetitionHandler) addDetailExtras(data map[string]any, comp *core.Record, matches []*core.Record) {
	if comp.GetString("type") == "league" {
		standings, _ := h.leagueSvc.ComputeStandings(comp.Id)
		data["Standings"] = standings
		for _, s := range standings {
			if s.Penalty > 0 {
				data["HasPenalties"] = true
				break
			}
		}
		if len(matches) > 0 {
			data["RoundDates"] = h.buildRoundDates(comp)
		}
	}
	attachedViews, unattachedDocs := h.buildDetailDocs(comp)
	data["AttachedDocViews"] = attachedViews
	data["UnattachedDocs"] = unattachedDocs
}

func (h *CompetitionHandler) buildDetailDocs(comp *core.Record) ([]DocumentView, []*core.Record) {
	attachedIDs := comp.GetStringSlice("documents")
	attachMode := Mode{Admin: true, Full: false, Editable: true}
	var views []DocumentView
	for _, did := range attachedIDs {
		if doc, err := h.app.FindRecordById("documents", did); err == nil {
			dv := NewDocumentView(doc, attachMode)
			dv.CompetitionID = comp.Id
			views = append(views, dv)
		}
	}
	attachedSet := make(map[string]struct{}, len(attachedIDs))
	for _, did := range attachedIDs {
		attachedSet[did] = struct{}{}
	}
	allDocs, _ := h.app.FindRecordsByFilter("documents", "", "title", 0, 0, nil)
	var unattached []*core.Record
	for _, d := range allDocs {
		if _, ok := attachedSet[d.Id]; !ok {
			unattached = append(unattached, d)
		}
	}
	return views, unattached
}

func anyUnpaid(entries []pairEntry) bool {
	for _, pe := range entries {
		if !pe.Paid {
			return true
		}
	}
	return false
}

func countUnpaid(entries []pairEntry) int {
	n := 0
	for _, pe := range entries {
		if !pe.Paid {
			n++
		}
	}
	return n
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

	setSchedulingFields(record, e)

	if err := h.app.Save(record); err != nil {
		slog.Error("create competition failed", "err", err)
		return alertError(e, "Error al crear la competición")
	}

	defaults, _ := h.app.FindRecordsByFilter("documents", "is_default = true", "", 0, 0, nil)
	if len(defaults) > 0 {
		ids := make([]string, len(defaults))
		for i, d := range defaults {
			ids[i] = d.Id
		}
		record.Set("documents", ids)
		if err := h.app.Save(record); err != nil {
			slog.Error("preload default documents", "comp", record.Id, "err", err)
		}
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

	oldStart := record.GetString("start_date")
	oldEnd := record.GetString("end_date")

	record.Set("name", e.Request.FormValue("name"))
	record.Set("type", e.Request.FormValue("type"))
	record.Set("category", e.Request.FormValue("category"))
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")

	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, _ := strconv.Atoi(v)
		record.Set("quorum_timeout_hours", hours)
	}

	setSchedulingFields(record, e)

	if err := h.app.Save(record); err != nil {
		slog.Error("update competition failed", "err", err)
		return alertError(e, "Error al guardar la competición")
	}

	if record.GetString("start_date") != oldStart || record.GetString("end_date") != oldEnd {
		resetWarnLevels(h.app, id)
		h.refreshRoundSchedule(record)
	}

	return redirectHX(e, "/admin/competitions")
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

// FinalizeCompetition ends a competition's recovery window immediately.
func (h *CompetitionHandler) FinalizeCompetition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	record.Set("finalized", true)
	if err := h.app.Save(record); err != nil {
		slog.Error("finalize competition failed", "err", err)
		return alertError(e, "Error al finalizar la competición")
	}

	return redirectHX(e, "/admin/competitions/"+id)
}

// ApplyPenalty creates a new penalty row or voids an existing one.
func (h *CompetitionHandler) ApplyPenalty(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	action := e.Request.FormValue("action")

	if action == "remove" {
		penaltyID := e.Request.FormValue("penalty_id")
		if err := league.VoidPenalty(h.app, penaltyID); err != nil {
			return alertError(e, "Error al quitar la penalización")
		}
		return redirectHX(e, "/admin/competitions/"+id)
	}

	pairID := e.Request.FormValue("pair_id")
	if pairID == "" {
		return alertError(e, "Debes seleccionar una pareja")
	}
	reason := strings.TrimSpace(e.Request.FormValue("reason"))
	amountStr := e.Request.FormValue("amount")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		return alertError(e, "El importe debe ser mayor que cero")
	}
	if reason == "" {
		return alertError(e, "El motivo es obligatorio")
	}

	if err := league.ApplyPenalty(h.app, league.PenaltyInput{CompetitionID: id, PairID: pairID, Reason: reason, AdminID: e.Auth.Id, Amount: amount}); err != nil {
		return alertError(e, "Error al guardar la penalización")
	}
	return redirectHX(e, "/admin/competitions/"+id)
}

type roundDate struct {
	Number int
	Date   string // "YYYY-MM-DD" for the input value
}

type roundGroup struct {
	Number  int
	Matches []MatchCard
}

func (h *CompetitionHandler) buildRoundDates(comp *core.Record) []roundDate {
	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		return nil
	}
	var dates []roundDate
	for r := 1; r <= rounds; r++ {
		d := roundDate{Number: r}
		if t, ok := league.RoundArrangeDate(comp, r); ok {
			d.Date = t.Format("2006-01-02")
		}
		dates = append(dates, d)
	}
	return dates
}

func (h *CompetitionHandler) buildRoundGroups(matches []*core.Record, pairNames map[string]string) []roundGroup {
	noPairs := map[string]struct{}{}
	roundMap := map[int][]MatchCard{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], NewMatchRow(m, pairNames, noPairs))
	}
	var rounds []roundGroup
	for rn, ms := range roundMap {
		rounds = append(rounds, roundGroup{Number: rn, Matches: ms})
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i].Number < rounds[j].Number
	})
	for ri := 1; ri < len(rounds); ri++ {
		prevRound := rounds[ri-1].Number
		for mi := range rounds[ri].Matches {
			rounds[ri].Matches[mi].PopulateFeeder(prevRound, mi)
		}
	}
	return rounds
}

func (h *CompetitionHandler) buildDisputeCards(matches []*core.Record) []MatchCard {
	var cards []MatchCard
	for _, m := range matches {
		if m.GetString("status") == league.StatusDisputed {
			cards = append(cards, NewMatchCard(h.app, m, AdminFull, ""))
		}
	}
	return cards
}

// PenaltyRow is one penalty entry for the admin UI.
type PenaltyRow struct {
	ID        string
	Amount    float64
	Reason    string
	AdminName string
	Date      string
}

func (h *CompetitionHandler) getPenaltyRows(compID string) map[string][]PenaltyRow {
	rows, err := h.app.FindRecordsByFilter("penalties",
		"competition = {:c} && voided = false", "-created", 0, 0,
		map[string]any{"c": compID})
	if err != nil {
		return map[string][]PenaltyRow{}
	}
	out := make(map[string][]PenaltyRow, len(rows))
	for _, r := range rows {
		adminName := "Sistema"
		if aid := r.GetString("applied_by"); aid != "" {
			adminName = league.PlayerName(h.app, aid)
		}
		out[r.GetString("pair")] = append(out[r.GetString("pair")], PenaltyRow{
			ID:        r.Id,
			Amount:    r.GetFloat("amount"),
			Reason:    r.GetString("reason"),
			AdminName: adminName,
			Date:      r.GetDateTime("created").Time().Format("02/01/2006"),
		})
	}
	return out
}

func getSeeding(comp *core.Record) map[string]int {
	seeding := make(map[string]int)
	if err := comp.UnmarshalJSONField("seeding", &seeding); err != nil {
		slog.Warn("unmarshal seeding", "err", err)
	}
	return seeding
}

func resetWarnLevels(app core.App, compID string) {
	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp}", "", 0, 0, map[string]any{"comp": compID})
	if err != nil {
		slog.Error("reset warn levels: list matches", "comp", compID, "err", err)
		return
	}
	for _, m := range matches {
		if m.GetInt("last_warn_level") == 0 {
			continue
		}
		m.Set("last_warn_level", 0)
		if err := app.Save(m); err != nil {
			slog.Error("reset warn level", "match", m.Id, "err", err)
		}
	}
}

func (h *CompetitionHandler) refreshRoundSchedule(comp *core.Record) {
	if league.IsPlayoff(comp) {
		return
	}
	if comp.GetString("round_arrange_dates") != "" {
		return
	}
	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		matches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": comp.Id})
		for _, m := range matches {
			if rn := m.GetInt("round_number"); rn > rounds {
				rounds = rn
			}
		}
		if rounds > 0 {
			comp.Set("rounds", rounds)
		}
	}
	if rounds == 0 {
		return
	}
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := h.app.Save(comp); err != nil {
		slog.Error("refresh round schedule failed", "competition", comp.Id, "err", err)
	}
}

// UpdateRoundDates saves admin-edited per-round arrange-by dates.
func (h *CompetitionHandler) UpdateRoundDates(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	rounds := comp.GetInt("rounds")
	if rounds == 0 {
		return alertError(e, "No hay jornadas generadas")
	}

	schedule := make(map[int]time.Time, rounds)
	for r := 1; r <= rounds; r++ {
		v := e.Request.FormValue("round_date_" + strconv.Itoa(r))
		if v == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return alertError(e, "Fecha inválida en jornada "+strconv.Itoa(r))
		}
		schedule[r] = t
	}

	b, _ := json.Marshal(schedule)
	comp.Set("round_arrange_dates", string(b))
	if err := h.app.Save(comp); err != nil {
		slog.Error("update round dates failed", "competition", id, "err", err)
		return alertError(e, "Error al guardar las fechas")
	}

	resetWarnLevels(h.app, id)
	return redirectHX(e, "/admin/competitions/"+id)
}

// RegenerateRoundDates overwrites stored dates from the current start/end/rounds.
func (h *CompetitionHandler) RegenerateRoundDates(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	rounds := comp.GetInt("rounds")
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := h.app.Save(comp); err != nil {
		slog.Error("regenerate round dates failed", "competition", id, "err", err)
		return alertError(e, "Error al regenerar las fechas")
	}

	resetWarnLevels(h.app, id)
	return redirectHX(e, "/admin/competitions/"+id)
}

func setSchedulingFields(record *core.Record, e *core.RequestEvent) {
	if v := e.Request.FormValue("start_date"); v != "" {
		record.Set("start_date", v)
	}
	if v := e.Request.FormValue("end_date"); v != "" {
		record.Set("end_date", v)
	}

	grace := 3
	if v := e.Request.FormValue("arrange_grace_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			grace = n
		}
	}
	record.Set("arrange_grace_days", grace)

	record.Set("auto_flag", e.Request.FormValue("auto_flag") == "on")

	ws := e.Request.FormValue("walkover_score")
	if ws == "" {
		ws = "6-0 6-0"
	}
	record.Set("walkover_score", ws)

	dp := 3
	if v := e.Request.FormValue("default_penalty"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			dp = n
		}
	}
	record.Set("default_penalty", dp)

	rd := 14
	if v := e.Request.FormValue("recovery_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			rd = n
		}
	}
	record.Set("recovery_days", rd)
}

// AttachDocument adds a document to a competition's attached documents.
func (h *CompetitionHandler) AttachDocument(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	docID := e.Request.FormValue("document")
	comp.Set("documents", league.AppendUnique(comp.GetStringSlice("documents"), docID))
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al adjuntar el documento")
	}
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}

// DetachDocument removes a document from a competition without deleting it.
func (h *CompetitionHandler) DetachDocument(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	docID := e.Request.PathValue("docId")
	comp.Set("documents", league.RemoveString(comp.GetStringSlice("documents"), docID))
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al quitar el documento")
	}
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}

// AdminBroadcast sends an announcement to all players of a competition.
func (h *CompetitionHandler) AdminBroadcast(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	title := strings.TrimSpace(e.Request.FormValue("title"))
	body := strings.TrimSpace(e.Request.FormValue("body"))
	if title == "" || body == "" {
		return alertError(e, "El título y el mensaje son obligatorios")
	}

	seen := make(map[string]struct{})
	var players []string
	for _, pid := range comp.GetStringSlice("pairs") {
		for _, uid := range league.PlayersForPair(h.app, pid) {
			if _, ok := seen[uid]; !ok {
				seen[uid] = struct{}{}
				players = append(players, uid)
			}
		}
	}

	h.notifier.NotifyPlayers(players, league.Notification{
		Type:  "general",
		Title: title,
		Body:  body,
	})
	h.notifier.EmailPlayers(players, title, body, "/competition/"+comp.Id)

	slog.Info("broadcast sent", "competition", comp.Id, "players", len(players))
	return alertSuccess(e, "Anuncio enviado a "+strconv.Itoa(len(players))+" jugadores")
}
