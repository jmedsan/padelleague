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

// releasedMatch builds a leveled competition with one released match.
func releasedMatch(tb testing.TB, app *tests.TestApp) (*core.Record, *core.Record) {
	p1 := handlers.MakePairTB(tb, app, "RlsA")
	p2 := handlers.MakePairTB(tb, app, "RlsB")
	p3 := handlers.MakePairTB(tb, app, "RlsC")
	comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
	m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, league.StatusReleased)
	m.Set("round_number", 0)
	require.NoError(tb, app.Save(m))
	return m, p1
}

func TestReleasedRefusesAdminWrites(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, path, body string }{
		{"message", "/thread/message", "content=hola"},
		{"override", "/admin-override", "date=2026-10-10"},
		{"cancel date", "/cancel-date", ""},
		{"arbitration", "/arbitration", "reason=x"},
		{"correct", "/correct", "score=6-3 6-4"},
		{"proposal", "/thread/proposal", "type=date&date=2026-10-10"},
		{"GET thread", "/thread", ""},
		{"GET thread messages", "/thread-messages", ""},
		{"respond", "/thread/proposal/x/respond", "action=accept"},
		{"withdraw", "/thread/proposal/x/withdraw", ""},
		{"reject and counter", "/thread/proposal/x/reject-and-counter", "type=date&date=2026-10-10"},
		{"dispute resolve", "!/admin/disputes/{id}/resolve", "score=6-3 6-4"},
		{"walkover approve", "!/admin/disputes/{id}/walkover-approve", ""},
	} {
		s := &tests.ApiScenario{
			TestAppFactory:  handlers.TestAppFactory,
			Name:            "released match refuses " + c.name,
			Method:          http.MethodPost,
			ExpectedStatus:  200,
			ExpectedContent: []string{"El partido está liberado"},
		}
		if strings.HasPrefix(c.name, "GET ") {
			s.Method = http.MethodGet
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			m, _ := releasedMatch(tb, app)
			s.URL = "/match/" + m.Id + c.path
			if strings.HasPrefix(c.path, "!") {
				s.URL = strings.ReplaceAll(c.path[1:], "{id}", m.Id)
			}
			hdrs := handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
			hdrs["Content-Type"] = "application/x-www-form-urlencoded"
			s.Headers = hdrs
			if c.body != "" {
				s.Body = strings.NewReader(c.body)
			}
		}
		s.Test(t)
	}
}

func TestMatchPage_ReleasedPlayer404(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "player gets 404 on a released match",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"Partido no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		m, p1 := releasedMatch(tb, app)
		user, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestAdminRelease_RejectsPendingProposals(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "release rejects the pending result proposal as the admin",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var propID, adminID string
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PrpA")
		p2 := handlers.MakePairTB(tb, app, "PrpB")
		p3 := handlers.MakePairTB(tb, app, "PrpC")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		compID = comp.Id
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("round_number", 0)
		require.NoError(tb, app.Save(m))
		u1, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		propID = handlers.MakeResultProposal(tb, app, m.Id, u1.Id, "6-3 6-4").Id
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id
		compID = m.GetString("competition")
		s.URL = "/match/" + m.Id + "/release"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("match_messages", propID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", p.GetString("proposal_status"))
		assert.NotEmpty(tb, adminID)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return "/admin/competitions/" + compID })
	s.Test(t)
}

func TestICalMatch_Released404(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "ics of a released match is 404",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"Partido no encontrado"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		m, p1 := releasedMatch(tb, app)
		m.Set("date", "2026-12-01T10:00:00Z")
		require.NoError(tb, app.Save(m))
		user, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/ical/match/" + m.Id
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestAdminCompetition_ReleasedAccordion(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin page lists the released match in Liberados",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{`id="released-matches"`, "Liberados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		m, _ := releasedMatch(tb, app)
		s.URL = "/admin/competitions/" + m.GetString("competition")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestAdminRelease_Lifecycle(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "release marks the match, logs it and deletes its notifications",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, adminID, compID string
	handlers.ExpectRedirect(s, func(core.App) string { return "/admin/competitions/" + compID })
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "LcA")
		p2 := handlers.MakePairTB(tb, app, "LcB")
		p3 := handlers.MakePairTB(tb, app, "LcC")
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		col, err := app.FindCollectionByNameOrId("notifications")
		require.NoError(tb, err)
		n := core.NewRecord(col)
		n.Set("user", p1.GetString("player1"))
		n.Set("related_match", m.Id)
		n.Set("title", "x")
		n.Set("type", "match_assigned")
		require.NoError(tb, app.Save(n))
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id
		s.URL = "/match/" + m.Id + "/release"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, league.StatusReleased, m.GetString("status"))
		notifs, err := app.FindRecordsByFilter("notifications", "related_match = {:m}", "", 0, 0, map[string]any{"m": matchID})
		require.NoError(tb, err)
		assert.Empty(tb, notifs)
		log, err := app.FindRecordsByFilter("match_messages", "match = {:m} && type = 'admin_action'", "", 0, 0, map[string]any{"m": matchID})
		require.NoError(tb, err)
		require.Len(tb, log, 1)
		assert.Equal(tb, adminID, log[0].GetString("author"))
	}
	s.Test(t)
}

func TestAdminRelease_Refusals(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, want string
		leveled    bool
		status     string
	}{
		{"not leveled", "Solo se puede liberar un partido de una liga nivelada", false, "pending"},
		{"already released", "El partido ya está liberado", true, league.StatusReleased},
	} {
		s := &tests.ApiScenario{
			TestAppFactory:  handlers.TestAppFactory,
			Name:            "release refused: " + c.name,
			Method:          http.MethodPost,
			ExpectedStatus:  200,
			ExpectedContent: []string{c.want},
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupProductionRoutes(tb, app, e)
			p1 := handlers.MakePairTB(tb, app, "RfA")
			p2 := handlers.MakePairTB(tb, app, "RfB")
			p3 := handlers.MakePairTB(tb, app, "RfC")
			var comp *core.Record
			if c.leveled {
				comp = makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
			} else {
				comp = handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			}
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, c.status)
			s.URL = "/match/" + m.Id + "/release"
			s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
		}
		s.Test(t)
	}
}
