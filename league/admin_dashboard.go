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

// AdminDashboard returns setup checklists and alerts for the admin home.
func AdminDashboard(app core.App, now time.Time) ([]CompSetup, []AdminAlert, error) {
	allComps, err := app.FindRecordsByFilter("competitions",
		"", "name", 0, 0, nil)
	if err != nil {
		return nil, nil, err
	}

	var setups []CompSetup
	var alerts []AdminAlert

	for _, c := range allComps {
		if !c.GetBool("active") {
			setup := buildSetup(app, c)
			setups = append(setups, setup)
			continue
		}
		compAlerts := buildAlerts(app, c, now)
		alerts = append(alerts, compAlerts...)
	}

	sortAlerts(alerts)
	return setups, alerts, nil
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

func buildAlerts(app core.App, c *core.Record, now time.Time) []AdminAlert {
	compName := c.GetString("name")
	var alerts []AdminAlert

	disputed, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'disputed'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})
	for _, m := range disputed {
		p1, p2 := pairNamesForMatch(app, m)
		desc := "Disputa abierta"
		if s := m.GetString("scores"); s != "" {
			desc = s
			if ds := m.GetString("disputed_scores"); ds != "" {
				desc += " → propone: " + ds
			}
		}
		alerts = append(alerts, AdminAlert{
			Kind: "dispute", MatchID: m.Id,
			Pair1ID: m.GetString("pair1"), Pair1: p1,
			Pair2ID: m.GetString("pair2"), Pair2: p2,
			CompName: compName, RoundNumber: m.GetInt("round_number"),
			Description: desc,
		})
	}

	pending, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'pending'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})
	alerts = append(alerts, pendingAlerts(app, c, pending, now)...)

	return alerts
}

// pendingAlerts builds walkover-approval and overdue alerts for a
// competition's pending matches. A walkover approval is always surfaced; an
// overdue nudge is skipped once the competition has finished.
func pendingAlerts(app core.App, c *core.Record, pending []*core.Record, now time.Time) []AdminAlert {
	compName := c.GetString("name")
	graceDays := c.GetInt("arrange_grace_days")

	phase := PhaseOf(c, now)
	recovery := phase == PhaseRecovery

	var alerts []AdminAlert
	for _, m := range pending {
		rn := m.GetInt("round_number")

		if m.GetString("review_type") == "walkover" {
			p1, p2 := pairNamesForMatch(app, m)
			alerts = append(alerts, AdminAlert{
				Kind: "walkover", MatchID: m.Id,
				Pair1ID: m.GetString("pair1"), Pair1: p1,
				Pair2ID: m.GetString("pair2"), Pair2: p2,
				CompName: compName, RoundNumber: rn,
				Description: "Incomparecencia pendiente de aprobación",
			})
			continue
		}

		if phase == PhaseFinished {
			continue
		}

		deadline, ok := RoundArrangeDate(c, rn)
		if !ok {
			continue
		}
		wl := WarningLevel(deadline, graceDays, now)
		if wl >= WarnOverdue {
			p1, p2 := pairNamesForMatch(app, m)
			alerts = append(alerts, AdminAlert{
				Kind: "overdue", MatchID: m.Id,
				Pair1ID: m.GetString("pair1"), Pair1: p1,
				Pair2ID: m.GetString("pair2"), Pair2: p2,
				CompName: compName, RoundNumber: rn,
				Description: "Vencido — sin organizar",
				Recovery:    recovery,
			})
		}
	}
	return alerts
}

// OutstandingMatch is one non-final match decorated with its deadline and
// warning level for the admin outstanding-matches view.
type OutstandingMatch struct {
	MatchID          string
	CompetitionName  string
	RoundNumber      int
	Pair1ID, Pair2ID string
	Pair1, Pair2     string
	Status           string
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
			CompetitionName: compName,
			RoundNumber:     m.GetInt("round_number"),
			Pair1ID:         m.GetString("pair1"),
			Pair1:           p1,
			Pair2ID:         m.GetString("pair2"),
			Pair2:           p2,
			Status:          m.GetString("status"),
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
