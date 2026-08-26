package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// ═══════════════════════════════════════════════════════════════════════
// Group 1: Dashboard summary counts (lines 84-88)
// Survivors: playedMatches++, disputeCount++, status checks
// ═══════════════════════════════════════════════════════════════════════

func TestDashboardSummaryCounts(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin dashboard shows correct summary counts",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Test Comp"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Dash A")
		p2 := makePairTB(tb, app, "Dash B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Test Comp")
		require.NoError(tb, app.Save(comp))
		m1 := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m1.Set("winner", p1.Id)
		m1.Set("scores", "6-3 6-4")
		require.NoError(tb, app.Save(m1))
		m2 := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m2.Set("scores", "6-3 6-4")
		m2.Set("dispute_notes", "wrong score")
		require.NoError(tb, app.Save(m2))
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		b := string(body)
		assert.Contains(tb, b, `value="1"`, "progress bar value must be exactly 1")
		assert.Contains(tb, b, "1 disputa", "exactly 1 dispute expected")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 2: Quorum issue detection (lines 150-168)
// ═══════════════════════════════════════════════════════════════════════

func TestDashboardQuorumIssue(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin shows quorum issue for expired submission",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Quorum", "enviado hace"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Q A")
		p2 := makePairTB(tb, app, "Q B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 24)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))
		sa := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
		_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
			Bind(map[string]any{"sa": sa, "id": m.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDashboardQuorumNoIssueWhenFresh(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /admin no quorum issue when submission is recent",
		Method:             http.MethodGet,
		URL:                "/admin",
		ExpectedStatus:     200,
		NotExpectedContent: []string{"Quorum"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "QF A")
		p2 := makePairTB(tb, app, "QF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 24)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))
		sa := time.Now().Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
		_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
			Bind(map[string]any{"sa": sa, "id": m.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDashboardQuorumZeroHoursNoIssue(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /admin no quorum issue when quorum_timeout_hours is 0",
		Method:             http.MethodGet,
		URL:                "/admin",
		ExpectedStatus:     200,
		NotExpectedContent: []string{"Quorum"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "QZ A")
		p2 := makePairTB(tb, app, "QZ B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 0)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))
		sa := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
		_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
			Bind(map[string]any{"sa": sa, "id": m.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDashboardQuorumShowsHoursWhenLessThanDay(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin quorum shows hours when less than 1 day",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Quorum"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "QH A")
		p2 := makePairTB(tb, app, "QH B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("quorum_timeout_hours", 2)
		require.NoError(tb, app.Save(comp))
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(m))
		sa := time.Now().Add(-6 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
		_, err := app.DB().NewQuery("UPDATE matches SET submitted_at = {:sa} WHERE id = {:id}").
			Bind(map[string]any{"sa": sa, "id": m.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		b := string(body)
		assert.Contains(tb, b, "enviado hace 6 horas",
			"quorum issue must show hours not days when elapsed < 24h")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 3: Pending issue detection (lines 178-210)
// Overdue + stale detection
// ═══════════════════════════════════════════════════════════════════════

func TestDashboardOverdueMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin shows overdue issue for past-dated pending match",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Vencido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "OD A")
		p2 := makePairTB(tb, app, "OD B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2020-01-01")
		require.NoError(tb, app.Save(m))
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDashboardNotOverdueForFutureDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /admin no overdue for future-dated pending match",
		Method:             http.MethodGet,
		URL:                "/admin",
		ExpectedStatus:     200,
		NotExpectedContent: []string{"Vencido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "NF A")
		p2 := makePairTB(tb, app, "NF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2099-12-31")
		require.NoError(tb, app.Save(m))
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestDashboardStaleMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin shows stale issue for inactive pending match",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Inactivo", "sin actividad"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "ST A")
		p2 := makePairTB(tb, app, "ST B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		// Create a message older than 14 days
		msgCol, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(msgCol)
		msg.Set("match", m.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("content", "old message")
		msg.Set("type", "chat")
		require.NoError(tb, app.Save(msg))
		// Override created via raw SQL — autodate fields ignore SetRaw on save
		oldCreated := time.Now().Add(-15 * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
		_, err = app.DB().NewQuery("UPDATE match_messages SET created = {:c} WHERE id = {:id}").
			Bind(map[string]any{"c": oldCreated, "id": msg.Id}).Execute()
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 4: Detail page — IsLeague, HasFixtures, standings
// ═══════════════════════════════════════════════════════════════════════

func TestDetailPageLeagueVsPlayoff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		compType string
		want     string
		deny     string
	}{
		{"league shows standings", "league", "Clasificación", ""},
		{"playoff no standings", "playoff", "", "Clasificación"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content []string
			var notContent []string
			if tc.want != "" {
				content = []string{tc.want}
			}
			if tc.deny != "" {
				notContent = []string{tc.deny}
			}
			s := &tests.ApiScenario{
				TestAppFactory:     testAppFactory,
				Name:               tc.name,
				Method:             http.MethodGet,
				ExpectedStatus:     200,
				ExpectedContent:    content,
				NotExpectedContent: notContent,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)
				admin := makeAdminUserTB(tb, app)
				p1 := makePairTB(tb, app, "DT A")
				p2 := makePairTB(tb, app, "DT B")
				comp := makeCompetitionTB(tb, app, tc.compType, []*core.Record{p1, p2})
				s.URL = "/admin/competitions/" + comp.Id
				s.Headers = authHeaders(tb, admin)
			}
			s.Test(t)
		})
	}
}

func TestDetailPageHasFixtures(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "detail page with fixtures shows match data",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "HF A")
		p2 := makePairTB(tb, app, "HF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 5: buildDisputeViews (line 345)
// ═══════════════════════════════════════════════════════════════════════

func TestDetailPageShowsDisputes(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "detail page shows disputed matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Disputa"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "DP A")
		p2 := makePairTB(tb, app, "DP B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", p1.GetString("player1"))
		m.Set("disputed_by", p2.GetString("player1"))
		m.Set("dispute_notes", "wrong score")
		require.NoError(tb, app.Save(m))
		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 6+7: Create and Update competition
// ═══════════════════════════════════════════════════════════════════════

func TestCreateCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions creates with all fields",
		Method:         http.MethodPost,
		URL:            "/admin/competitions",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("name=Nueva+Liga&type=league&category=A&active=on&play_twice=on&quorum_timeout_hours=48")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions",
			"name = 'Nueva Liga'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		c := comps[0]
		assert.True(tb, c.GetBool("active"), "active must be true")
		assert.True(tb, c.GetBool("play_twice"), "play_twice must be true")
		assert.Equal(tb, "league", c.GetString("type"))
		assert.Equal(tb, float64(48), c.GetFloat("quorum_timeout_hours"))
	}
	s.Test(t)
}

func TestCreateCompetitionInactive(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions without active=on creates inactive",
		Method:         http.MethodPost,
		URL:            "/admin/competitions",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Body = strings.NewReader("name=Inactive+Comp&type=league&category=A")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions",
			"name = 'Inactive Comp'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		assert.False(tb, comps[0].GetBool("active"), "active must be false when not sent")
		assert.False(tb, comps[0].GetBool("play_twice"), "play_twice must be false when not sent")
	}
	s.Test(t)
}

func TestUpdateCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id} updates fields",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Updated&type=playoff&category=B&play_twice=on&quorum_timeout_hours=72&default_penalty=5")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "Updated", c.GetString("name"))
		assert.Equal(tb, "playoff", c.GetString("type"))
		assert.True(tb, c.GetBool("play_twice"))
		assert.Equal(tb, float64(72), c.GetFloat("quorum_timeout_hours"))
		assert.Equal(tb, float64(5), c.GetFloat("default_penalty"))
	}
	s.Test(t)
}

func TestUpdateCompetitionPlayTwiceOff(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id} without play_twice sets false",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		comp.Set("play_twice", true)
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=NoTwice&type=league&category=A")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.False(tb, c.GetBool("play_twice"), "play_twice must be false when checkbox not sent")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 8: Toggle active
// ═══════════════════════════════════════════════════════════════════════

func TestToggleCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/toggle flips active",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/toggle"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.False(tb, c.GetBool("active"), "toggle should flip active from true to false")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 9: AddPair with seed
// ═══════════════════════════════════════════════════════════════════════

func TestAddPairWithSeed(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/pairs adds pair with seed",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, newPairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AP A")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1})
		compID = comp.Id
		newPair := makePairTB(tb, app, "AP B")
		newPairID = newPair.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + newPair.Id + "&seed=3")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, newPairID, "pair must be enrolled")
		var seeding map[string]int
		require.NoError(tb, c.UnmarshalJSONField("seeding", &seeding))
		assert.Equal(tb, 3, seeding[newPairID], "seed must be stored")
	}
	s.Test(t)
}

func TestAddPairWithSeedZero(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/pairs seed=0 skips seeding",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, newPairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AZ A")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1})
		compID = comp.Id
		newPair := makePairTB(tb, app, "AZ B")
		newPairID = newPair.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + newPair.Id + "&seed=0")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, newPairID, "pair must be enrolled")
		var seeding map[string]int
		require.NoError(tb, c.UnmarshalJSONField("seeding", &seeding))
		_, hasSeed := seeding[newPairID]
		assert.False(tb, hasSeed, "seed=0 must not store a seeding entry")
	}
	s.Test(t)
}

func TestAddPairDuplicate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects duplicate pair",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"jugadores duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AD A")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 10: RemovePair cleans up seeding + payment
// ═══════════════════════════════════════════════════════════════════════

func TestRemovePairCleansUpSeedingAndPayment(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST remove-pair cleans up seeding and payment_status",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, removedID, keptID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "RM A")
		p2 := makePairTB(tb, app, "RM B")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		comp.Set("seeding", map[string]int{p1.Id: 1, p2.Id: 2})
		comp.Set("payment_status", map[string]bool{p1.Id: true, p2.Id: true})
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		removedID = p1.Id
		keptID = p2.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.NotContains(tb, pairs, removedID)
		assert.Contains(tb, pairs, keptID)
		var seeding map[string]int
		require.NoError(tb, c.UnmarshalJSONField("seeding", &seeding))
		_, hasRemoved := seeding[removedID]
		assert.False(tb, hasRemoved, "removed pair's seed must be deleted")
		assert.Equal(tb, 2, seeding[keptID], "kept pair's seed must remain")
		var payment map[string]bool
		require.NoError(tb, c.UnmarshalJSONField("payment_status", &payment))
		_, hasRemovedPay := payment[removedID]
		assert.False(tb, hasRemovedPay, "removed pair's payment must be deleted")
		assert.True(tb, payment[keptID], "kept pair's payment must remain")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 11: CopyPairs
// ═══════════════════════════════════════════════════════════════════════

func TestCopyPairsWithSeeding(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST copy-pairs copies pairs and seeding for playoff",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"copiadas"},
	}
	var targetID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "CP A")
		p2 := makePairTB(tb, app, "CP B")
		source := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		source.Set("seeding", map[string]int{p1.Id: 1, p2.Id: 2})
		require.NoError(tb, app.Save(source))
		pairID = p1.Id
		target := makeCompetitionTB(tb, app, "playoff", nil)
		targetID = target.Id
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=" + source.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", targetID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, pairID)
		var seeding map[string]int
		require.NoError(tb, c.UnmarshalJSONField("seeding", &seeding))
		assert.Equal(tb, 1, seeding[pairID], "copied seeding must match source")
	}
	s.Test(t)
}

func TestCopyPairsLeagueSkipsSeeding(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST copy-pairs to league skips seeding",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"copiadas"},
	}
	var targetID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "CL A")
		source := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1})
		source.Set("seeding", map[string]int{p1.Id: 1})
		require.NoError(tb, app.Save(source))
		pairID = p1.Id
		target := makeCompetitionTB(tb, app, "league", nil)
		targetID = target.Id
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=" + source.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", targetID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, pairID)
		var seeding map[string]int
		require.NoError(tb, c.UnmarshalJSONField("seeding", &seeding))
		_, hasSeed := seeding[pairID]
		assert.False(tb, hasSeed, "league target must not copy seeding")
	}
	s.Test(t)
}

func TestCopyPairsSkipsDuplicates(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST copy-pairs skips already-enrolled pairs",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"omitidas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "CD A")
		source := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		target := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=" + source.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		assert.Contains(tb, string(body), "0 parejas copiadas")
		assert.Contains(tb, string(body), "1 omitidas")
	}
	s.Test(t)
}

func TestCopyPairsEmptySource(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST copy-pairs with empty source_competition returns error",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Selecciona una competición"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		target := makeCompetitionTB(tb, app, "league", nil)
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 12: TogglePaymentAll
// ═══════════════════════════════════════════════════════════════════════

func TestTogglePaymentAll(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST payment-all marks all pairs paid",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	var pairIDs []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "PA A")
		p2 := makePairTB(tb, app, "PA B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		pairIDs = []string{p1.Id, p2.Id}
		s.URL = "/admin/competitions/" + comp.Id + "/payment-all"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		var status map[string]bool
		require.NoError(tb, c.UnmarshalJSONField("payment_status", &status))
		for _, pid := range pairIDs {
			assert.True(tb, status[pid], "pair %s must be marked paid", pid)
		}
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 13: Penalty default amount
// ═══════════════════════════════════════════════════════════════════════

func TestPenaltyUsesDefaultAmount(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST penalty uses default_penalty from competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "PD A")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		comp.Set("default_penalty", 5)
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		pairID = p1.Id
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + p1.Id + "&action=apply")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		var penalties map[string]float64
		require.NoError(tb, c.UnmarshalJSONField("penalty_points", &penalties))
		assert.Equal(tb, float64(5), penalties[pairID], "must use competition's default_penalty, not hardcoded 3")
	}
	s.Test(t)
}

func TestPenaltyRemove(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST penalty action=remove deletes penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "PR A")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		comp.Set("penalty_points", map[string]float64{p1.Id: 3})
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		pairID = p1.Id
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + p1.Id + "&action=remove")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		var penalties map[string]float64
		require.NoError(tb, c.UnmarshalJSONField("penalty_points", &penalties))
		_, hasPenalty := penalties[pairID]
		assert.False(tb, hasPenalty, "penalty must be deleted after remove")
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 14: validatePlayerUniqueness
// ═══════════════════════════════════════════════════════════════════════

func TestAddPairPlayerOverlap(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects overlapping players",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"jugadores duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		// Create two pairs sharing a player
		u1 := makeUserTB(tb, app, "Overlap1", "")
		u2 := makeUserTB(tb, app, "Overlap2", "")
		u3 := makeUserTB(tb, app, "Overlap3", "")
		pairCol, _ := app.FindCollectionByNameOrId("pairs")
		pairA := core.NewRecord(pairCol)
		pairA.Set("player1", u1.Id)
		pairA.Set("player2", u2.Id)
		pairA.Set("name", "Pair OA")
		require.NoError(tb, app.Save(pairA))
		pairB := core.NewRecord(pairCol)
		pairB.Set("player1", u1.Id)
		pairB.Set("player2", u3.Id)
		pairB.Set("name", "Pair OB")
		require.NoError(tb, app.Save(pairB))
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pairA})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pairB.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 15: HasUnpaid flag on detail page
// ═══════════════════════════════════════════════════════════════════════

func TestDetailPageHasUnpaid(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "detail page shows unpaid indicator",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Marcar todos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "UP A")
		p2 := makePairTB(tb, app, "UP B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 10a: Detail page HasFixtures=false shows "Generar" not "Regenerar" (line 265)
// ═══════════════════════════════════════════════════════════════════════

func TestDetailPageNoFixturesShowsGenerateButton(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "detail page with no matches shows Generar not Regenerar",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Generar calendario"},
		NotExpectedContent: []string{"Regenerar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "NF A")
		p2 := makePairTB(tb, app, "NF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 10b: Dashboard played-count with all-final matches (lines 84-85)
// Kills CONDITIONALS_NEGATION on "status == final" → playedMatches++
// ═══════════════════════════════════════════════════════════════════════

func TestDashboardAllMatchesFinal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin all-final matches shows correct played count",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  200,
		ExpectedContent: []string{"2/2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "AF A")
		p2 := makePairTB(tb, app, "AF B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m1 := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m1.Set("winner", p1.Id)
		m1.Set("scores", "6-3 6-4")
		require.NoError(tb, app.Save(m1))
		m2 := makeMatchTB(tb, app, comp.Id, p2.Id, p1.Id, "final")
		m2.Set("winner", p2.Id)
		m2.Set("scores", "6-4 6-3")
		require.NoError(tb, app.Save(m2))
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		b := string(body)
		assert.Contains(tb, b, `value="2"`, "progress bar value must be exactly 2")
		assert.Contains(tb, b, "sin disputas", "no disputes when all matches final")
	}
	s.Test(t)
}

func TestDashboardNoDisputedMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "GET /admin no disputes when no disputed matches",
		Method:             http.MethodGet,
		URL:                "/admin",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"sin disputas"},
		NotExpectedContent: []string{"en disputa"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "ND A")
		p2 := makePairTB(tb, app, "ND B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		_ = m
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

// ═══════════════════════════════════════════════════════════════════════
// Group 11: Detail page round sort order (line 337, NOT COVERED)
// ═══════════════════════════════════════════════════════════════════════

func TestDetailRoundSortOrder(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/competitions/{id} rounds sorted ascending",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornada 1", "Jornada 2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "RS A")
		p2 := makePairTB(tb, app, "RS B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m1 := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m1.Set("round_number", 2)
		require.NoError(tb, app.Save(m1))
		m2 := makeMatchTB(tb, app, comp.Id, p2.Id, p1.Id, "pending")
		m2.Set("round_number", 1)
		require.NoError(tb, app.Save(m2))
		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body, err := io.ReadAll(res.Body)
		require.NoError(tb, err)
		b := string(body)
		idx1 := strings.Index(b, "Jornada 1")
		idx2 := strings.Index(b, "Jornada 2")
		assert.Greater(tb, idx1, -1, "Jornada 1 must appear")
		assert.Greater(tb, idx2, -1, "Jornada 2 must appear")
		assert.Less(tb, idx1, idx2, "Jornada 1 must appear before Jornada 2")
	}
	s.Test(t)
}

// TestPaymentStatusSurvivesDBRoundTrip toggles a pair's payment, then
// re-reads the competition from the database and verifies the status
// persists. The bug: getPaymentStatus used a type switch that didn't
// handle types.JSONRaw, so after a DB round-trip the payment map was
// always empty.
func TestPaymentStatusSurvivesDBRoundTrip(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/payment persists after DB round-trip",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "Pay A")
		p2 := makePairTB(tb, app, "Pay B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		pairID = p1.Id
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)

		var status map[string]bool
		require.NoError(tb, comp.UnmarshalJSONField("payment_status", &status))
		assert.True(tb, status[pairID],
			"pair must be marked paid after toggle and DB round-trip")
	}
	s.Test(t)
}

// TestPenaltyMapSurvivesDBRoundTrip sets a penalty, re-reads from DB,
// verifies getPenaltyMap returns the correct value.
func TestPenaltyMapSurvivesDBRoundTrip(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/penalty persists after DB round-trip",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "Pen A")
		p2 := makePairTB(tb, app, "Pen B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		pairID = p1.Id
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + p1.Id + "&action=apply")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)

		var penalties map[string]float64
		require.NoError(tb, comp.UnmarshalJSONField("penalty_points", &penalties))
		assert.Equal(tb, float64(3), penalties[pairID],
			"penalty must persist after DB round-trip")
	}
	s.Test(t)
}

func TestAdminCompetitionDetailWithData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/competitions/{id} with matches and disputes",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Detail A", "Detail B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Detail A")
		p2 := makePairTB(tb, app, "Detail B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Pending match
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Confirmed match (stale)
		confirmed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		confirmed.Set("scores", "6-3 6-4")
		confirmed.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(confirmed))

		// Disputed match
		disputed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		disputed.Set("scores", "6-3 6-4")
		disputed.Set("dispute_notes", "Score wrong")
		require.NoError(tb, app.Save(disputed))

		// Final match
		col, _ := app.FindCollectionByNameOrId("matches")
		final := core.NewRecord(col)
		final.Set("competition", comp.Id)
		final.Set("pair1", p1.Id)
		final.Set("pair2", p2.Id)
		final.Set("status", "final")
		final.Set("scores", "6-2 6-1")
		final.Set("winner", p1.Id)
		final.Set("round_number", 1)
		require.NoError(tb, app.Save(final))

		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminCompDetailWithPenalties(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/competitions/{id} with penalties/seeding/payment",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Pen A", "Pen B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Pen A")
		p2 := makePairTB(tb, app, "Pen B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		comp.Set("penalty_points", map[string]any{p1.Id: 1.0})
		comp.Set("payment_status", map[string]any{p1.Id: true, p2.Id: false})
		comp.Set("seeding", map[string]any{p1.Id: 1, p2.Id: 2})
		comp.Set("quorum_timeout_hours", 48)
		require.NoError(tb, app.Save(comp))

		// Add matches at different statuses to cover classifyMatchIssues
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		confirmed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		confirmed.Set("scores", "6-3 6-4")
		confirmed.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(confirmed))

		disputed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		disputed.Set("scores", "6-3 6-4")
		disputed.Set("dispute_notes", "Wrong score")
		require.NoError(tb, app.Save(disputed))

		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminCopyPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/copy-pairs copies pairs",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"copiadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "CopyA")
		p2 := makePairTB(tb, app, "CopyB")
		source := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		target := makeCompetitionTB(tb, app, "league", nil)
		s.URL = "/admin/competitions/" + target.Id + "/copy-pairs"
		s.Body = strings.NewReader("source_competition=" + source.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminTogglePaymentAll(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/payment-all marks all paid",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "PayAllA")
		p2 := makePairTB(tb, app, "PayAllB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		s.URL = "/admin/competitions/" + comp.Id + "/payment-all"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestUpdateRoundDates(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/round-dates saves edited dates",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "RdA")
		p2 := makePairTB(tb, app, "RdB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("rounds", 2)
		comp.Set("round_arrange_dates", league.StoreRoundSchedule(
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 2))
		require.NoError(tb, app.Save(comp))
		compID = comp.Id

		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/admin/competitions/" + comp.Id + "/round-dates"
		s.Body = strings.NewReader("round_date_1=2026-06-20&round_date_2=2026-06-28")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		raw := comp.GetString("round_arrange_dates")
		var stored map[string]time.Time
		require.NoError(tb, json.Unmarshal([]byte(raw), &stored))
		assert.Equal(tb, time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), stored["1"])
		assert.Equal(tb, time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC), stored["2"])
	}
	s.Test(t)
}

func TestRegenerateRoundDates(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/round-dates/regenerate overwrites",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "RgA")
		p2 := makePairTB(tb, app, "RgB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("rounds", 2)
		comp.Set("round_arrange_dates", `{"1":"2099-01-01T00:00:00Z","2":"2099-06-01T00:00:00Z"}`)
		require.NoError(tb, app.Save(comp))
		compID = comp.Id

		s.URL = "/admin/competitions/" + comp.Id + "/round-dates/regenerate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		raw := comp.GetString("round_arrange_dates")
		var stored map[string]time.Time
		require.NoError(tb, json.Unmarshal([]byte(raw), &stored))
		assert.False(tb, stored["1"].Year() == 2099, "regenerate must overwrite the old 2099 date")
	}
	s.Test(t)
}
