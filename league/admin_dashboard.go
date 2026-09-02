package league

import (
	"cmp"
	"log/slog"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// SetupItem represents one step in the competition setup checklist.
type SetupItem struct {
	Label string
	Done  bool
}

// CompSetup holds setup status for a not-yet-active competition.
type CompSetup struct {
	CompID   string
	CompName string
	Items    []SetupItem
	Ready    bool
}

// AdminAlert represents an issue the admin should address.
type AdminAlert struct {
	Kind        string // "dispute", "overdue", "walkover"
	MatchID     string
	Pair1ID     string
	Pair1       string
	Pair2ID     string
	Pair2       string
	CompName    string
	RoundNumber int
	Description string
	Recovery    bool // true when the competition is in its recovery window
}

// HealthItem is one entry inside a HealthCategory: a match needing attention,
// or (for the unpaid category) a pair with an outstanding balance. Urgent and
// CategoryTitle mirror the parent HealthCategory's fields, set by
// HealthReport once every item is classified — so a HealthItem carries
// enough context to render standalone (e.g. the home page's flattened
// urgent-only list) without needing its parent category alongside it.
type HealthItem struct {
	MatchID  string // empty for unpaid items, which are per-pair not per-match
	CompID   string
	Pair1    string
	Pair2    string
	CompName string
	Round    int
	Detail   string
	Warning  Warning
	Recovery bool

	Urgent        bool
	CategoryTitle string
}

// HealthCategory groups every HealthItem of one kind for the admin health
// dashboard. Always present in HealthReport's result, even when Items is
// empty, so the dashboard shows a fixed set of categories.
type HealthCategory struct {
	Key     string // "disputes", "walkovers", "overdue", "unscheduled", "unpaid"
	Title   string
	Items   []HealthItem
	ListURL string
	Urgent  bool
}

// healthCategoryDefs is the fixed, ordered set of categories HealthReport
// always returns.
var healthCategoryDefs = []HealthCategory{
	{Key: "disputes", Title: "Disputas", ListURL: "/admin/disputes", Urgent: true},
	{Key: "walkovers", Title: "Incomparecencias", ListURL: "/admin/disputes", Urgent: true},
	{Key: "overdue", Title: "Vencidos", ListURL: "/admin/outstanding"},
	{Key: "unscheduled", Title: "Sin fecha", ListURL: "/admin/outstanding"},
	{Key: "unpaid", Title: "Sin pagar"},
}

// HealthReport merges every admin-facing match/payment issue across active
// competitions into a fixed set of categories: disputes, walkovers, overdue
// (round deadline + grace — the same definition the reminder cron uses),
// unscheduled (no date set), and unpaid. Always returns all categories, even
// when empty, so a surface rendering this list shows the full picture.
func HealthReport(app core.App, now time.Time) []HealthCategory {
	categories := make(map[string]*HealthCategory, len(healthCategoryDefs))
	report := make([]HealthCategory, len(healthCategoryDefs))
	for i, def := range healthCategoryDefs {
		report[i] = def
		categories[def.Key] = &report[i]
	}

	activeComps, err := app.FindRecordsByFilter("competitions", "active = true", "name", 0, 0, nil)
	if err != nil {
		slog.Error("health report: load active competitions", "err", err)
		return report
	}
	for _, c := range activeComps {
		addCompHealth(app, c, now, categories)
	}
	addUnpaidHealth(app, activeComps, categories["unpaid"])

	for i := range report {
		for j := range report[i].Items {
			report[i].Items[j].Urgent = report[i].Urgent
			report[i].Items[j].CategoryTitle = report[i].Title
		}
		sortHealthItems(report[i].Items)
	}
	return report
}

// CompHealthItems returns the HealthItems for one competition's given
// category keys (e.g. "disputes", "walkovers"), regardless of whether the
// competition is active — unlike HealthReport, which only scans active
// competitions for the admin-wide dashboard. Use this for a single
// competition's own detail page, where an inactive competition must still
// show its open disputes.
func CompHealthItems(app core.App, compID string, now time.Time, keys ...string) []HealthItem {
	comp, err := app.FindRecordById("competitions", compID)
	if err != nil {
		return nil
	}
	categories := make(map[string]*HealthCategory, len(healthCategoryDefs))
	report := make([]HealthCategory, len(healthCategoryDefs))
	for i, def := range healthCategoryDefs {
		report[i] = def
		categories[def.Key] = &report[i]
	}
	addCompHealth(app, comp, now, categories)

	wanted := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		wanted[k] = struct{}{}
	}
	var items []HealthItem
	for _, cat := range report {
		if _, ok := wanted[cat.Key]; !ok {
			continue
		}
		for _, item := range cat.Items {
			item.Urgent = cat.Urgent
			item.CategoryTitle = cat.Title
			items = append(items, item)
		}
	}
	sortHealthItems(items)
	return items
}

// compHealthCtx bundles the per-competition values used while classifying
// its pending matches into health categories.
type compHealthCtx struct {
	comp      *core.Record
	compName  string
	graceDays int
	phase     Phase
	recovery  bool
	now       time.Time
}

func addCompHealth(app core.App, c *core.Record, now time.Time, categories map[string]*HealthCategory) {
	phase := PhaseOf(c, now)
	ctx := compHealthCtx{
		comp:      c,
		compName:  c.GetString("name"),
		graceDays: c.GetInt("arrange_grace_days"),
		phase:     phase,
		recovery:  phase == PhaseRecovery,
		now:       now,
	}

	disputed, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'disputed'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})
	for _, m := range disputed {
		if m.GetString("review_type") == "walkover" {
			categories["walkovers"].Items = append(categories["walkovers"].Items,
				walkoverHealthItem(app, m, ctx.compName))
			continue
		}
		categories["disputes"].Items = append(categories["disputes"].Items,
			disputeHealthItem(app, m, ctx.compName))
	}

	pending, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'pending'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})
	for _, m := range pending {
		addPendingHealth(app, m, ctx, categories)
	}
}

// addPendingHealth classifies a status=pending match. review_type=walkover
// is not checked here: ReportUnplayed always pairs it with status=disputed
// (see addCompHealth), so no live code path produces a pending walkover.
func addPendingHealth(app core.App, m *core.Record, ctx compHealthCtx, categories map[string]*HealthCategory) {
	rn := m.GetInt("round_number")
	item := healthItem(app, m, ctx.compName, rn)

	if m.GetString("date") == "" {
		unscheduled := item
		unscheduled.Detail = "Sin fecha propuesta"
		categories["unscheduled"].Items = append(categories["unscheduled"].Items, unscheduled)
	}

	if ctx.phase == PhaseFinished {
		return
	}
	deadline, ok := RoundArrangeDate(ctx.comp, rn)
	if !ok {
		return
	}
	wl := WarningLevel(deadline, ctx.graceDays, ctx.now)
	if wl >= WarnOverdue {
		item.Detail = "Vencido — sin organizar"
		item.Warning = wl
		item.Recovery = ctx.recovery
		categories["overdue"].Items = append(categories["overdue"].Items, item)
	}
}

func disputeHealthItem(app core.App, m *core.Record, compName string) HealthItem {
	item := healthItem(app, m, compName, m.GetInt("round_number"))
	item.Detail = "Disputa abierta"
	if s := m.GetString("scores"); s != "" {
		item.Detail = s
		if ds := m.GetString("disputed_scores"); ds != "" {
			item.Detail += " → propone: " + ds
		}
	}
	return item
}

func walkoverHealthItem(app core.App, m *core.Record, compName string) HealthItem {
	item := healthItem(app, m, compName, m.GetInt("round_number"))
	item.Detail = "Incomparecencia pendiente de aprobación"
	return item
}

func healthItem(app core.App, m *core.Record, compName string, round int) HealthItem {
	p1, p2 := pairNamesForMatch(app, m)
	return HealthItem{
		MatchID: m.Id, CompID: m.GetString("competition"),
		Pair1: p1, Pair2: p2, CompName: compName, Round: round,
	}
}

func addUnpaidHealth(app core.App, activeComps []*core.Record, unpaid *HealthCategory) {
	for _, comp := range activeComps {
		ps := make(map[string]bool)
		_ = comp.UnmarshalJSONField("payment_status", &ps)
		pairIDs := comp.GetStringSlice("pairs")
		pairNames := PairNames(app, pairIDs)
		compName := comp.GetString("name")
		for _, pid := range pairIDs {
			if ps[pid] {
				continue
			}
			unpaid.Items = append(unpaid.Items, HealthItem{
				CompID: comp.Id, Pair1: pairNames[pid], CompName: compName,
				Detail: "Pago pendiente",
			})
		}
	}
}

// sortHealthItems orders most-urgent first: warning desc, competition name,
// round number, match ID.
func sortHealthItems(items []HealthItem) {
	slices.SortStableFunc(items, func(a, b HealthItem) int {
		if a.Warning != b.Warning {
			return cmp.Compare(b.Warning, a.Warning)
		}
		if c := cmp.Compare(a.CompName, b.CompName); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Round, b.Round); c != 0 {
			return c
		}
		return cmp.Compare(a.MatchID, b.MatchID)
	})
}

// AdminDashboard returns setup checklists and alerts for the admin home.
// Alerts are derived from HealthReport's urgent categories (disputes,
// walkovers) and its overdue category, so all three surfaces share one
// classification.
func AdminDashboard(app core.App, now time.Time) ([]CompSetup, []AdminAlert, error) {
	allComps, err := app.FindRecordsByFilter("competitions",
		"", "name", 0, 0, nil)
	if err != nil {
		return nil, nil, err
	}

	var setups []CompSetup
	for _, c := range allComps {
		if !c.GetBool("active") {
			setups = append(setups, buildSetup(app, c))
		}
	}

	alerts := alertsFromHealthReport(HealthReport(app, now))
	return setups, alerts, nil
}

func alertsFromHealthReport(categories []HealthCategory) []AdminAlert {
	kindByKey := map[string]string{"disputes": "dispute", "walkovers": "walkover", "overdue": "overdue"}
	var alerts []AdminAlert
	for _, cat := range categories {
		kind, ok := kindByKey[cat.Key]
		if !ok {
			continue
		}
		for _, item := range cat.Items {
			alerts = append(alerts, AdminAlert{
				Kind: kind, MatchID: item.MatchID,
				Pair1: item.Pair1, Pair2: item.Pair2,
				CompName: item.CompName, RoundNumber: item.Round,
				Description: item.Detail, Recovery: item.Recovery,
			})
		}
	}
	sortAlerts(alerts)
	return alerts
}

func buildSetup(app core.App, c *core.Record) CompSetup {
	pairIDs := c.GetStringSlice("pairs")
	hasPairs := len(pairIDs) >= 2

	matches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 1, 0,
		map[string]any{"cid": c.Id})
	hasFixtures := len(matches) > 0

	hasDates := !c.GetDateTime("start_date").Time().IsZero() &&
		!c.GetDateTime("end_date").Time().IsZero()

	items := []SetupItem{
		{Label: "Parejas añadidas", Done: hasPairs},
		{Label: "Jornadas generadas", Done: hasFixtures},
		{Label: "Fechas configuradas", Done: hasDates},
	}

	ready := hasPairs && hasFixtures && hasDates

	return CompSetup{
		CompID:   c.Id,
		CompName: c.GetString("name"),
		Items:    items,
		Ready:    ready,
	}
}

// OutstandingMatch is one non-final match decorated with its deadline and
// warning level for the admin outstanding-matches view.
type OutstandingMatch struct {
	MatchID          string
	CompetitionID    string
	CompetitionName  string
	RoundNumber      int
	Pair1ID, Pair2ID string
	Pair1, Pair2     string
	Status           string
	StatusLabel      string
	ArrangeBy        string // "DD/MM", or "" when no schedule (e.g. playoffs)
	Warning          Warning
	deadline         time.Time // sort key backing ArrangeBy; zero when unset
}

// OutstandingMatches returns every non-final match in an active competition,
// decorated with its stored deadline and warning level, ordered most-urgent
// first. Playoff matches show status only — their dates are fixed, not
// scheduled against a deadline.
func OutstandingMatches(app core.App, now time.Time) []OutstandingMatch {
	comps, err := app.FindRecordsByFilter("competitions", "active = true", "", 0, 0, nil)
	if err != nil {
		return nil
	}

	var out []OutstandingMatch
	for _, c := range comps {
		out = append(out, outstandingForComp(app, c, now)...)
	}

	sortOutstanding(out)
	return out
}

func outstandingForComp(app core.App, c *core.Record, now time.Time) []OutstandingMatch {
	compName := c.GetString("name")
	graceDays := c.GetInt("arrange_grace_days")
	isPlayoff := IsPlayoff(c)

	matches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status != 'final'", "", 0, 0,
		map[string]any{"cid": c.Id})

	out := make([]OutstandingMatch, 0, len(matches))
	for _, m := range matches {
		p1, p2 := pairNamesForMatch(app, m)
		om := OutstandingMatch{
			MatchID:         m.Id,
			CompetitionID:   c.Id,
			CompetitionName: compName,
			RoundNumber:     m.GetInt("round_number"),
			Pair1ID:         m.GetString("pair1"),
			Pair1:           p1,
			Pair2ID:         m.GetString("pair2"),
			Pair2:           p2,
			Status:          m.GetString("status"),
			StatusLabel:     StatusLabel(m.GetString("status")),
		}
		if !isPlayoff {
			if deadline, ok := RoundArrangeDate(c, om.RoundNumber); ok {
				om.deadline = deadline
				om.ArrangeBy = fmtShortDate(deadline)
				om.Warning = WarningLevel(deadline, graceDays, now)
			}
		}
		out = append(out, om)
	}
	return out
}

// sortOutstanding orders most-urgent first: warning desc, deadline asc (a
// missing deadline sorts last), competition name, round number, match ID.
func sortOutstanding(out []OutstandingMatch) {
	slices.SortStableFunc(out, func(a, b OutstandingMatch) int {
		if a.Warning != b.Warning {
			return cmp.Compare(b.Warning, a.Warning) // desc
		}
		if !a.deadline.Equal(b.deadline) {
			if a.deadline.IsZero() {
				return 1
			}
			if b.deadline.IsZero() {
				return -1
			}
			return a.deadline.Compare(b.deadline)
		}
		if c := cmp.Compare(a.CompetitionName, b.CompetitionName); c != 0 {
			return c
		}
		if c := cmp.Compare(a.RoundNumber, b.RoundNumber); c != 0 {
			return c
		}
		return cmp.Compare(a.MatchID, b.MatchID)
	})
}

func pairNamesForMatch(app core.App, m *core.Record) (string, string) {
	p1Name, p2Name := "?", "?"
	if p, err := app.FindRecordById("pairs", m.GetString("pair1")); err == nil {
		p1Name = p.GetString("name")
	}
	if p, err := app.FindRecordById("pairs", m.GetString("pair2")); err == nil {
		p2Name = p.GetString("name")
	}
	return p1Name, p2Name
}

// PlayoffPrompt names a finished league that should prompt a playoff.
type PlayoffPrompt struct {
	CompID   string
	CompName string
}

// PlayoffPrompts returns finished league competitions with no active playoff,
// so the admin is prompted to create one. A running playoff suppresses all
// prompts (see the single-competition limitation in Technical Decisions).
func PlayoffPrompts(app core.App, activeComps []*core.Record, now time.Time) []PlayoffPrompt {
	if slices.ContainsFunc(activeComps, IsPlayoff) {
		return nil
	}
	var prompts []PlayoffPrompt
	for _, c := range activeComps {
		if IsPlayoff(c) || PhaseOf(c, now) != PhaseFinished {
			continue
		}
		if !allMatchesFinal(app, c.Id) {
			continue
		}
		prompts = append(prompts, PlayoffPrompt{CompID: c.Id, CompName: c.GetString("name")})
	}
	return prompts
}

func allMatchesFinal(app core.App, compID string) bool {
	pending, err := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status != 'final'", "", 1, 0,
		map[string]any{"cid": compID})
	if err != nil {
		slog.Error("playoff prompts: count non-final matches", "comp", compID, "err", err)
		return false
	}
	return len(pending) == 0
}

func sortAlerts(alerts []AdminAlert) {
	kindOrder := map[string]int{"dispute": 0, "walkover": 1, "overdue": 2}
	slices.SortStableFunc(alerts, func(a, b AdminAlert) int {
		return cmp.Compare(kindOrder[a.Kind], kindOrder[b.Kind])
	})
}
