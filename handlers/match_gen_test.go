package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score on match with no prior score (lines 319-320)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AO A")
		p2 := makePairTB(tb, app, "AO B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, "final", m.GetString("status"))

		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Resultado establecido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Resultado establecido'")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: score correction (existing score → new score) (line 319 negation)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideCorrectedScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override corrects existing score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AC A")
		p2 := makePairTB(tb, app, "AC B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-4+6-3")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Resultado corregido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Resultado corregido'")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: venue set on match with no prior venue (lines 350-351)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewVenue(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new venue",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AV A")
		p2 := makePairTB(tb, app, "AV B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		venue := makeVenueTB(tb, app, "Padel Test")
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("venue_id=" + venue.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "Padel Test", m.GetString("club"))

		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Club establecido") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Club establecido'")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// AdminOverride: date set on match with no prior date (line 331)
// ═══════════════════════════════════════════════════════════════════════

func TestAdminOverrideNewDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/admin-override sets new date",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AD A")
		p2 := makePairTB(tb, app, "AD B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-01")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msgs, _ := app.FindRecordsByFilter("match_messages",
			"match = {:id} && type = 'admin_action'", "", 0, 0,
			map[string]any{"id": matchID})
		require.GreaterOrEqual(tb, len(msgs), 1)
		found := false
		for _, msg := range msgs {
			if strings.Contains(msg.GetString("content"), "Fecha establecida") {
				found = true
			}
		}
		assert.True(tb, found, "timeline must contain 'Fecha establecida'")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// buildShareText: final match shows correct winner (lines 364, 371)
// ═══════════════════════════════════════════════════════════════════════

func TestBuildShareTextFinalMatch(t *testing.T) {
	t.Parallel()
	p1Name := "Pair Alpha"
	p2Name := "Pair Beta"
	pairNames := map[string]string{"p1id": p1Name, "p2id": p2Name}

	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)

	t.Run("pair1 wins", func(t *testing.T) {
		t.Parallel()
		m := core.NewRecord(col)
		m.Set("status", "final")
		m.Set("pair1", "p1id")
		m.Set("pair2", "p2id")
		m.Set("scores", "6-3 6-4")
		m.Set("winner", "p1id")
		text := buildShareText(m, pairNames)
		assert.NotEmpty(t, text)
		assert.Contains(t, text, "Pair+Alpha")
		assert.Contains(t, text, "Ganador")
		assert.Contains(t, text, "Pair+Alpha%21")
	})

	t.Run("pair2 wins", func(t *testing.T) {
		t.Parallel()
		m := core.NewRecord(col)
		m.Set("status", "final")
		m.Set("pair1", "p1id")
		m.Set("pair2", "p2id")
		m.Set("scores", "3-6 4-6")
		m.Set("winner", "p2id")
		text := buildShareText(m, pairNames)
		assert.Contains(t, text, "Pair+Beta%21")
		assert.NotContains(t, text, "Pair+Alpha%21")
	})

	t.Run("non-final returns empty", func(t *testing.T) {
		t.Parallel()
		m := core.NewRecord(col)
		m.Set("status", "pending")
		m.Set("pair1", "p1id")
		m.Set("pair2", "p2id")
		text := buildShareText(m, pairNames)
		assert.Empty(t, text)
	})
}

// ═══════════════════════════════════════════════════════════════════════
// MatchSubmit: rival notification goes to correct team (line 226)
// ═══════════════════════════════════════════════════════════════════════

func TestMatchSubmitNotifiesRival(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /match/{id}/submit notifies opponent pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pair2Player1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Sub A")
		p2 := makePairTB(tb, app, "Sub B")
		pair2Player1ID = p2.GetString("player1")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		submitter, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		hdrs := authHeaders(tb, submitter)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, _ := app.FindRecordsByFilter("notifications",
			"user = {:uid}", "", 0, 0,
			map[string]any{"uid": pair2Player1ID})
		assert.GreaterOrEqual(tb, len(notifs), 1,
			"rival player must receive a notification")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// MatchDetail: competition name shown (lines 156, 158)
// ═══════════════════════════════════════════════════════════════════════

func TestMatchDetailShowsCompName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /match/{id} shows competition name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Visible"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "MD A")
		p2 := makePairTB(tb, app, "MD B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Liga Visible")
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + m.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// playerNameIfSet: empty returns empty, non-empty returns name (line 357)
// ═══════════════════════════════════════════════════════════════════════

func TestPlayerNameIfSet(t *testing.T) {
	t.Parallel()
	app, err := tests.NewTestApp(tmplDataDir)
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	t.Run("empty returns empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", playerNameIfSet(app, ""))
	})

	t.Run("valid user returns name", func(t *testing.T) {
		t.Parallel()
		u := makeUserTB(t, app, "NameTest", "nametest@test.local")
		name := playerNameIfSet(app, u.Id)
		assert.Equal(t, "NameTest", name)
	})
}
