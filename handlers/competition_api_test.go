package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

func setupCompRoutes(_ testing.TB, app *tests.TestApp, e *core.ServeEvent) {
	viewsFS := os.DirFS("..")
	r := render.New(viewsFS, "")
	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)

	e.Router.BindFunc(middleware.CookieAuth)

	auth := NewAuthHandler(app, r.Page)
	e.Router.GET("/login", auth.Login)

	comp := NewCompetitionHandler(app, svc, r.Page)
	fixture := NewFixtureHandler(app, svc, r.Page)
	dispute := NewDisputeHandler(app, notifier, r.Page)

	g := e.Router.Group("/admin")
	g.BindFunc(requireAuthTest)
	g.BindFunc(requireAdminTest)
	g.GET("", comp.Dashboard)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions", comp.Create)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/pairs", comp.AddPair)
	g.POST("/competitions/{id}/remove-pair", comp.RemovePair)
	g.POST("/competitions/{id}/payment", comp.TogglePayment)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/disputes/{id}/resolve", dispute.DisputesResolve)
	g.POST("/disputes/{id}/walkover-approve", dispute.WalkoverApprove)
}

func TestCompUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id} updates competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Updated&type=league")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "Updated", c.GetString("name"))
	}
	s.Test(t)
}

func TestCompCreateSchedulingFields(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions persists scheduling fields",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=SchedTest&type=league&start_date=2026-09-01&end_date=2026-12-15&arrange_grace_days=5&auto_flag=on&walkover_score=6-1+6-1&default_penalty=7")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'SchedTest'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		c := comps[0]
		assert.Contains(tb, c.GetString("start_date"), "2026-09-01")
		assert.Contains(tb, c.GetString("end_date"), "2026-12-15")
		assert.Equal(tb, 5.0, c.GetFloat("arrange_grace_days"))
		assert.Equal(tb, true, c.GetBool("auto_flag"))
		assert.Equal(tb, "6-1 6-1", c.GetString("walkover_score"))
		assert.Equal(tb, 7.0, c.GetFloat("default_penalty"))
	}
	s.Test(t)
}

func TestCompCreateSchedulingDefaults(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions applies scheduling defaults",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=DefTest&type=league")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'DefTest'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		c := comps[0]
		assert.Equal(tb, 3.0, c.GetFloat("arrange_grace_days"))
		assert.Equal(tb, false, c.GetBool("auto_flag"))
		assert.Equal(tb, "6-0 6-0", c.GetString("walkover_score"))
		assert.Equal(tb, 3.0, c.GetFloat("default_penalty"))
	}
	s.Test(t)
}

func TestCompUpdateSchedulingFields(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id} updates scheduling fields",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Sched&type=league&start_date=2026-10-01&end_date=2026-11-30&arrange_grace_days=2&auto_flag=on&walkover_score=6-2+6-2&default_penalty=4")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, c.GetString("start_date"), "2026-10-01")
		assert.Contains(tb, c.GetString("end_date"), "2026-11-30")
		assert.Equal(tb, 2.0, c.GetFloat("arrange_grace_days"))
		assert.Equal(tb, true, c.GetBool("auto_flag"))
		assert.Equal(tb, "6-2 6-2", c.GetString("walkover_score"))
		assert.Equal(tb, 4.0, c.GetFloat("default_penalty"))
	}
	s.Test(t)
}

func TestCompToggle(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/toggle toggles active",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/toggle"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, false, c.GetBool("active"))
	}
	s.Test(t)
}

func TestCompAddPair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/pairs adds pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		pair := makePairTB(tb, app, "NewPair")
		pairID = pair.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, pairID)
	}
	s.Test(t)
}

func TestCompRemovePair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/remove-pair removes pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "RemPair")
		pairID = pair.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.NotContains(tb, pairs, pairID)
	}
	s.Test(t)
}

func TestCompTogglePayment(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/payment toggles payment",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "PayPair")
		pairID = pair.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		raw := c.Get("payment_status")
		b, _ := json.Marshal(raw)
		var status map[string]bool
		require.NoError(tb, json.Unmarshal(b, &status))
		assert.Equal(tb, true, status[pairID], "pair must be marked as paid")
	}
	s.Test(t)
}

func TestCompApplyPenalty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/penalty applies penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "PenPair")
		pairID = pair.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + pair.Id + "&action=apply")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		raw := c.Get("penalty_points")
		b, _ := json.Marshal(raw)
		var penalties map[string]float64
		require.NoError(tb, json.Unmarshal(b, &penalties))
		assert.Equal(tb, float64(3), penalties[pairID], "default penalty must be 3")
	}
	s.Test(t)
}

func TestCompAddPairDuplicateRejectedByUniqueness(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects re-adding same pair",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "DupPair")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompAddPairOverlappingPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects overlapping player",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)

		u1 := makeUserTB(tb, app, "Overlap1", "")
		u2 := makeUserTB(tb, app, "Overlap2", "")
		u3 := makeUserTB(tb, app, "Overlap3", "")

		col, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)
		pair1 := core.NewRecord(col)
		pair1.Set("name", "PairA")
		pair1.Set("player1", u1.Id)
		pair1.Set("player2", u2.Id)
		require.NoError(tb, app.Save(pair1))

		pair2 := core.NewRecord(col)
		pair2.Set("name", "PairB")
		pair2.Set("player1", u1.Id)
		pair2.Set("player2", u3.Id)
		require.NoError(tb, app.Save(pair2))

		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pair1})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair2.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompRemovePairCleansUpMetadata(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/remove-pair cleans seeding and payment",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "RemMetaA")
		p2 := makePairTB(tb, app, "RemMetaB")
		pairID = p1.Id
		p2ID = p2.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, c.GetStringSlice("pairs"), pairID)
		assert.Contains(tb, c.GetStringSlice("pairs"), p2ID, "other pair must remain")
	}
	s.Test(t)
}

func TestCompGenerateFixtures(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/generate creates fixtures",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "GenA")
		p2 := makePairTB(tb, app, "GenB")
		p3 := makePairTB(tb, app, "GenC")
		p4 := makePairTB(tb, app, "GenD")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-09-01 00:00:00.000Z")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": compID})
		require.NoError(tb, err)
		assert.Greater(tb, len(matches), 0)

		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Greater(tb, comp.GetInt("rounds"), 0, "GenerateFixtures must persist rounds")
		assert.NotEmpty(tb, comp.GetString("round_arrange_dates"), "GenerateFixtures must persist round schedule")
	}
	s.Test(t)
}

func TestGenerateFixturesTooFewPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/competitions/{id}/generate with 1 pair rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"al menos 2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "FewA")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestGeneratePlayoffWithByes(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/competitions/{id}/generate playoff with 3 teams creates byes",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "ByeA")
		p2 := makePairTB(tb, app, "ByeB")
		p3 := makePairTB(tb, app, "ByeC")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2, p3})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": compID})
		require.NoError(tb, err)
		r1 := 0
		r2 := 0
		for _, m := range matches {
			switch m.GetInt("round_number") {
			case 1:
				r1++
			case 2:
				r2++
			}
		}
		assert.Equal(tb, 1, r1, "round 1 should have 1 match (top seed gets bye)")
		assert.Equal(tb, 1, r2, "round 2 (final) should have 1 match")
		for _, m := range matches {
			if m.GetInt("round_number") == 2 {
				assert.NotEmpty(tb, m.GetString("pair1"), "final round pair1 must be pre-set from bye")
			}
		}
	}
	s.Test(t)
}

func TestDisputeResolve(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve resolves dispute",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupCompRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makePairTB(tb, app, "DispA")
		p2 := makePairTB(tb, app, "DispB")
		p1ID = p1.Id
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=6-3+6-4&winner=" + p1.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	s.Test(t)
}
