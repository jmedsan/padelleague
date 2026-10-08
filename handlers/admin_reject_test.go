package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// adminDecision is one admin action on a match holding one pending result
// proposal and one pending date proposal.
type adminDecision struct {
	name   string
	url    func(m *core.Record) string
	body   func(m *core.Record) string
	prep   func(tb testing.TB, app core.App, m *core.Record) // optional extra state
	result string                                            // "rejected" or "pending": the result proposal
	date   string                                            // same, for the date proposal
}

func (d adminDecision) run(t *testing.T) {
	t.Helper()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           d.name,
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	var admin, author *core.Record
	var resultProp, dateProp *core.Record
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RejA")
		p2 := handlers.MakePairTB(tb, app, "RejB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		author = mustUser(tb, app, p1.GetString("player1"))
		resultProp = handlers.MakeResultProposal(tb, app, m.Id, author.Id, "6-3 6-4")
		dateProp = schedulingProposal(tb, app, m.Id, author.Id, "2026-09-20")
		if d.prep != nil {
			d.prep(tb, app, m)
			m = mustRecord(tb, app, "matches", m.Id)
		}
		admin = makeAdminUser(tb, app)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		hdrs["HX-Request"] = "true"
		s.Headers = hdrs
		s.URL = d.url(m)
		s.Body = strings.NewReader(d.body(m))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assertProposal(tb, app, resultProp, d.result, admin.Id, author.Id)
		assertProposal(tb, app, dateProp, d.date, admin.Id, author.Id)
		_ = matchID
	}
	s.Test(t)
}

func mustUser(tb testing.TB, app core.App, id string) *core.Record {
	return mustRecord(tb, app, "users", id)
}

func mustRecord(tb testing.TB, app core.App, col, id string) *core.Record {
	tb.Helper()
	rec, err := app.FindRecordById(col, id)
	require.NoError(tb, err)
	return rec
}

// assertProposal checks the proposal's final status and, when it is rejected,
// that the admin rejected it: the reason is recorded and the timeline holds one
// frozen "reject" entry attributed to the admin, showing the proposal's data.
func assertProposal(tb testing.TB, app core.App, prop *core.Record, want, adminID, authorID string) {
	tb.Helper()
	got := mustRecord(tb, app, "match_messages", prop.Id)
	assert.Equal(tb, want, got.GetString("proposal_status"), "%s proposal", prop.GetString("type"))
	entries, err := app.FindRecordsByFilter("match_messages", "parent = {:p}", "", 0, 0, map[string]any{"p": prop.Id})
	require.NoError(tb, err)
	if want != "rejected" {
		assert.Empty(tb, entries, "no response entry for a proposal nobody decided")
		return
	}
	assert.Equal(tb, "Resuelto por el administrador", got.GetString("rejection_text"))
	require.Len(tb, entries, 1, "one timeline entry for the admin's rejection")
	entry := entries[0]
	assert.Equal(tb, adminID, entry.GetString("author"), "attributed to the admin")
	assert.NotEqual(tb, authorID, entry.GetString("author"))
	pd := league.ParseProposalData(entry.Get("proposal_data"))
	assert.Equal(tb, "reject", pd.Action)
	want2 := league.ParseProposalData(prop.Get("proposal_data"))
	assert.Equal(tb, want2.Scores, pd.Scores, "frozen snapshot: result")
	assert.Equal(tb, want2.Date, pd.Date, "frozen snapshot: date")
}

func TestAdminDecisions_RejectTheProposalsTheyDecide(t *testing.T) {
	t.Parallel()
	matchURL := func(suffix string) func(*core.Record) string {
		return func(m *core.Record) string { return "/match/" + m.Id + suffix }
	}
	for _, d := range []adminDecision{
		{name: "override score rejects both proposals", url: matchURL("/admin-override"),
			body: func(*core.Record) string { return "scores=6-1+6-1" }, result: "rejected", date: "rejected"},
		{name: "override date rejects only the date proposal", url: matchURL("/admin-override"),
			body: func(*core.Record) string { return "date=2026-09-25" }, result: "pending", date: "rejected"},
		{name: "override time rejects only the date proposal", url: matchURL("/admin-override"),
			body: func(*core.Record) string { return "time=19:30" }, result: "pending", date: "rejected"},
		{name: "override court rejects nothing", url: matchURL("/admin-override"),
			body: func(*core.Record) string { return "court_number=3" }, result: "pending", date: "pending"},
		{name: "admin correct finalizes and rejects both", url: matchURL("/correct"),
			body: func(*core.Record) string { return "scores=6-4+6-3" }, result: "rejected", date: "rejected"},
		{name: "walkover approve rejects both", url: func(m *core.Record) string { return "/admin/disputes/" + m.Id + "/walkover-approve" },
			prep: func(tb testing.TB, app core.App, m *core.Record) {
				m.Set("review_type", "walkover")
				require.NoError(tb, app.Save(m))
			},
			body:   func(m *core.Record) string { return "winner=" + m.GetString("pair1") },
			result: "rejected", date: "rejected"},
		{name: "pair withdrawal walkover rejects both",
			url: func(m *core.Record) string {
				return "/admin/competitions/" + m.GetString("competition") + "/withdraw-pair"
			},
			body:   func(m *core.Record) string { return "pair_id=" + m.GetString("pair1") },
			result: "rejected", date: "rejected"},
		{name: "dispute resolve rejects both", url: func(m *core.Record) string { return "/admin/disputes/" + m.Id + "/resolve" },
			prep: func(tb testing.TB, app core.App, m *core.Record) {
				m.Set("status", "disputed")
				require.NoError(tb, app.Save(m))
			},
			body: func(*core.Record) string { return "score=6-2+6-2" }, result: "rejected", date: "rejected"},
	} {
		d.run(t)
	}
}
