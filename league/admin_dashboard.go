package league

import (
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
	Pair1       string
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
		alerts = append(alerts, AdminAlert{
			Kind: "dispute", MatchID: m.Id,
			Pair1: p1, Pair2: p2,
			CompName: compName, RoundNumber: m.GetInt("round_number"),
			Description: "Disputa abierta",
		})
	}

	pending, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'pending'",
		"round_number", 0, 0,
		map[string]any{"cid": c.Id})
	alerts = append(alerts, pendingAlerts(app, c, compName, pending, now)...)

	return alerts
}

// pendingAlerts builds walkover-approval and overdue alerts for a
// competition's pending matches. A walkover approval is always surfaced; an
// overdue nudge is skipped once the competition has finished.
func pendingAlerts(app core.App, c *core.Record, compName string, pending []*core.Record, now time.Time) []AdminAlert {
	graceDays := c.GetInt("arrange_grace_days")

	phase := PhaseUnknown
	if !IsPlayoff(c) {
		phase = CompetitionPhase(c, now)
	}
	recovery := phase == PhaseRecovery

	var alerts []AdminAlert
	for _, m := range pending {
		rn := m.GetInt("round_number")

		if m.GetString("review_type") == "walkover" {
			p1, p2 := pairNamesForMatch(app, m)
			alerts = append(alerts, AdminAlert{
				Kind: "walkover", MatchID: m.Id,
				Pair1: p1, Pair2: p2,
				CompName: compName, RoundNumber: rn,
				Description: "Walkover pendiente de aprobación",
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
				Pair1: p1, Pair2: p2,
				CompName: compName, RoundNumber: rn,
				Description: "Vencido — sin organizar",
				Recovery:    recovery,
			})
		}
	}
	return alerts
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

func sortAlerts(alerts []AdminAlert) {
	kindOrder := map[string]int{"dispute": 0, "walkover": 1, "overdue": 2}
	for i := 1; i < len(alerts); i++ {
		for j := i; j > 0; j-- {
			if kindOrder[alerts[j].Kind] < kindOrder[alerts[j-1].Kind] {
				alerts[j], alerts[j-1] = alerts[j-1], alerts[j]
			}
		}
	}
}
