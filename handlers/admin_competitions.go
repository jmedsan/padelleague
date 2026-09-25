package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/render"
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
	data := h.buildDetailData(e, id, comp)
	data["CompReminderHoursDisplay"] = compReminderHoursDisplay(comp)
	return h.renderPage(e, "admin/competition-detail.html", data)
}

func (h *CompetitionHandler) buildDetailData(e *core.RequestEvent, id string, comp *core.Record) map[string]any {
	pairIDs := comp.GetStringSlice("pairs")
	pairEntries, allPairs := h.loadPairEntries(comp, pairIDs)

	allComps := findRecordsLogged(h.app, "Detail: find other competitions", RecordQuery{
		Collection: "competitions", Filter: "id != {:cid}", Sort: "name", Params: map[string]any{"cid": id},
	})
	matches := findRecordsLogged(h.app, "Detail: find matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid}", Sort: "round_number,created", Params: map[string]any{"cid": id},
	})
	allUsers := findRecordsLogged(h.app, "Detail: find players", RecordQuery{
		Collection: "users", Filter: "roles ~ 'player'", Sort: "display_name",
	})

	pairNameMap := league.PairNames(h.app, pairIDs)
	isLeveled := league.IsLeveled(comp)
	var rounds []roundGroup
	var pairFilter string
	if isLeveled {
		pairFilter = resolveAdminPairFilter(e.Request.URL.Query(), pairIDs)
		rounds = h.buildLeveledRoundGroups(comp, matches, pairNameMap, pairFilter)
	} else {
		rounds = h.buildRoundGroups(comp, matches, pairNameMap)
	}

	penaltyRows := h.getPenaltyRows(id)
	disputes := league.CompHealthItems(h.app, id, time.Now(), "disputes", "walkovers")
	seedRankMap := buildSeedRankMap(comp)

	data := map[string]any{
		"PageTitle":           comp.GetString("name"),
		"Competition":         comp,
		"Entries":             pairEntries,
		"AllPairs":            allPairs,
		"AllCompetitions":     allComps,
		"AllUsers":            allUsers,
		"Rounds":              rounds,
		"AutoExpandRound":     firstIncompleteRoundGroup(rounds),
		"Disputes":            disputes,
		"PenaltyRows":         penaltyRows,
		"ActivePenalty":       firstActivePenalty(penaltyRows),
		"IsLeague":            comp.GetString("type") == "league",
		"IsLeveled":           isLeveled,
		"SeedRankMap":         seedRankMap,
		"HasFixtures":         len(matches) > 0,
		"HasUnpaid":           anyUnpaid(pairEntries),
		"UnpaidCount":         countUnpaid(pairEntries),
		"Phase":               league.PhaseOf(comp, time.Now()),
		"Mode":                AdminFull,
		"FooterCompetitionID": id,
	}
	if isLeveled {
		data["PairOptions"] = buildPairOptions(h.app, pairIDs, map[string]struct{}{}, pairFilter)
	}
	h.addDetailExtras(data, comp, matches, fileTokenFor(e))
	return data
}

// resolveAdminPairFilter reads the "pair" query param for the admin match
// list filter. Empty or "all" (or an id outside the competition) means no
// filter — show every pair's matches.
func resolveAdminPairFilter(query map[string][]string, compPairIDs []string) string {
	vals, ok := query["pair"]
	if !ok || len(vals) == 0 {
		return ""
	}
	pair := vals[0]
	if pair == "all" || !slices.Contains(compPairIDs, pair) {
		return ""
	}
	return pair
}

func (h *CompetitionHandler) loadPairEntries(comp *core.Record, pairIDs []string) ([]pairEntry, []*core.Record) {
	seeding := getSeeding(comp)
	payment := paymentInfo{app: h.app, status: getPaymentStatus(comp), paidAt: getPaymentDates(comp), paidBy: getPaymentActors(comp)}
	withdrawnIDs := comp.GetStringSlice("withdrawn_pairs")
	withdrawnSet := make(map[string]bool, len(withdrawnIDs))
	for _, wid := range withdrawnIDs {
		withdrawnSet[wid] = true
	}
	return buildPairEntries(pairIDs, seeding, payment, withdrawnSet), availablePairs(h.app, pairIDs)
}

func buildSeedRankMap(comp *core.Record) map[string]int {
	seedPairs := comp.GetStringSlice("seed_pairs")
	m := make(map[string]int, len(seedPairs))
	for i, pid := range seedPairs {
		m[pid] = i + 1
	}
	return m
}

func applyCompFormFields(record *core.Record, e *core.RequestEvent, clearReminderIfEmpty bool) error {
	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			return alertError(e, "Tiempo de espera debe ser un número")
		}
		record.Set("quorum_timeout_hours", hours)
	}
	if raw := e.Request.FormValue("match_reminder_hours"); raw != "" {
		rh, err := league.ParseReminderHours(raw)
		if err != nil {
			return alertError(e, "Recordatorios: usa horas separadas por comas (p. ej. 26, 1)")
		}
		record.Set("match_reminder_hours", rh)
	} else if clearReminderIfEmpty {
		record.Set("match_reminder_hours", nil)
	}
	if msg := setSchedulingFields(record, e); msg != "" {
		return alertError(e, msg)
	}
	return nil
}

func applyCompIdentity(record *core.Record, e *core.RequestEvent) string {
	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return "El nombre es obligatorio"
	}
	record.Set("name", name)
	record.Set("type", e.Request.FormValue("type"))
	record.Set("active", e.Request.FormValue("active") == "on")
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")
	gt := e.Request.FormValue("gender_type")
	if gt == "" {
		gt = "free"
	}
	record.Set("gender_type", gt)
	return ""
}

// Create handles POST to create a new competition.
func (h *CompetitionHandler) Create(e *core.RequestEvent) error {
	col, err := h.app.FindCollectionByNameOrId("competitions")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	if msg := applyCompIdentity(record, e); msg != "" {
		return alertError(e, msg)
	}
	if err := applyCompFormFields(record, e, false); err != nil {
		return err
	}
	if msg := validateLeveledFields(record, e, 0, false); msg != "" {
		return alertError(e, msg)
	}

	if err := h.app.Save(record); err != nil {
		slog.Error("create competition failed", "err", err)
		return alertError(e, "Error al crear la competición")
	}

	defaults := findRecordsLogged(h.app, "Create: find default documents", RecordQuery{
		Collection: "documents", Filter: "is_default = true",
	})
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

	flash(e, "Competición creada")
	return redirectHX(e, "/admin/competitions/"+record.Id)
}

// Update handles POST to modify an existing competition's settings.
func (h *CompetitionHandler) Update(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	before := record.Original()
	oldTarget := record.GetInt("target_matches")

	oldStart := record.GetString("start_date")
	oldEnd := record.GetString("end_date")

	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return alertError(e, "El nombre es obligatorio")
	}
	record.Set("name", name)
	record.Set("type", e.Request.FormValue("type"))
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")
	if gt := e.Request.FormValue("gender_type"); gt != "" {
		record.Set("gender_type", gt)
	}

	if err := applyCompFormFields(record, e, true); err != nil {
		return err
	}

	hasFixtures := hasCompetitionMatches(h.app, id)
	if msg := validateLeveledFields(record, e, oldTarget, hasFixtures); msg != "" {
		return alertError(e, msg)
	}

	if err := h.app.Save(record); err != nil {
		slog.Error("update competition failed", "err", err)
		return alertError(e, "Error al guardar la competición")
	}
	if detail := competitionUpdateDetail(before, record); detail != "" {
		league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "settings_changed", Detail: detail})
	}

	if record.GetString("start_date") != oldStart || record.GetString("end_date") != oldEnd {
		resetWarnLevels(h.app, id)
		h.refreshRoundSchedule(record)
		if league.IsLeveled(record) {
			h.refreshLeveledArrangeBy(record)
		}
	}

	flash(e, "Competición actualizada")
	return redirectHX(e, "/admin/competitions")
}

// LogoUpload handles POST to upload and set a competition's logo image.
// Admin only. The image is compressed via league.CompressLogoBytes
// (aspect-ratio-preserving, no square crop) before being saved.
func (h *CompetitionHandler) LogoUpload(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	fh := fileHeader(e, "logo")
	if fh == nil {
		return alertError(e, "Selecciona una imagen")
	}

	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return alertError(e, "El archivo debe ser una imagen")
	}

	if fh.Size > avatarMaxUploadSize {
		return alertError(e, "La imagen no puede superar los 5 MB")
	}

	f, errMsg := compressLogo(fh, id+"_logo.jpg")
	if errMsg != "" {
		return alertError(e, errMsg)
	}

	record.Set("logo", f)
	if err := h.app.Save(record); err != nil {
		slog.Error("save competition logo", "err", err)
		return alertError(e, "Error al guardar el logo")
	}

	flash(e, "Logo actualizado")
	return redirectHX(e, "/admin/competitions/"+id)
}

// LogoDelete handles POST to clear a competition's logo. Admin only.
func (h *CompetitionHandler) LogoDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	record.Set("logo", "")
	if err := h.app.Save(record); err != nil {
		slog.Error("delete competition logo", "err", err)
		return alertError(e, "Error al eliminar el logo")
	}

	flash(e, "Logo eliminado")
	return redirectHX(e, "/admin/competitions/"+id)
}

// Toggle switches a competition between active and inactive states.
func (h *CompetitionHandler) Toggle(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	activating := !record.GetBool("active")
	record.Set("active", activating)
	if err := h.app.Save(record); err != nil {
		slog.Error("toggle competition active failed", "err", err)
		return alertError(e, "Error al cambiar el estado")
	}

	kind, detail := "deactivated", "desactivó la competición"
	if activating {
		kind, detail = "activated", "activó la competición"
	}
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: kind, Detail: detail})

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
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "finalized", Detail: "finalizó la competición"})

	flash(e, "Competición finalizada")
	return redirectHX(e, "/admin/competitions/"+id)
}

// PublishCalendar makes a draft calendar visible to players and notifies
// every player in the competition.
func (h *CompetitionHandler) PublishCalendar(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	if comp.GetString("calendar_status") != "draft" {
		return alertError(e, "No hay un calendario en borrador para publicar")
	}

	matches, err := h.app.FindRecordsByFilter("matches", "competition = {:id}", "", 1, 0, map[string]any{"id": id})
	if err != nil || len(matches) == 0 {
		return alertError(e, "No hay partidos que publicar")
	}

	comp.Set("calendar_status", "published")
	if err := h.app.Save(comp); err != nil {
		slog.Error("publish calendar failed", "competition", id, "err", err)
		return alertError(e, "Error al publicar el calendario")
	}
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "calendar_published", Detail: "publicó el calendario"})

	compName := league.CompetitionName(h.app, id)
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
	h.notifier.NotifyPlayers(players, league.NotifCalendarPublished(id, compName))

	flash(e, "Calendario publicado")
	return redirectHX(e, "/admin/competitions/"+id)
}

// ApplyPenalty creates a new penalty row or voids an existing one.
func (h *CompetitionHandler) ApplyPenalty(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	action := e.Request.FormValue("action")

	if action == "remove" {
		penaltyID := e.Request.FormValue("penalty_id")
		voidReason := strings.TrimSpace(e.Request.FormValue("void_reason"))
		rec, err := league.VoidPenalty(h.app, league.VoidPenaltyInput{PenaltyID: penaltyID, AdminID: e.Auth.Id, Reason: voidReason})
		if err != nil {
			return alertError(e, "Error al quitar la penalización")
		}
		h.notifyPenalty(rec, id, "Penalización anulada", fmt.Sprintf("%.0f puntos anulados", rec.GetFloat("amount")))
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

	rec, err := league.ApplyPenalty(h.app, league.PenaltyInput{CompetitionID: id, PairID: pairID, Reason: reason, AdminID: e.Auth.Id, Amount: amount})
	if err != nil {
		return alertError(e, "Error al guardar la penalización")
	}
	h.notifyPenalty(rec, id, "Penalización aplicada", fmt.Sprintf("%.0f puntos — %s", amount, reason))
	return redirectHX(e, "/admin/competitions/"+id)
}

// notifyPenalty notifies both players of a penalty's pair after an apply or void.
func (h *CompetitionHandler) notifyPenalty(penalty *core.Record, compID, title, body string) {
	players := league.PlayersForPair(h.app, penalty.GetString("pair"))
	h.notifier.NotifyPlayers(players, league.Notification{
		Type: "penalty", Title: title, Body: body, Link: "/competition/" + compID,
	})
}

type roundDate struct {
	Number int
	Date   string // "YYYY-MM-DD" for the input value
}

type roundGroup struct {
	Number  int
	Key     string // unique key for auto-expand matching
	Title   string // display title
	Matches []MatchCard
	Played  int
	Total   int
	Warning league.Warning
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

func (h *CompetitionHandler) buildRoundGroups(comp *core.Record, matches []*core.Record, pairNames map[string]string) []roundGroup {
	noPairs := map[string]struct{}{}
	var allCards []MatchCard
	roundMap := map[int][]int{}
	for _, m := range matches {
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], len(allCards))
		allCards = append(allCards, NewMatchRow(m, pairNames, noPairs))
	}
	enrichWithPendingResults(h.app, allCards)
	var rounds []roundGroup
	for rn, idxs := range roundMap {
		ms := make([]MatchCard, len(idxs))
		for i, idx := range idxs {
			ms[i] = allCards[idx]
		}
		key := fmt.Sprintf("round-%d", rn)
		rounds = append(rounds, roundGroup{Number: rn, Key: key, Title: fmt.Sprintf("Jornada %d", rn), Matches: ms})
	}
	sort.Slice(rounds, func(i, j int) bool {
		return rounds[i].Number < rounds[j].Number
	})
	populateRoundProgress(comp, rounds)
	return rounds
}

// buildLeveledRoundGroups builds admin round groups for a leveled league,
// grouping matches into Jornada and monthly "Jugados — <mes año>" groups.
// pairFilter, when non-empty, restricts the list to matches involving that
// pair (mirrors the public competition page's team filter).
func (h *CompetitionHandler) buildLeveledRoundGroups(comp *core.Record, matches []*core.Record, pairNames map[string]string, pairFilter string) []roundGroup {
	noPairs := map[string]struct{}{}
	var allCards []MatchCard
	for _, m := range matches {
		if pairFilter != "" && m.GetString("pair1") != pairFilter && m.GetString("pair2") != pairFilter {
			continue
		}
		allCards = append(allCards, NewMatchRow(m, pairNames, noPairs))
	}
	enrichWithPendingResults(h.app, allCards)

	tz := league.Timezone(h.app)
	groups := leveledGroups(allCards, tz, leveledWindowFor(comp))
	result := make([]roundGroup, len(groups))
	for i, g := range groups {
		played, total := 0, len(g.Matches)
		for _, mc := range g.Matches {
			if mc.Match.GetString("status") == league.StatusFinal {
				played++
			}
		}
		result[i] = roundGroup{Key: g.Key, Title: g.Title, Matches: g.Matches, Played: played, Total: total}
	}
	return result
}

// firstIncompleteRoundGroup returns the key of the first round with an
// unplayed match, or "" if every round is complete (or there are none).
func firstIncompleteRoundGroup(rounds []roundGroup) string {
	for _, r := range rounds {
		if r.Played < r.Total {
			return r.Key
		}
	}
	return ""
}

// populateRoundProgress fills each round's Played/Total/Warning in place.
// Warning is skipped for playoffs, which have admin-fixed dates instead of
// the recommended-arrange-by deadlines this warning is based on.
func populateRoundProgress(comp *core.Record, rounds []roundGroup) {
	isPlayoff := league.IsPlayoff(comp)
	graceDays := comp.GetInt("arrange_grace_days")
	now := time.Now()
	for i := range rounds {
		rounds[i].Total = len(rounds[i].Matches)
		for _, m := range rounds[i].Matches {
			if m.Match.GetString("status") == league.StatusFinal {
				rounds[i].Played++
			}
		}
		if isPlayoff || rounds[i].Played == rounds[i].Total {
			continue
		}
		if deadline, ok := league.RoundArrangeDate(comp, rounds[i].Number); ok {
			rounds[i].Warning = league.WarningLevel(deadline, graceDays, now)
		}
	}
}

// PenaltyRow is one penalty entry for the admin UI.
type PenaltyRow struct {
	ID         string
	Amount     float64
	Reason     string
	AdminName  string
	Date       string
	Voided     bool
	VoidedBy   string
	VoidReason string
}

func (h *CompetitionHandler) getPenaltyRows(compID string) map[string][]PenaltyRow {
	rows := findRecordsLogged(h.app, "getPenaltyRows", RecordQuery{
		Collection: "penalties",
		Filter:     "competition = {:c}",
		Sort:       "-created",
		Params:     map[string]any{"c": compID},
	})
	out := make(map[string][]PenaltyRow, len(rows))
	for _, r := range rows {
		adminName := "Sistema"
		if aid := r.GetString("applied_by"); aid != "" {
			adminName = league.PlayerName(h.app, aid)
		}
		var date string
		if created := r.GetDateTime("created"); !created.IsZero() {
			date = render.FmtTime(created.Time())
		}
		row := PenaltyRow{
			ID:        r.Id,
			Amount:    r.GetFloat("amount"),
			Reason:    r.GetString("reason"),
			AdminName: adminName,
			Date:      date,
			Voided:    r.GetBool("voided"),
		}
		if row.Voided {
			row.VoidReason = r.GetString("void_reason")
			if vid := r.GetString("voided_by"); vid != "" {
				row.VoidedBy = league.PlayerName(h.app, vid)
			} else {
				row.VoidedBy = "Sistema"
			}
		}
		out[r.GetString("pair")] = append(out[r.GetString("pair")], row)
	}
	return out
}

// firstActivePenalty returns, per pair, the first non-voided penalty row —
// used for the mobile list's one-line penalty summary.
func firstActivePenalty(rows map[string][]PenaltyRow) map[string]*PenaltyRow {
	out := make(map[string]*PenaltyRow, len(rows))
	for pairID, pairRows := range rows {
		for i := range pairRows {
			if !pairRows[i].Voided {
				out[pairID] = &pairRows[i]
				break
			}
		}
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
		matches := findRecordsLogged(h.app, "refreshRoundSchedule: find matches", RecordQuery{
			Collection: "matches", Filter: "competition = {:cid}", Params: map[string]any{"cid": comp.Id},
		})
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

// refreshLeveledArrangeBy recomputes arrange_by for every non-final leveled
// match using its stored slot, after start_date/end_date changed. Matches
// with slot=0 (round-robin or pre-migration) are left untouched.
func (h *CompetitionHandler) refreshLeveledArrangeBy(comp *core.Record) {
	matches := findRecordsLogged(h.app, "refreshLeveledArrangeBy: find matches", RecordQuery{
		Collection: "matches",
		Filter:     "competition = {:cid} && status != 'final'",
		Params:     map[string]any{"cid": comp.Id},
	})
	for _, m := range matches {
		slot := m.GetInt("slot")
		if slot <= 0 {
			continue
		}
		deadline, ok := league.SlotDeadline(comp, slot)
		if !ok {
			continue
		}
		m.Set("arrange_by", deadline.Format("2006-01-02"))
		if err := h.app.Save(m); err != nil {
			slog.Error("refresh leveled arrange_by failed", "match", m.Id, "err", err)
		}
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
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "settings_changed", Detail: "cambió las fechas de jornada"})

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
	league.LogCompetitionEvent(h.app, league.CompetitionEvent{CompetitionID: id, ActorID: e.Auth.Id, Kind: "settings_changed", Detail: "cambió las fechas de jornada"})

	resetWarnLevels(h.app, id)
	return redirectHX(e, "/admin/competitions/"+id)
}

// setSchedulingFields validates and applies the scheduling fields shared by
// Create and Update. It returns a ready-to-display Spanish error message
// (empty on success) rather than an error, since every caller only ever
// shows the message verbatim via alertError — never wraps or type-checks it.
func setSchedulingFields(record *core.Record, e *core.RequestEvent) string {
	if v := e.Request.FormValue("start_date"); v != "" {
		record.Set("start_date", v)
	}
	if v := e.Request.FormValue("end_date"); v != "" {
		record.Set("end_date", v)
	}
	start := record.GetString("start_date")
	end := record.GetString("end_date")
	if start != "" && end != "" && end < start {
		return "La fecha de fin debe ser posterior a la de inicio"
	}

	grace, msg := formIntValidated(e, "arrange_grace_days", 3)
	if msg != "" {
		return "Días de gracia: " + msg
	}
	record.Set("arrange_grace_days", grace)

	ws := e.Request.FormValue("walkover_score")
	if ws == "" {
		ws = "6-0 6-0"
	}
	if _, err := league.ParseScore(ws); err != nil {
		return "Marcador de incomparecencia inválido. Usa el formato: 6-0 6-0"
	}
	record.Set("walkover_score", ws)

	penalty, msg := formIntValidated(e, "default_penalty", 3)
	if msg != "" {
		return "Penalización: " + msg
	}
	record.Set("default_penalty", penalty)

	recovery, msg := formIntValidated(e, "recovery_days", 7)
	if msg != "" {
		return "Período extra: " + msg
	}
	record.Set("recovery_days", recovery)

	maxPending, msg := formIntValidated(e, "max_pending_matches", 2)
	if msg != "" {
		return "Máx. partidos pendientes: " + msg
	}
	record.Set("max_pending_matches", maxPending)

	target, msg := formIntValidated(e, "target_matches", 0)
	if msg != "" {
		return "Partidos por pareja: " + msg
	}
	record.Set("target_matches", target)

	open, msg := formIntValidated(e, "open_assignments", 0)
	if msg != "" {
		return "Partidos abiertos: " + msg
	}
	record.Set("open_assignments", open)

	return ""
}

// validateLeveledFields checks leveled-league constraints that depend on other
// fields (play_twice, open vs target) and on existing matches. hasFixtures must
// be true when the competition already has generated matches.
func validateLeveledFields(record *core.Record, _ *core.RequestEvent, oldTarget int, hasFixtures bool) string {
	target := record.GetInt("target_matches")
	if target == 0 {
		return ""
	}
	if record.GetBool("play_twice") {
		return "Una liga nivelada no puede ser a doble vuelta"
	}
	open := record.GetInt("open_assignments")
	if open > target {
		return "Partidos abiertos a la vez no puede superar los partidos por pareja"
	}
	if hasFixtures && target != oldTarget {
		return "No se puede cambiar con el calendario generado"
	}
	if hasFixtures && (record.GetDateTime("start_date").IsZero() || record.GetDateTime("end_date").IsZero()) {
		return "No se pueden eliminar las fechas de una competición nivelada con partidos"
	}
	return ""
}

// hasCompetitionMatches reports whether the competition has any matches.
func hasCompetitionMatches(app core.App, compID string) bool {
	matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 1, 0,
		map[string]any{"c": compID})
	return err == nil && len(matches) > 0
}

// formIntValidated parses a form field as a non-negative integer, returning
// a Spanish display message (empty on success) instead of an error.
func formIntValidated(e *core.RequestEvent, field string, def int) (int, string) {
	v := e.Request.FormValue(field)
	if v == "" {
		return def, ""
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, "debe ser un número"
	}
	if n < 0 {
		return 0, "no puede ser negativo"
	}
	return n, ""
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
	flash(e, "Documento adjuntado")
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
	flash(e, "Documento desvinculado")
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}

func compReminderHoursDisplay(comp *core.Record) string {
	raw := comp.GetString("match_reminder_hours")
	if raw == "" {
		return ""
	}
	var hours []int
	if json.Unmarshal([]byte(raw), &hours) != nil || len(hours) == 0 {
		return ""
	}
	return league.FormatReminderHours(hours)
}
