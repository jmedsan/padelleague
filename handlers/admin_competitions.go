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

	pairIDs := comp.GetStringSlice("pairs")
	seeding := getSeeding(comp)
	paymentStatus := getPaymentStatus(comp)
	penaltyRows := h.getPenaltyRows(id)

	pairEntries := buildPairEntries(h.app, pairIDs, seeding, paymentStatus)
	allPairs := availablePairs(h.app, pairIDs)
	allComps := findRecordsLogged(h.app, "Detail: find other competitions", RecordQuery{
		Collection: "competitions", Filter: "id != {:cid}", Sort: "name", Params: map[string]any{"cid": id},
	})

	matches := findRecordsLogged(h.app, "Detail: find matches", RecordQuery{
		Collection: "matches", Filter: "competition = {:cid}", Sort: "round_number,created", Params: map[string]any{"cid": id},
	})

	pairNameMap := league.PairNames(h.app, pairIDs)

	rounds := h.buildRoundGroups(comp, matches, pairNameMap)
	disputes := league.CompHealthItems(h.app, id, time.Now(), "disputes", "walkovers")
	allUsers := findRecordsLogged(h.app, "Detail: find players", RecordQuery{
		Collection: "users", Filter: "roles ~ 'player'", Sort: "display_name",
	})
	isLeague := comp.GetString("type") == "league"

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
		"IsLeague":            isLeague,
		"HasFixtures":         len(matches) > 0,
		"HasUnpaid":           anyUnpaid(pairEntries),
		"UnpaidCount":         countUnpaid(pairEntries),
		"Phase":               league.PhaseOf(comp, time.Now()),
		"Mode":                AdminFull,
		"Invitations":         CompetitionInvitations(h.app, id),
		"FooterCompetitionID": id,
	}
	h.addDetailExtras(data, comp, matches)
	return h.renderPage(e, "admin/competition-detail.html", data)
}

func (h *CompetitionHandler) addDetailExtras(data map[string]any, comp *core.Record, matches []*core.Record) {
	if comp.GetString("type") == "league" {
		rows, _ := h.leagueSvc.ComputeStandings(comp.Id)
		hasPlayed := false
		for _, s := range rows {
			if s.Played > 0 {
				hasPlayed = true
			}
			if s.Penalty > 0 {
				data["HasPenalties"] = true
			}
		}
		if len(rows) >= 2 && hasPlayed {
			data["Standings"] = rows
		}
		if len(matches) > 0 {
			data["RoundDates"] = h.buildRoundDates(comp)
		}
	}
	attachedViews, unattachedDocs := h.buildDetailDocs(comp)
	data["AttachedDocViews"] = attachedViews
	data["UnattachedDocs"] = unattachedDocs

	attachedSponsors, unattachedSponsors := h.buildDetailSponsors(comp)
	data["AttachedSponsors"] = attachedSponsors
	data["UnattachedSponsors"] = unattachedSponsors
}

func (h *CompetitionHandler) buildDetailSponsors(comp *core.Record) ([]*core.Record, []*core.Record) {
	attachedIDs := comp.GetStringSlice("sponsors")
	var attached []*core.Record
	attachedSet := make(map[string]struct{}, len(attachedIDs))
	for _, sid := range attachedIDs {
		attachedSet[sid] = struct{}{}
		if s, err := h.app.FindRecordById("sponsors", sid); err == nil {
			attached = append(attached, s)
		}
	}
	allSponsors := findRecordsLogged(h.app, "buildDetailSponsors: find sponsors", RecordQuery{
		Collection: "sponsors", Sort: "name",
	})
	var unattached []*core.Record
	for _, s := range allSponsors {
		if _, ok := attachedSet[s.Id]; !ok {
			unattached = append(unattached, s)
		}
	}
	return attached, unattached
}

func (h *CompetitionHandler) buildDetailDocs(comp *core.Record) ([]DocumentView, []*core.Record) {
	attachedIDs := comp.GetStringSlice("documents")
	attachMode := Mode{Admin: true, Editable: true, Row: true}
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
	allDocs := findRecordsLogged(h.app, "buildDetailDocs: find documents", RecordQuery{
		Collection: "documents", Sort: "title",
	})
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
	name := strings.TrimSpace(e.Request.FormValue("name"))
	if name == "" {
		return alertError(e, "El nombre es obligatorio")
	}
	record.Set("name", name)
	record.Set("type", e.Request.FormValue("type"))
	record.Set("active", e.Request.FormValue("active") == "on")
	record.Set("play_twice", e.Request.FormValue("play_twice") == "on")
	if gt := e.Request.FormValue("gender_type"); gt != "" {
		record.Set("gender_type", gt)
	} else {
		record.Set("gender_type", "free")
	}

	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			return alertError(e, "Tiempo de espera debe ser un número")
		}
		record.Set("quorum_timeout_hours", hours)
	}

	if msg := setSchedulingFields(record, e); msg != "" {
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

	return redirectHX(e, "/admin/competitions/"+record.Id)
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

	if v := e.Request.FormValue("quorum_timeout_hours"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			return alertError(e, "Tiempo de espera debe ser un número")
		}
		record.Set("quorum_timeout_hours", hours)
	}

	if msg := setSchedulingFields(record, e); msg != "" {
		return alertError(e, msg)
	}

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

	return redirectHX(e, "/admin/competitions/"+id)
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
	populateRoundProgress(comp, rounds)
	return rounds
}

// firstIncompleteRoundGroup returns the number of the first round with an
// unplayed match, or 0 if every round is complete (or there are none).
func firstIncompleteRoundGroup(rounds []roundGroup) int {
	for _, r := range rounds {
		if r.Played < r.Total {
			return r.Number
		}
	}
	return 0
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
			Date:      render.FmtTime(r.GetDateTime("created").Time()),
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

	recovery, msg := formIntValidated(e, "recovery_days", 14)
	if msg != "" {
		return "Período extra: " + msg
	}
	record.Set("recovery_days", recovery)
	return ""
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
		Link:  "/competition/" + comp.Id,
	})
	h.notifier.EmailPlayers(players, title, body, "/competition/"+comp.Id)

	slog.Info("broadcast sent", "competition", comp.Id, "players", len(players))
	return alertSuccess(e, "Anuncio enviado a "+strconv.Itoa(len(players))+" jugadores")
}
