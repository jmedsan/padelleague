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

func TestAdminCompDetailWithPenalties(t *testing.T) {
	s := &tests.ApiScenario{
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

func TestAdminOverrideWithDateChange(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /match/{id}/admin-override changes date and time",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrDate A")
		p2 := makePairTB(tb, app, "OvrDate B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = m.Id
		m.Set("date", "2026-09-01")
		m.Set("time", "18:00")
		m.Set("club", "Old Club")
		require.NoError(tb, app.Save(m))

		venue := makeVenueTB(tb, app, "New Club")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("date=2026-09-15&time=20:00&venue_id=" + venue.Id)
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "20:00", m.GetString("time"))
		assert.Equal(tb, "New Club", m.GetString("club"))
	}
	s.Test(t)
}

func TestAdminOverrideNoChanges(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/admin-override with no changes warns",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-warning"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrNone A")
		p2 := makePairTB(tb, app, "OvrNone B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("")
		admin := makeAdminUserTB(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOverrideNonAdmin(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/admin-override as player fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Solo administradores"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OvrNoAdm A")
		p2 := makePairTB(tb, app, "OvrNoAdm B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/match/" + m.Id + "/admin-override"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestForgotPasswordSubmitWithUser(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /forgot-password with existing user",
		Method:          http.MethodPost,
		URL:             "/forgot-password",
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		makeUserTB(tb, app, "Reset User", "resetuser@test.local")
		s.Body = strings.NewReader("email=resetuser@test.local")
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestMatchConfirmSameTeam(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/confirm by submitter team fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No puedes confirmar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SameConf A")
		p2 := makePairTB(tb, app, "SameConf B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		submitter := p1.GetString("player1")
		m.Set("scores", "6-3 6-4")
		m.Set("submitted_by", submitter)
		require.NoError(tb, app.Save(m))

		s.URL = "/match/" + m.Id + "/confirm"
		user, _ := app.FindRecordById("users", submitter)
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestMatchWalkoverWrongStatus(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/walkover on non-pending fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "WOStatus A")
		p2 := makePairTB(tb, app, "WOStatus B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")

		s.URL = "/match/" + m.Id + "/walkover"
		s.Body = strings.NewReader("absent_team=2")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestGeneratePlayoffFixtures(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/generate for playoff",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PO A")
		p2 := makePairTB(tb, app, "PO B")
		p3 := makePairTB(tb, app, "PO C")
		p4 := makePairTB(tb, app, "PO D")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2, p3, p4})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:comp}", "", 0, 0,
			map[string]any{"comp": compID})
		require.NoError(tb, err)
		assert.Equal(tb, 3, len(matches))
	}
	s.Test(t)
}

func TestGenerateLeagueFixtures(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST /admin/competitions/{id}/generate for league",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "LG A")
		p2 := makePairTB(tb, app, "LG B")
		p3 := makePairTB(tb, app, "LG C")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:comp}", "", 0, 0,
			map[string]any{"comp": compID})
		require.NoError(tb, err)
		assert.Equal(tb, 3, len(matches))
	}
	s.Test(t)
}

func TestGenerateFixturesRegenerate(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /admin/competitions/{id}/generate with existing matches warns",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-warning"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Regen A")
		p2 := makePairTB(tb, app, "Regen B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestMatchSubmitAlreadyScored(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /match/{id}/submit on non-pending fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya tiene"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SubDup A")
		p2 := makePairTB(tb, app, "SubDup B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")

		s.URL = "/match/" + m.Id + "/submit"
		s.Body = strings.NewReader("scores=6-3+6-4")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := authHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}
