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
)

func findResultEvents(app *tests.TestApp, matchID string) []*core.Record {
	recs, _ := app.FindRecordsByFilter("match_messages",
		"match = {:id} && type = 'result_event'", "", 0, 0,
		map[string]any{"id": matchID})
	return recs
}

func findTimelineEntries(app *tests.TestApp, matchID, entryType string) []*core.Record {
	recs, _ := app.FindRecordsByFilter("match_messages",
		"match = {:id} && type = {:type}", "", 0, 0,
		map[string]any{"id": matchID, "type": entryType})
	return recs
}

func TestSubmitCreatesTimelineEntry(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "score submit writes result_submission timeline entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TL Sub A")
		p2 := handlers.MakePairTB(tb, app, "TL Sub B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		playerID = p1.GetString("player1")
		player, err := app.FindRecordById("users", playerID)
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, player)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		entries := findTimelineEntries(app, matchID, "result_submission")
		require.Len(tb, entries, 1, "submit must write one result_submission")
		assert.Equal(tb, playerID, entries[0].GetString("author"))
		assert.Equal(tb, "6-3 6-4", entries[0].GetString("content"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestCorrectCreatesTimelineEntry(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "correct writes result_submission timeline entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	var correctorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TL Corr A")
		p2 := handlers.MakePairTB(tb, app, "TL Corr B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		correctorID = p1.GetString("player1")
		m.Set("submitted_by", correctorID)
		m.Set("submitted_at", "2099-01-01 00:00:00.000Z")
		require.NoError(tb, app.Save(m))
		handlers.MakeResultProposal(tb, app, m.Id, correctorID, "6-3 6-4")
		matchID = m.Id
		player, err := app.FindRecordById("users", correctorID)
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		hdrs := handlers.AuthHeaders(tb, player)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pending, _ := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"-created", 0, 0, map[string]any{"mid": matchID})
		require.Len(tb, pending, 1, "correct must leave one pending result_submission")
		assert.Equal(tb, correctorID, pending[0].GetString("author"))
		assert.Equal(tb, "6-4 6-3", pending[0].GetString("content"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestReportUnplayedCreatesTimelineEntry(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "report unplayed writes result_event timeline entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	var reporterID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TL Unp A")
		p2 := handlers.MakePairTB(tb, app, "TL Unp B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		reporterID = p1.GetString("player1")
		player, err := app.FindRecordById("users", reporterID)
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/report-unplayed"
		s.Body = strings.NewReader("reason=rival+no+show")
		hdrs := handlers.AuthHeaders(tb, player)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		entries := findResultEvents(app, matchID)
		require.Len(tb, entries, 1, "report-unplayed must write one result_event")
		assert.Equal(tb, reporterID, entries[0].GetString("author"))
		assert.Contains(tb, entries[0].GetString("content"), "reportó el partido como no jugado")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestDisputeResolveCreatesTimelineEntry(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "dispute resolve writes result_event timeline entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "TL Res A")
		p2 := handlers.MakePairTB(tb, app, "TL Res B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		m.Set("disputed_by", p2.GetString("player1"))
		m.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/admin/disputes/" + m.Id + "/resolve"
		s.Body = strings.NewReader("score=6-4+6-3")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		entries := findResultEvents(app, matchID)
		require.Len(tb, entries, 1, "dispute resolve must write one result_event")
		assert.Contains(tb, entries[0].GetString("content"), "resolvió la disputa")
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

func TestWalkoverApproveCreatesTimelineEntry(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "walkover approve writes result_event timeline entry",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "TL WO A")
		p2 := handlers.MakePairTB(tb, app, "TL WO B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m.Set("review_type", "walkover")
		m.Set("walkover_requested_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/admin/disputes/" + m.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p2.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		entries := findResultEvents(app, matchID)
		require.Len(tb, entries, 1, "walkover approve must write one result_event")
		assert.Contains(tb, entries[0].GetString("content"), "aprobó incomparecencia")
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}
