package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/hooks"
	"padelleague/league"
	"padelleague/notify"
)

func TestCompUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id} updates competition",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Updated&type=league")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "Updated", c.GetString("name"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompCreateSchedulingFields(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions persists scheduling fields",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=SchedTest&type=league&start_date=2026-09-01&end_date=2026-12-15&arrange_grace_days=5&walkover_score=6-1+6-1&default_penalty=7")
		hdrs := handlers.AuthHeaders(tb, admin)
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
		assert.Equal(tb, "6-1 6-1", c.GetString("walkover_score"))
		assert.Equal(tb, 7.0, c.GetFloat("default_penalty"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + newestCompetitionID(app) })
	s.Test(t)
}

func TestCompCreateSchedulingDefaults(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions applies scheduling defaults",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=DefTest&type=league")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'DefTest'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		c := comps[0]
		assert.Equal(tb, 3.0, c.GetFloat("arrange_grace_days"))
		assert.Equal(tb, "6-0 6-0", c.GetString("walkover_score"))
		assert.Equal(tb, 3.0, c.GetFloat("default_penalty"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + newestCompetitionID(app) })
	s.Test(t)
}

func TestCompUpdateSchedulingFields(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id} updates scheduling fields",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Sched&type=league&start_date=2026-10-01&end_date=2026-11-30&arrange_grace_days=2&walkover_score=6-2+6-2&default_penalty=4")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, c.GetString("start_date"), "2026-10-01")
		assert.Contains(tb, c.GetString("end_date"), "2026-11-30")
		assert.Equal(tb, 2.0, c.GetFloat("arrange_grace_days"))
		assert.Equal(tb, "6-2 6-2", c.GetString("walkover_score"))
		assert.Equal(tb, 4.0, c.GetFloat("default_penalty"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompCreateInvalidWalkoverScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions rejects invalid walkover_score",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"incomparecencia inválido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=BadWO&type=league&walkover_score=abc")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompUpdateInvalidWalkoverScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id} rejects invalid walkover_score",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"incomparecencia inválido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=Updated&type=league&walkover_score=invalid")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompToggle(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/toggle toggles active",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/toggle"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, false, c.GetBool("active"))
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/competitions"))
	s.Test(t)
}

func TestCompAddPair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/pairs adds pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		pair := handlers.MakePairTB(tb, app, "NewPair")
		pairID = pair.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.Contains(tb, pairs, pairID)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompRemovePair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/remove-pair removes pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := handlers.MakePairTB(tb, app, "RemPair")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		pairs := c.GetStringSlice("pairs")
		assert.NotContains(tb, pairs, pairID)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompRemovePairRefusedWithMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id}/remove-pair refuses when pair has matches",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No se puede eliminar una pareja con partidos programados o jugados"},
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "RemMatchA")
		p2 := handlers.MakePairTB(tb, app, "RemMatchB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		pairID = p1.Id
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, c.GetStringSlice("pairs"), pairID, "pair must not be removed")
	}
	s.Test(t)
}

func TestCompTogglePayment(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/payment toggles payment",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := handlers.MakePairTB(tb, app, "PayPair")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
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
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompTogglePaymentStampsActorAndDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/payment records who and when",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		adminID = admin.Id
		pair := handlers.MakePairTB(tb, app, "StampPair")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		var paidAt map[string]string
		require.NoError(tb, c.UnmarshalJSONField("payment_paid_at", &paidAt))
		var paidBy map[string]string
		require.NoError(tb, c.UnmarshalJSONField("payment_paid_by", &paidBy))
		assert.NotEmpty(tb, paidAt[pairID], "payment_paid_at must be stamped")
		assert.Equal(tb, adminID, paidBy[pairID], "payment_paid_by must record the acting admin")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestCompTogglePaymentUnpaidKeepsStaleStamp documents current behavior: toggling
// a pair from paid back to unpaid does NOT clear payment_paid_at/payment_paid_by —
// only the paid path re-stamps them. This is safe because the admin template only
// displays the stamp when payment_status is currently true (see PaidAt in
// competition-detail.html), but the raw stamp fields do go stale on unpaid.
func TestCompTogglePaymentUnpaidKeepsStaleStamp(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/payment toggling to unpaid leaves prior stamp intact",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID, priorAdminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		priorAdminID = admin.Id
		pair := handlers.MakePairTB(tb, app, "StaleStampPair")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		comp.Set("payment_status", map[string]any{pairID: true})
		comp.Set("payment_paid_at", map[string]any{pairID: "2026-01-01T00:00:00Z"})
		comp.Set("payment_paid_by", map[string]any{pairID: admin.Id})
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/payment"
		s.Body = strings.NewReader("pair_id=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		var status map[string]bool
		require.NoError(tb, c.UnmarshalJSONField("payment_status", &status))
		assert.False(tb, status[pairID], "pair must be marked unpaid after toggle")

		var paidAt map[string]string
		require.NoError(tb, c.UnmarshalJSONField("payment_paid_at", &paidAt))
		var paidBy map[string]string
		require.NoError(tb, c.UnmarshalJSONField("payment_paid_by", &paidBy))
		assert.Equal(tb, "2026-01-01T00:00:00Z", paidAt[pairID], "stamp is not cleared on unpaid (current behavior)")
		assert.Equal(tb, priorAdminID, paidBy[pairID], "stamp is not cleared on unpaid (current behavior)")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompApplyPenalty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/penalty applies penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := handlers.MakePairTB(tb, app, "PenPair")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/penalty"
		s.Body = strings.NewReader("pair_id=" + pair.Id + "&action=apply&amount=3&reason=Prueba")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		rows, err := app.FindRecordsByFilter("penalties",
			"competition = {:c} && pair = {:p} && voided = false", "", 0, 0,
			map[string]any{"c": compID, "p": pairID})
		require.NoError(tb, err)
		require.Len(tb, rows, 1, "must create exactly one penalty row")
		assert.Equal(tb, 3.0, rows[0].GetFloat("amount"), "default penalty must be 3")
		assert.Equal(tb, "Prueba", rows[0].GetString("reason"))

		want := league.Notification{
			Type:     "penalty",
			Title:    "Penalización aplicada",
			Body:     "3 puntos — Prueba",
			Link:     "/competition/" + compID,
			CompName: "Test Competition",
		}
		for _, uid := range league.PlayersForPair(app, pairID) {
			assertNotified(tb, app, uid, want)
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompAddPairDuplicateRejectedByUniqueness(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects re-adding same pair",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := handlers.MakePairTB(tb, app, "DupPair")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompAddPairOverlappingPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects overlapping player",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"duplicados"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)

		u1 := handlers.MakeUserTB(tb, app, "Overlap1", "")
		u2 := handlers.MakeUserTB(tb, app, "Overlap2", "")
		u3 := handlers.MakeUserTB(tb, app, "Overlap3", "")

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

		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair1})
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair2.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompAddPairGenderRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id}/pairs rejects wrong gender",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"solo masculina"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairWithGendersTB(tb, app, "MixedPair", "male", "female")
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		comp.Set("gender_type", "male")
		require.NoError(tb, app.Save(comp))
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCompAddPairGenderAccepted(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/pairs accepts matching gender",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairWithGendersTB(tb, app, "MixedOK", "male", "female")
		pairID = pair.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		comp.Set("gender_type", "mixed")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/pairs"
		s.Body = strings.NewReader("pair=" + pair.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Contains(tb, c.GetStringSlice("pairs"), pairID)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompRemovePairCleansUpMetadata(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/remove-pair cleans seeding and payment",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "RemMetaA")
		p2 := handlers.MakePairTB(tb, app, "RemMetaB")
		pairID = p1.Id
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, c.GetStringSlice("pairs"), pairID)
		assert.Contains(tb, c.GetStringSlice("pairs"), p2ID, "other pair must remain")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompGenerateFixtures(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/generate creates fixtures",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "GenA")
		p2 := handlers.MakePairTB(tb, app, "GenB")
		p3 := handlers.MakePairTB(tb, app, "GenC")
		p4 := handlers.MakePairTB(tb, app, "GenD")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-09-01 00:00:00.000Z")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = handlers.AuthHeaders(tb, admin)
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
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompGenerateFixtures_DraftModeNoNotification(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/generate sets calendar_status=draft and does not notify players",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "NotA")
		p2 := handlers.MakePairTB(tb, app, "NotB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-09-01 00:00:00.000Z")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		notifs, err := app.FindRecordsByFilter("notifications",
			"type = 'match_assigned'", "", 0, 0, nil)
		require.NoError(tb, err)
		assert.Empty(tb, notifs, "generating fixtures must not notify players — that happens on publish")

		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "draft", comp.GetString("calendar_status"))

		events, err := app.FindRecordsByFilter("competition_events",
			"competition = {:cid} && kind = 'fixtures_generated'", "", 0, 0, map[string]any{"cid": compID})
		require.NoError(tb, err)
		assert.Len(tb, events, 1, "generation must log a fixtures_generated competition_event")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestCompRegenerateFixtures_PublishedGoesBackToDraft(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/generate?confirm=true on a published competition reverts to draft",
		Method:         http.MethodPost,
		URL:            "?confirm=true",
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "RegenPubA")
		p2 := handlers.MakePairTB(tb, app, "RegenPubB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		comp.Set("calendar_status", "published")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate?confirm=true"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "draft", comp.GetString("calendar_status"),
			"regenerating a published calendar must revert it to draft")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestGenerateFixturesTooFewPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id}/generate with 1 pair rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"al menos 2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "FewA")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1})
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestGeneratePlayoffWithByes(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/generate playoff with 3 teams creates byes",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "ByeA")
		p2 := handlers.MakePairTB(tb, app, "ByeB")
		p3 := handlers.MakePairTB(tb, app, "ByeC")
		comp := handlers.MakeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2, p3})
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id + "/generate"
		s.Headers = handlers.AuthHeaders(tb, admin)
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
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestDisputeResolve(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve resolves dispute",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID, p2ID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DispA")
		p2 := handlers.MakePairTB(tb, app, "DispB")
		p1ID = p1.Id
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=6-3+6-4&winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-3 6-4", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))

		compRec, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		want := league.Notification{
			Type:     "dispute",
			Title:    "Disputa resuelta",
			Body:     "El administrador ha resuelto la disputa",
			MatchID:  matchID,
			CompName: compRec.GetString("name"),
		}
		for _, uid := range league.PlayersForPair(app, p1ID) {
			assertNotified(tb, app, uid, want)
		}
		for _, uid := range league.PlayersForPair(app, p2ID) {
			assertNotified(tb, app, uid, want)
		}
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

func TestPairsUpdateRejectsOverlappingPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/pairs/{id} rejects overlapping player",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya participa en otra pareja"},
	}
	var pair2ID, playerCID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)

		playerA := handlers.MakeUserTB(tb, app, "PlayerA", "")
		playerB := handlers.MakeUserTB(tb, app, "PlayerB", "")
		playerC := handlers.MakeUserTB(tb, app, "PlayerC", "")
		playerD := handlers.MakeUserTB(tb, app, "PlayerD", "")

		col, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)

		pair1 := core.NewRecord(col)
		pair1.Set("name", "Pair1")
		pair1.Set("player1", playerA.Id)
		pair1.Set("player2", playerB.Id)
		pair1.Set("captain", playerA.Id)
		require.NoError(tb, app.Save(pair1))

		pair2 := core.NewRecord(col)
		pair2.Set("name", "Pair2")
		pair2.Set("player1", playerC.Id)
		pair2.Set("player2", playerD.Id)
		pair2.Set("captain", playerC.Id)
		require.NoError(tb, app.Save(pair2))

		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair1, pair2})
		_ = comp

		pair2ID = pair2.Id
		playerCID = playerC.Id

		s.URL = "/admin/pairs/" + pair2.Id
		// Send playerD as captain (valid after player1 swap), so captain check passes
		// and overlap check fires.
		s.Body = strings.NewReader("player1=" + playerA.Id + "&captain=" + playerD.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pair2, err := app.FindRecordById("pairs", pair2ID)
		require.NoError(tb, err)
		assert.Equal(tb, playerCID, pair2.GetString("player1"))
	}
	s.Test(t)
}

func TestPairsUpdateAllowsNonOverlapping(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/pairs/{id} allows non-overlapping update",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)

		playerA := handlers.MakeUserTB(tb, app, "PlayerA", "")
		playerB := handlers.MakeUserTB(tb, app, "PlayerB", "")
		playerC := handlers.MakeUserTB(tb, app, "PlayerC", "")
		playerD := handlers.MakeUserTB(tb, app, "PlayerD", "")

		col, err := app.FindCollectionByNameOrId("pairs")
		require.NoError(tb, err)

		pair1 := core.NewRecord(col)
		pair1.Set("name", "Pair1")
		pair1.Set("player1", playerA.Id)
		pair1.Set("player2", playerB.Id)
		pair1.Set("captain", playerA.Id)
		require.NoError(tb, app.Save(pair1))

		pair2 := core.NewRecord(col)
		pair2.Set("name", "Pair2")
		pair2.Set("player1", playerC.Id)
		pair2.Set("player2", playerD.Id)
		pair2.Set("captain", playerC.Id)
		require.NoError(tb, app.Save(pair2))

		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair1, pair2})
		_ = comp

		s.URL = "/admin/pairs/" + pair2.Id
		s.Body = strings.NewReader("name=RenamedPair")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/pairs"))
	s.Test(t)
}

// withdrawScenario builds a POST withdraw-pair scenario. setup creates the
// competition and its matches, returning the competition id and the pair to withdraw.
// Callers set AfterTestFunc (and ExpectRedirect or ExpectedStatus) on the result.
func withdrawScenario(name string, setup func(tb testing.TB, app *tests.TestApp) (string, string)) *tests.ApiScenario {
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           name,
		Method:         http.MethodPost,
		ExpectedStatus: http.StatusNoContent,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		compID, pairID := setup(tb, app)
		// Hooks go on after the fixtures so seeding can set matches straight to final.
		notifier := notify.NewNotifier(app, "", "")
		hooks.Register(app, hooks.Deps{Svc: league.New(app, notifier), Notifier: notifier})
		s.URL = "/admin/competitions/" + compID + "/withdraw-pair"
		s.Body = strings.NewReader("pair_id=" + pairID)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	return s
}

func loadMatch(tb testing.TB, app core.App, id string) *core.Record {
	tb.Helper()
	m, err := app.FindRecordById("matches", id)
	require.NoError(tb, err)
	return m
}

func matchMessages(tb testing.TB, app core.App, matchID, msgType string) []*core.Record {
	tb.Helper()
	msgs, err := app.FindRecordsByFilter("match_messages", "match = {:m} && type = {:t}", "", 0, 0,
		map[string]any{"m": matchID, "t": msgType})
	require.NoError(tb, err)
	return msgs
}

func addChat(tb testing.TB, app core.App, matchID, authorID string) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("author", authorID)
	msg.Set("type", "chat")
	msg.Set("content", "¿A qué hora jugamos?")
	require.NoError(tb, app.Save(msg))
	return msg
}

func releaseEventCount(tb testing.TB, app core.App, compID string) int {
	tb.Helper()
	evs, err := app.FindRecordsByFilter("competition_events",
		"competition = {:c} && kind = 'assignment_released'", "", 0, 0, map[string]any{"c": compID})
	require.NoError(tb, err)
	return len(evs)
}

func withdrawalNoticeCount(tb testing.TB, app core.App, pairID string) int {
	tb.Helper()
	n := 0
	for _, uid := range league.PlayersForPair(app, pairID) {
		recs, err := app.FindRecordsByFilter("notifications",
			"user = {:user} && title = 'Pareja retirada'", "", 0, 0, map[string]any{"user": uid})
		require.NoError(tb, err)
		n += len(recs)
	}
	return n
}

// notificationsTitled returns the user's notifications with the given title.
func notificationsTitled(tb testing.TB, app core.App, userID, title string) []*core.Record {
	tb.Helper()
	recs, err := app.FindRecordsByFilter("notifications",
		"user = {:user} && title = {:title}", "", 0, 0, map[string]any{"user": userID, "title": title})
	require.NoError(tb, err)
	return recs
}

// TestWithdrawPair_NoticesOnePerChangedMatch: the opponent gets one notice for the changed match, linking to
// the match, and the withdrawn pair's players get one withdrawal notice.
func TestWithdrawPair_NoticesOnePerChangedMatch(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair sends one notice per changed match and one withdrawal notice",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			matchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, uid := range league.PlayersForPair(app, p2.Id) {
			recs := notificationsTitled(tb, app, uid, "Pareja retirada")
			require.Len(tb, recs, 1, "opponent %s must get one notice", uid)
			assert.Equal(tb, "/match/"+matchID, recs[0].GetString("link"))
		}
		for _, uid := range league.PlayersForPair(app, p1.Id) {
			assert.Len(tb, notificationsTitled(tb, app, uid, "Retirada de la competición"), 1,
				"withdrawn player %s must get one withdrawal notice", uid)
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_LeveledReleasedNoticeLinksToCompetition: the opponent of a released leveled match gets one
// notice that links to the competition page, because the match no longer exists.
func TestWithdrawPair_LeveledReleasedNoticeLinksToCompetition(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var compID string
	s := withdrawScenario("POST withdraw-pair notifies the opponent of a released leveled match with a competition link",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3)
			compID = comp.Id
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, uid := range league.PlayersForPair(app, p2.Id) {
			recs := notificationsTitled(tb, app, uid, "Pareja retirada")
			require.Len(tb, recs, 1, "opponent %s must get one notice", uid)
			assert.Equal(tb, "/competition/"+compID, recs[0].GetString("link"))
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// leveledWithdrawComp is a three-pair league with target_matches 1, which makes IsLeveled true.
func leveledWithdrawComp(tb testing.TB, app *tests.TestApp, pairs ...*core.Record) *core.Record {
	tb.Helper()
	comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
	comp.Set("target_matches", 1)
	require.NoError(tb, app.Save(comp))
	return comp
}

// TestWithdrawPair_StatusMatrix: each pre-score, disputed and played match ends final with the
// opponent as winner, the walkover score in the winner's order, and the review type the status implies.
func TestWithdrawPair_StatusMatrix(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var pending, scheduled, proposed, pendingProposed, disputed, played string
	s := withdrawScenario("POST withdraw-pair applies the official walkover to every match state",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			pending = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			scheduled = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p1.Id, "scheduled").Id
			proposedMatch := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
			handlers.MakeResultProposal(tb, app, proposedMatch.Id, league.PlayersForPair(app, p2.Id)[0], "6-2 6-3")
			proposed = proposedMatch.Id
			pendingProposedMatch := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "pending")
			handlers.MakeResultProposal(tb, app, pendingProposedMatch.Id, league.PlayersForPair(app, p3.Id)[0], "6-2 6-3")
			pendingProposed = pendingProposedMatch.Id
			disputed = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p1.Id, "disputed").Id
			playedMatch := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			playedMatch.Set("scores", "3-6 4-6")
			playedMatch.Set("winner", p2.Id)
			require.NoError(tb, app.Save(playedMatch))
			played = playedMatch.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		want := []struct{ label, id, winner, scores, review, rating string }{
			{"pending", pending, p2.Id, "0-6 0-6", "walkover", ""},
			{"scheduled", scheduled, p3.Id, "6-0 6-0", "walkover", ""},
			{"proposed", proposed, p2.Id, "0-6 0-6", "", "6-2 6-3"},
			{"pending proposed", pendingProposed, p3.Id, "0-6 0-6", "", "6-2 6-3"},
			{"disputed", disputed, p3.Id, "6-0 6-0", "walkover", ""},
			{"played", played, p2.Id, "0-6 0-6", "", "3-6 4-6"},
		}
		for _, w := range want {
			m := loadMatch(tb, app, w.id)
			assert.Equal(tb, "final", m.GetString("status"), w.label+" status")
			assert.Equal(tb, w.winner, m.GetString("winner"), w.label+" winner")
			assert.Equal(tb, w.scores, m.GetString("scores"), w.label+" scores")
			assert.Equal(tb, w.review, m.GetString("review_type"), w.label+" review_type")
			assert.Equal(tb, w.rating, m.GetString("rating_scores"), w.label+" rating_scores")
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_AdminOverrideFinalUnchanged: a final match an admin set to the official result is left alone.
func TestWithdrawPair_AdminOverrideFinalUnchanged(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair leaves an admin-set official result unchanged",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "0-6 0-6")
			m.Set("winner", p2.Id)
			m.Set("admin_result", true)
			require.NoError(tb, app.Save(m))
			matchID = m.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, matchID)
		assert.Equal(tb, "0-6 0-6", m.GetString("scores"))
		assert.Equal(tb, p2.Id, m.GetString("winner"))
		assert.Equal(tb, "", m.GetString("review_type"))
		assert.Empty(tb, matchMessages(tb, app, matchID, "result_event"))
		assert.Zero(tb, withdrawalNoticeCount(tb, app, p2.Id))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_ApprovedWalkoverUnchanged: a final approved walkover is left alone.
func TestWithdrawPair_ApprovedWalkoverUnchanged(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair leaves an approved walkover unchanged",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "0-6 0-6")
			m.Set("winner", p2.Id)
			m.Set("review_type", "walkover")
			m.Set("admin_result", true)
			require.NoError(tb, app.Save(m))
			matchID = m.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, matchID)
		assert.Equal(tb, "0-6 0-6", m.GetString("scores"))
		assert.Equal(tb, p2.Id, m.GetString("winner"))
		assert.Equal(tb, "walkover", m.GetString("review_type"))
		assert.Empty(tb, matchMessages(tb, app, matchID, "result_event"))
		assert.Zero(tb, withdrawalNoticeCount(tb, app, p2.Id))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayerSixZeroChanged: a final 6-0 6-0 the players produced becomes an admin walkover.
func TestWithdrawPair_PlayerSixZeroChanged(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair turns a player-produced 6-0 6-0 into an admin walkover",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "0-6 0-6")
			m.Set("winner", p2.Id)
			require.NoError(tb, app.Save(m))
			matchID = m.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, matchID)
		// A played match keeps its previous review type (requirements: review type rule).
		assert.Empty(tb, m.GetString("review_type"))
		assert.Equal(tb, "0-6 0-6", m.GetString("rating_scores"))
		assert.True(tb, m.GetBool("admin_result"))
		assert.Equal(tb, "0-6 0-6", m.GetString("scores"))
		events := matchMessages(tb, app, matchID, "result_event")
		require.Len(tb, events, 1)
		assert.Equal(tb, "Incomparecencia — WdrawA se ha retirado de la competición", events[0].GetString("content"))
		// One notice per opponent player.
		require.Len(tb, league.PlayersForPair(app, p2.Id), 2)
		assert.Equal(tb, 2, withdrawalNoticeCount(tb, app, p2.Id))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_WonByWithdrawnBecomesLoss: a final the withdrawn pair won becomes a loss.
func TestWithdrawPair_WonByWithdrawnBecomesLoss(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair turns a final won by the withdrawn pair into a loss",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "6-0 6-0")
			m.Set("winner", p1.Id)
			require.NoError(tb, app.Save(m))
			matchID = m.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, matchID)
		assert.Equal(tb, p2.Id, m.GetString("winner"))
		assert.Equal(tb, "0-6 0-6", m.GetString("scores"))
		// A played match keeps its previous review type (requirements: review type rule).
		assert.Empty(tb, m.GetString("review_type"))
		assert.Equal(tb, "6-0 6-0", m.GetString("rating_scores"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_ClearsArbitration: an open arbitration request of any category is cleared.
func TestWithdrawPair_ClearsArbitration(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var schedID, pendingID string
	s := withdrawScenario("POST withdraw-pair clears open arbitration requests",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			pm := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			pm.Set("arbitration", "scheduling")
			pm.Set("arbitration_by", league.PlayersForPair(app, p2.Id)[0])
			require.NoError(tb, app.Save(pm))
			pendingID = pm.Id
			sm := handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p1.Id, "scheduled")
			sm.Set("arbitration", "abandonment")
			sm.Set("arbitration_by", league.PlayersForPair(app, p3.Id)[0])
			require.NoError(tb, app.Save(sm))
			schedID = sm.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range []string{pendingID, schedID} {
			m := loadMatch(tb, app, id)
			assert.Equal(tb, "", m.GetString("arbitration"), "arbitration of %s", id)
			assert.Equal(tb, "", m.GetString("arbitration_by"), "arbitration_by of %s", id)
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_KeepsEarlierTimeline: earlier thread messages stay, one withdrawal entry per changed match.
func TestWithdrawPair_KeepsEarlierTimeline(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var pendingID, schedID string
	var chatPending, chatSched *core.Record
	s := withdrawScenario("POST withdraw-pair adds exactly one timeline entry per changed match",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			pendingID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			schedID = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p1.Id, "scheduled").Id
			chatPending = addChat(tb, app, pendingID, league.PlayersForPair(app, p2.Id)[0])
			chatSched = addChat(tb, app, schedID, league.PlayersForPair(app, p3.Id)[0])
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, c := range []*core.Record{chatPending, chatSched} {
			again, err := app.FindRecordById("match_messages", c.Id)
			require.NoError(tb, err)
			assert.Equal(tb, c.GetString("content"), again.GetString("content"))
		}
		assert.Len(tb, matchMessages(tb, app, pendingID, "result_event"), 1)
		assert.Len(tb, matchMessages(tb, app, schedID, "result_event"), 1)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffFinishedMatchesStay: a finished playoff match is not changed by a withdrawal.
func TestWithdrawPair_PlayoffFinishedMatchesStay(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var finishedID string
	s := withdrawScenario("POST withdraw-pair leaves a finished playoff match unchanged",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 := handlers.MakePairTB(tb, app, "WdrawC")
			p4 := handlers.MakePairTB(tb, app, "WdrawD")
			comp := makePlayoffComp(tb, app, []*core.Record{p1, p2, p3, p4}, nil)
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", "3-6 4-6")
			m.Set("winner", p2.Id)
			require.NoError(tb, app.Save(m))
			finishedID = m.Id
			handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, finishedID)
		assert.Equal(tb, "3-6 4-6", m.GetString("scores"))
		assert.Equal(tb, p2.Id, m.GetString("winner"))
		assert.Equal(tb, "", m.GetString("review_type"))
		assert.Empty(tb, matchMessages(tb, app, finishedID, "result_event"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffRoundWinnerResolvedOnSeed: a withdrawn pair that won its round while
// the round was open loses the next-round match once that round seeds it, and the opponent
// goes on to the round after.
func TestWithdrawPair_PlayoffRoundWinnerResolvedOnSeed(t *testing.T) {
	t.Parallel()
	var p1, p2, p3, p4 *core.Record
	var roundOneID, openMatchID, roundTwoID, roundThreeID string
	s := withdrawScenario("POST withdraw-pair resolves a withdrawn round winner when the next round seeds it",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			p4 = handlers.MakePairTB(tb, app, "WdrawD")
			comp := makePlayoffComp(tb, app, []*core.Record{p1, p2, p3, p4}, nil)
			won := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			won.Set("scores", "6-0 6-0")
			won.Set("winner", p1.Id)
			require.NoError(tb, app.Save(won))
			roundOneID = won.Id
			openMatchID = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending").Id
			next := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
			next.Set("round_number", 2)
			require.NoError(tb, app.Save(next))
			roundTwoID = next.Id
			third := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
			third.Set("round_number", 3)
			require.NoError(tb, app.Save(third))
			roundThreeID = third.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		// Finishing the open round seeds round two, which must resolve the withdrawn pair.
		open := loadMatch(tb, app, openMatchID)
		open.Set("status", "final")
		open.Set("scores", "6-2 6-3")
		open.Set("winner", p3.Id)
		require.NoError(tb, app.Save(open))

		won := loadMatch(tb, app, roundOneID)
		assert.Equal(tb, p1.Id, won.GetString("winner"), "a finished round-one match stands")
		assert.Equal(tb, "6-0 6-0", won.GetString("scores"))

		next := loadMatch(tb, app, roundTwoID)
		assert.Equal(tb, "final", next.GetString("status"))
		assert.Equal(tb, p3.Id, next.GetString("winner"))
		assert.Equal(tb, "walkover", next.GetString("review_type"))
		assert.Equal(tb, "0-6 0-6", next.GetString("scores"))

		// The walkover final advances its opponent: p3 is seeded into round three.
		third := loadMatch(tb, app, roundThreeID)
		assert.Equal(tb, p3.Id, third.GetString("pair1"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// playoffSeedsBracket seeds an eight-pair playoff: round one has two played matches (pairs[0] beat
// pairs[1], pairs[2] beat pairs[3]) and two open ones, and round two has two matches waiting to be
// seeded. The pairs at withdrawnBefore are withdrawn before the POST.
func playoffSeedsBracket(tb testing.TB, app *tests.TestApp, pairs *[8]*core.Record, withdrawnBefore ...int) (compID string, open, seeded []string) {
	tb.Helper()
	for i := range pairs {
		pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("WdrawP%d", i+1))
	}
	comp := makePlayoffComp(tb, app, pairs[:], nil)
	var withdrawn []string
	for _, i := range withdrawnBefore {
		withdrawn = append(withdrawn, pairs[i].Id)
	}
	comp.Set("withdrawn_pairs", withdrawn)
	require.NoError(tb, app.Save(comp))
	for _, won := range [][2]*core.Record{{pairs[0], pairs[1]}, {pairs[2], pairs[3]}} {
		m := handlers.MakeMatchTB(tb, app, comp.Id, won[0].Id, won[1].Id, "final")
		m.Set("scores", "6-0 6-0")
		m.Set("winner", won[0].Id)
		require.NoError(tb, app.Save(m))
	}
	for _, p := range [][2]*core.Record{{pairs[4], pairs[5]}, {pairs[6], pairs[7]}} {
		open = append(open, handlers.MakeMatchTB(tb, app, comp.Id, p[0].Id, p[1].Id, "pending").Id)
	}
	for range 2 {
		next := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
		next.Set("round_number", 2)
		require.NoError(tb, app.Save(next))
		seeded = append(seeded, next.Id)
	}
	return comp.Id, open, seeded
}

// finishOpenMatches finishes each match in ids with pair1 as the winner, which seeds the next round.
func finishOpenMatches(tb testing.TB, app *tests.TestApp, ids []string) {
	tb.Helper()
	for _, id := range ids {
		open := loadMatch(tb, app, id)
		open.Set("status", "final")
		open.Set("scores", "6-2 6-3")
		open.Set("winner", open.GetString("pair1"))
		require.NoError(tb, app.Save(open))
	}
}

// resultEventTexts returns the texts of matchID's result_event timeline entries, oldest first.
func resultEventTexts(tb testing.TB, app *tests.TestApp, matchID string) []string {
	tb.Helper()
	var texts []string
	for _, m := range matchMessages(tb, app, matchID, "result_event") {
		texts = append(texts, m.GetString("content"))
	}
	return texts
}

// TestWithdrawPair_PlayoffBothSeedsReplacedByLosers: when a round seeds a match whose two seeds are
// both withdrawn, each seed is replaced by the pair that lost to it in the previous round, and those
// two pairs play the match.
func TestWithdrawPair_PlayoffBothSeedsReplacedByLosers(t *testing.T) {
	t.Parallel()
	var pairs [8]*core.Record
	var open, seeded []string
	s := withdrawScenario("POST withdraw-pair replaces both withdrawn seeds with the round-one losers",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			compID, o, sd := playoffSeedsBracket(tb, app, &pairs, 0)
			open, seeded = o, sd
			return compID, pairs[2].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		finishOpenMatches(tb, app, open)

		replaced := loadMatch(tb, app, seeded[0])
		assert.Equal(tb, "pending", replaced.GetString("status"))
		assert.Equal(tb, pairs[1].Id, replaced.GetString("pair1"), "the loser to WdrawP1 takes its seed")
		assert.Equal(tb, pairs[3].Id, replaced.GetString("pair2"), "the loser to WdrawP3 takes its seed")
		assert.Empty(tb, replaced.GetString("winner"))
		assert.Empty(tb, replaced.GetString("review_type"))
		assert.Equal(tb, []string{
			"WdrawP2 juega en lugar de WdrawP1, retirada de la competición",
			"WdrawP4 juega en lugar de WdrawP3, retirada de la competición",
		}, resultEventTexts(tb, app, seeded[0]))

		other := loadMatch(tb, app, seeded[1])
		assert.Equal(tb, "pending", other.GetString("status"))
		assert.Equal(tb, pairs[4].Id, other.GetString("pair1"))
		assert.Equal(tb, pairs[6].Id, other.GetString("pair2"))
		assert.Empty(tb, resultEventTexts(tb, app, seeded[1]))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffWithdrawnReplacementGivesWalkover: when a seed's replacement is withdrawn
// too, the playable replacement wins the match by walkover.
func TestWithdrawPair_PlayoffWithdrawnReplacementGivesWalkover(t *testing.T) {
	t.Parallel()
	var pairs [8]*core.Record
	var open, seeded []string
	s := withdrawScenario("POST withdraw-pair gives a walkover to the playable replacement of a withdrawn one",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			compID, o, sd := playoffSeedsBracket(tb, app, &pairs, 0, 1)
			open, seeded = o, sd
			return compID, pairs[2].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		finishOpenMatches(tb, app, open)

		walkover := loadMatch(tb, app, seeded[0])
		assert.Equal(tb, "final", walkover.GetString("status"))
		assert.Equal(tb, pairs[3].Id, walkover.GetString("winner"))
		assert.Equal(tb, "walkover", walkover.GetString("review_type"))
		assert.Equal(tb, pairs[1].Id, walkover.GetString("pair1"))
		assert.Equal(tb, pairs[3].Id, walkover.GetString("pair2"))
		assert.Equal(tb, "0-6 0-6", walkover.GetString("scores"))
		assert.Equal(tb, []string{
			"WdrawP2 juega en lugar de WdrawP1, retirada de la competición",
			"WdrawP4 juega en lugar de WdrawP3, retirada de la competición",
			"Incomparecencia — WdrawP2 se ha retirado de la competición",
		}, resultEventTexts(tb, app, seeded[0]))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffBothSeedsNoReplacementWalkover: a seed that came from a bye has no
// replacement, so its side is left empty and the playable replacement wins by walkover.
func TestWithdrawPair_PlayoffBothSeedsNoReplacementWalkover(t *testing.T) {
	t.Parallel()
	var pairs [8]*core.Record
	var open []string
	var seeded []string
	s := withdrawScenario("POST withdraw-pair gives a walkover when a withdrawn seed has no replacement",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			for i := range pairs {
				pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("WdrawP%d", i+1))
			}
			comp := makePlayoffComp(tb, app, pairs[:], nil)
			comp.Set("withdrawn_pairs", []string{pairs[0].Id})
			require.NoError(tb, app.Save(comp))
			bye := handlers.MakeMatchTB(tb, app, comp.Id, pairs[0].Id, "", "final")
			bye.Set("winner", pairs[0].Id)
			require.NoError(tb, app.Save(bye))
			won := handlers.MakeMatchTB(tb, app, comp.Id, pairs[2].Id, pairs[3].Id, "final")
			won.Set("scores", "6-0 6-0")
			won.Set("winner", pairs[2].Id)
			require.NoError(tb, app.Save(won))
			for _, p := range [][2]*core.Record{{pairs[4], pairs[5]}, {pairs[6], pairs[7]}} {
				open = append(open, handlers.MakeMatchTB(tb, app, comp.Id, p[0].Id, p[1].Id, "pending").Id)
			}
			for range 2 {
				next := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
				next.Set("round_number", 2)
				require.NoError(tb, app.Save(next))
				seeded = append(seeded, next.Id)
			}
			return comp.Id, pairs[2].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		finishOpenMatches(tb, app, open)

		walkover := loadMatch(tb, app, seeded[0])
		assert.Equal(tb, "final", walkover.GetString("status"))
		assert.Equal(tb, pairs[3].Id, walkover.GetString("winner"))
		assert.Equal(tb, "walkover", walkover.GetString("review_type"))
		assert.Empty(tb, walkover.GetString("pair1"))
		assert.Equal(tb, pairs[3].Id, walkover.GetString("pair2"))
		assert.Equal(tb, "0-6 0-6", walkover.GetString("scores"))
		assert.Equal(tb, []string{
			"WdrawP4 juega en lugar de WdrawP3, retirada de la competición",
			"WdrawP4 gana por incomparecencia: no hay pareja rival que pueda jugar",
		}, resultEventTexts(tb, app, seeded[0]))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffRoundOneVoidGivesNextRoundWalkover: a first-round match whose two pairs are
// both withdrawn has no result, and the opponent waiting in the next round wins it by walkover.
// TestWithdrawPair_PlayoffRoundOneWinnerSeedsWalkover: a round-one walkover whose winner then withdraws
// stands, and the round-two seat it fed goes to the opponent by walkover once the round is seeded.
func TestWithdrawPair_PlayoffRoundOneWinnerSeedsWalkover(t *testing.T) {
	t.Parallel()
	var pairs [8]*core.Record
	var firstID string
	var open, seeded []string
	s := withdrawScenario("POST withdraw-pair keeps a round-one walkover and walks over its round-two seat",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			for i := range pairs {
				pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("WdrawP%d", i+1))
			}
			comp := makePlayoffComp(tb, app, pairs[:], nil)
			comp.Set("withdrawn_pairs", []string{pairs[0].Id})
			require.NoError(tb, app.Save(comp))
			// pairs[0] withdrew earlier, so pairs[1] won this match by walkover.
			walkover := handlers.MakeMatchTB(tb, app, comp.Id, pairs[0].Id, pairs[1].Id, "pending")
			league.ApplyWalkoverResult(walkover, pairs[1].Id, league.WalkoverScore(comp))
			require.NoError(tb, app.Save(walkover))
			firstID = walkover.Id
			won := handlers.MakeMatchTB(tb, app, comp.Id, pairs[2].Id, pairs[3].Id, "final")
			won.Set("scores", "6-0 6-0")
			won.Set("winner", pairs[2].Id)
			require.NoError(tb, app.Save(won))
			for _, p := range [][2]*core.Record{{pairs[4], pairs[5]}, {pairs[6], pairs[7]}} {
				open = append(open, handlers.MakeMatchTB(tb, app, comp.Id, p[0].Id, p[1].Id, "pending").Id)
			}
			for range 2 {
				next := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
				next.Set("round_number", 2)
				require.NoError(tb, app.Save(next))
				seeded = append(seeded, next.Id)
			}
			return comp.Id, pairs[1].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		first := loadMatch(tb, app, firstID)
		assert.Equal(tb, "final", first.GetString("status"))
		assert.Equal(tb, pairs[1].Id, first.GetString("winner"))
		assert.Equal(tb, "walkover", first.GetString("review_type"))
		assert.Equal(tb, "0-6 0-6", first.GetString("scores"))
		assert.Empty(tb, resultEventTexts(tb, app, firstID))

		finishOpenMatches(tb, app, open)

		walkover := loadMatch(tb, app, seeded[0])
		assert.Equal(tb, "final", walkover.GetString("status"))
		assert.Equal(tb, pairs[2].Id, walkover.GetString("winner"))
		assert.Equal(tb, "walkover", walkover.GetString("review_type"))
		assert.Equal(tb, pairs[1].Id, walkover.GetString("pair1"))
		assert.Equal(tb, pairs[2].Id, walkover.GetString("pair2"))
		assert.Equal(tb, "0-6 0-6", walkover.GetString("scores"))
		assert.Equal(tb, []string{
			"Incomparecencia — WdrawP2 se ha retirado de la competición",
		}, resultEventTexts(tb, app, seeded[0]))

		next := loadMatch(tb, app, seeded[1])
		assert.Equal(tb, "pending", next.GetString("status"))
		assert.Equal(tb, pairs[4].Id, next.GetString("pair1"))
		assert.Equal(tb, pairs[6].Id, next.GetString("pair2"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffVoidedSeedGivesNextRoundWalkover: a round-two match whose two replacements
// are both withdrawn has no result, and the opponent waiting in round three wins by walkover.
func TestWithdrawPair_PlayoffVoidedSeedGivesNextRoundWalkover(t *testing.T) {
	t.Parallel()
	var pairs [8]*core.Record
	var open, seeded []string
	var thirdID string
	s := withdrawScenario("POST withdraw-pair voids a round-two match whose replacements are both withdrawn",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			compID, o, sd := playoffSeedsBracket(tb, app, &pairs, 0, 1, 2)
			open, seeded = o, sd
			third := handlers.MakeMatchTB(tb, app, compID, "", "", "pending")
			third.Set("round_number", 3)
			require.NoError(tb, app.Save(third))
			thirdID = third.Id
			return compID, pairs[3].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		finishOpenMatches(tb, app, open)

		void := loadMatch(tb, app, seeded[0])
		assert.Equal(tb, "final", void.GetString("status"))
		assert.Empty(tb, void.GetString("winner"))
		assert.Empty(tb, void.GetString("scores"))
		assert.Empty(tb, void.GetString("review_type"))
		assert.Equal(tb, []string{
			"WdrawP2 juega en lugar de WdrawP1, retirada de la competición",
			"WdrawP4 juega en lugar de WdrawP3, retirada de la competición",
			"Sin resultado: no hay pareja que pueda jugar este partido",
		}, resultEventTexts(tb, app, seeded[0]))

		finishOpenMatches(tb, app, []string{seeded[1]})

		third := loadMatch(tb, app, thirdID)
		assert.Equal(tb, "final", third.GetString("status"))
		assert.Equal(tb, pairs[4].Id, third.GetString("winner"))
		assert.Equal(tb, "walkover", third.GetString("review_type"))
		assert.Empty(tb, third.GetString("pair1"))
		assert.Equal(tb, pairs[4].Id, third.GetString("pair2"))
		assert.Equal(tb, []string{
			"WdrawP5 gana por incomparecencia: no hay pareja rival que pueda jugar",
		}, resultEventTexts(tb, app, thirdID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffUnfinishedBecomesLoss: the withdrawn pair's unfinished round-one match is
// a walkover loss, and the opponent advances to the next round.
func TestWithdrawPair_PlayoffUnfinishedBecomesLoss(t *testing.T) {
	t.Parallel()
	var p1, p2, p3, p4 *core.Record
	var unfinishedID, openMatchID, roundTwoID string
	s := withdrawScenario("POST withdraw-pair turns an unfinished playoff match into a loss the opponent advances from",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			p4 = handlers.MakePairTB(tb, app, "WdrawD")
			comp := makePlayoffComp(tb, app, []*core.Record{p1, p2, p3, p4}, nil)
			unfinishedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			openMatchID = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending").Id
			next := handlers.MakeMatchTB(tb, app, comp.Id, "", "", "pending")
			next.Set("round_number", 2)
			require.NoError(tb, app.Save(next))
			roundTwoID = next.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		unfinished := loadMatch(tb, app, unfinishedID)
		assert.Equal(tb, "final", unfinished.GetString("status"))
		assert.Equal(tb, p2.Id, unfinished.GetString("winner"))
		assert.Equal(tb, "walkover", unfinished.GetString("review_type"))

		open := loadMatch(tb, app, openMatchID)
		open.Set("status", "final")
		open.Set("scores", "6-2 6-3")
		open.Set("winner", p3.Id)
		require.NoError(tb, app.Save(open))

		next := loadMatch(tb, app, roundTwoID)
		assert.Equal(tb, p2.Id, next.GetString("pair1"))
		assert.Equal(tb, p3.Id, next.GetString("pair2"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffEmptySlotLeftAlone: a round-one match without an opponent stays pending.
func TestWithdrawPair_PlayoffEmptySlotLeftAlone(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var byeID string
	s := withdrawScenario("POST withdraw-pair leaves a playoff match with no opponent pending",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := makePlayoffComp(tb, app, []*core.Record{p1, p2, p3}, nil)
			byeID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, "", "pending").Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, byeID)
		assert.Equal(tb, "pending", m.GetString("status"))
		assert.Equal(tb, "", m.GetString("pair2"))
		assert.Empty(tb, matchMessages(tb, app, byeID, "result_event"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_FinalizedRefused: a finalized competition refuses the withdrawal and changes nothing.
func TestWithdrawPair_FinalizedRefused(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var compID, matchID string
	s := withdrawScenario("POST withdraw-pair refuses a finalized competition",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			comp.Set("finalized", true)
			require.NoError(tb, app.Save(comp))
			matchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			compID = comp.Id
			return comp.Id, p1.Id
		})
	s.ExpectedStatus = http.StatusOK
	s.ExpectedContent = []string{"La competición está finalizada; no se puede retirar una pareja"}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Empty(tb, res.Header.Get("HX-Redirect"))
		m := loadMatch(tb, app, matchID)
		assert.Equal(tb, "pending", m.GetString("status"))
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, comp.GetStringSlice("withdrawn_pairs"), p1.Id)
	}
	s.Test(t)
}

// TestWithdrawPair_RecordsWithdrawnPair: the pair is added to withdrawn_pairs.
func TestWithdrawPair_RecordsWithdrawnPair(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var compID string
	s := withdrawScenario("POST withdraw-pair adds the pair to withdrawn_pairs",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			compID = comp.Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, []string{p1.Id}, comp.GetStringSlice("withdrawn_pairs"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_SecondWithdrawalRefused: withdrawing an already withdrawn pair is refused and changes nothing.
func TestWithdrawPair_SecondWithdrawalRefused(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var matchID string
	s := withdrawScenario("POST withdraw-pair refuses a pair that is already withdrawn",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			comp.Set("withdrawn_pairs", []string{p1.Id})
			require.NoError(tb, app.Save(comp))
			matchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			return comp.Id, p1.Id
		})
	s.ExpectedStatus = http.StatusOK
	s.ExpectedContent = []string{"La pareja ya está retirada"}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, "pending", loadMatch(tb, app, matchID).GetString("status"))
	}
	s.Test(t)
}

// TestWithdrawPair_TwoAdminsWithdrawOnce: two admins confirm the same withdrawal at once. One wins;
// the other is refused inside the transaction, so each match gets one timeline entry, the pair is
// listed once, and each opponent gets one notice per match.
func TestWithdrawPair_TwoAdminsWithdrawOnce(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "two admins withdraw one pair at the same time",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			notifier := notify.NewNotifier(app, "", "")
			hooks.Register(app, hooks.Deps{Svc: league.New(app, notifier), Notifier: notifier})
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			compID := fx[0].match.GetString("competition")
			path := "/admin/competitions/" + compID + "/withdraw-pair"
			form := "pair_id=" + fx[0].match.GetString("pair1")
			return []racer{{a1, path, form}, {a2, path, form}}
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture, rs []*httptest.ResponseRecorder) {
			require.Len(tb, rs, 2)
			assert.Equal(tb, 1, succeeded(rs), "exactly one admin may withdraw the pair")
			withdrawn := fx[0].match.GetString("pair1")
			comp := reload(tb, app, "competitions", fx[0].match.GetString("competition"))
			assert.Equal(tb, []string{withdrawn}, comp.GetStringSlice("withdrawn_pairs"))
			withdrawalText := league.WithdrawalEventText("Race A")
			for _, f := range fx {
				assert.Equal(tb, []string{withdrawalText}, resultEventTexts(tb, app, f.match.Id),
					"match %s: one withdrawal entry", f.match.Id)
			}
			opponent := reload(tb, app, "pairs", fx[0].match.GetString("pair2"))
			for _, uid := range league.PlayersForPair(app, opponent.Id) {
				assert.Len(tb, notificationsTitled(tb, app, uid, "Pareja retirada"), raceMatches,
					"opponent %s: one notice per changed match", uid)
			}
		})
}

// TestWithdrawPair_FailureRollsBack: a failed write on one match leaves every match, the
// withdrawn list and the notices untouched, and the admin sees the failure.
func TestWithdrawPair_FailureRollsBack(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var compID, pendingID, schedID string
	s := withdrawScenario("POST withdraw-pair rolls back every change when one match fails to save",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			compID = comp.Id
			pendingID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			schedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled").Id
			updates := 0
			app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
				updates++
				if updates == 2 {
					return errors.New("injected write failure")
				}
				return e.Next()
			})
			return comp.Id, p1.Id
		})
	s.ExpectedStatus = http.StatusOK
	s.ExpectedContent = []string{"Error al procesar el retiro"}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Empty(tb, res.Header.Get("HX-Redirect"))
		for _, id := range []string{pendingID, schedID} {
			m := loadMatch(tb, app, id)
			assert.Contains(tb, []string{"pending", "scheduled"}, m.GetString("status"), "match %s", id)
			assert.Empty(tb, m.GetString("review_type"), "match %s", id)
		}
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Empty(tb, comp.GetStringSlice("withdrawn_pairs"))
		assert.Zero(tb, withdrawalNoticeCount(tb, app, p2.Id))
		assert.Zero(tb, withdrawalNoticeCount(tb, app, p3.Id))
	}
	s.Test(t)
}

// TestWithdrawPair_MarksWithdrawnBeforeChanges: the withdrawn list is saved before any match changes.
func TestWithdrawPair_MarksWithdrawnBeforeChanges(t *testing.T) {
	t.Parallel()
	var p1, p2 *core.Record
	var compID string
	var updated bool
	var withdrawnAtFirstWrite []string
	s := withdrawScenario("POST withdraw-pair records the withdrawal before the first match write",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			compID = comp.Id
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
				if !updated {
					updated = true
					comp, err := e.App.FindRecordById("competitions", compID)
					if err != nil {
						return err
					}
					withdrawnAtFirstWrite = comp.GetStringSlice("withdrawn_pairs")
				}
				return e.Next()
			})
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		require.True(tb, updated, "a match write must happen")
		assert.Equal(tb, []string{p1.Id}, withdrawnAtFirstWrite)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_LeveledReleasesPendingAndScheduled: pending and scheduled matches are released, with one log event each.
func TestWithdrawPair_LeveledReleasesPendingAndScheduled(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var compID, pendingID, schedID string
	s := withdrawScenario("POST withdraw-pair releases pending and scheduled leveled matches",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3)
			compID = comp.Id
			pendingID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			schedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled").Id
			addChat(tb, app, pendingID, league.PlayersForPair(app, p2.Id)[0])
			addChat(tb, app, schedID, league.PlayersForPair(app, p3.Id)[0])
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range []string{pendingID, schedID} {
			_, err := app.FindRecordById("matches", id)
			assert.Error(tb, err, "match %s must be deleted", id)
			assert.Empty(tb, matchMessages(tb, app, id, "chat"), "thread of %s must be deleted", id)
		}
		assert.Equal(tb, 2, releaseEventCount(tb, app, compID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_LeveledTopUpAfterReleaseNeverPairsWithdrawn: the top-up that runs after the release
// gives the freed opponent a new rival, and never pairs it with the withdrawn pair.
func TestWithdrawPair_LeveledTopUpAfterReleaseNeverPairsWithdrawn(t *testing.T) {
	t.Parallel()
	var p1, p2, p3, p4 *core.Record
	var compID string
	var released, kept []string
	s := withdrawScenario("POST withdraw-pair: the top-up after a release pairs the freed opponent with a new rival",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			p4 = handlers.MakePairTB(tb, app, "WdrawD")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3, p4)
			comp.Set("target_matches", 2)
			comp.Set("open_assignments", 4)
			require.NoError(tb, app.Save(comp))
			compID = comp.Id
			released = []string{
				handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id,
				handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "pending").Id,
			}
			kept = []string{
				handlers.MakeMatchTB(tb, app, comp.Id, p2.Id, p4.Id, "pending").Id,
				handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending").Id,
			}
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range released {
			_, err := app.FindRecordById("matches", id)
			assert.Error(tb, err, "released match %s must be deleted", id)
		}
		matches, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0, map[string]any{"c": compID})
		require.NoError(tb, err)
		var freedRival []string
		for _, m := range matches {
			if slices.Contains(kept, m.Id) {
				continue
			}
			pair1, pair2 := m.GetString("pair1"), m.GetString("pair2")
			assert.NotEqual(tb, p1.Id, pair1, "match %s pairs the withdrawn pair", m.Id)
			assert.NotEqual(tb, p1.Id, pair2, "match %s pairs the withdrawn pair", m.Id)
			if pair1 == p2.Id || pair2 == p2.Id {
				freedRival = append(freedRival, m.GetString("pair1")+"/"+m.GetString("pair2"))
			}
		}
		assert.NotEmpty(tb, freedRival, "the top-up must give the freed opponent a new rival")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_LeveledSetsOfficialResultForPlayed: proposed, disputed and played matches get the
// official result; a scheduled match with a proposed result is kept, not released.
func TestWithdrawPair_LeveledSetsOfficialResultForPlayed(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var compID, proposedID, disputedID, playedID, scheduledID string
	s := withdrawScenario("POST withdraw-pair sets the official result on proposed, disputed and played leveled matches",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3)
			compID = comp.Id
			proposedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			handlers.MakeResultProposal(tb, app, proposedID, league.PlayersForPair(app, p2.Id)[0], "6-2 6-3")
			disputedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "disputed").Id
			played := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			played.Set("scores", "3-6 4-6")
			played.Set("winner", p2.Id)
			require.NoError(tb, app.Save(played))
			playedID = played.Id
			scheduledID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled").Id
			handlers.MakeResultProposal(tb, app, scheduledID, league.PlayersForPair(app, p3.Id)[0], "6-1 6-2")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		want := []struct{ label, id, winner, scores, review string }{
			{"proposed", proposedID, p2.Id, "0-6 0-6", ""},
			{"disputed", disputedID, p3.Id, "0-6 0-6", "walkover"},
			{"played", playedID, p2.Id, "0-6 0-6", ""},
			{"scheduled", scheduledID, p3.Id, "0-6 0-6", ""},
		}
		for _, w := range want {
			m := loadMatch(tb, app, w.id)
			assert.Equal(tb, "final", m.GetString("status"), w.label+" status")
			assert.Equal(tb, w.winner, m.GetString("winner"), w.label+" winner")
			assert.Equal(tb, w.scores, m.GetString("scores"), w.label+" scores")
			assert.Equal(tb, w.review, m.GetString("review_type"), w.label+" review_type")
		}
		assert.Zero(tb, releaseEventCount(tb, app, compID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_StandardLeagueReleasesNothing: a standard league never deletes a match.
func TestWithdrawPair_StandardLeagueReleasesNothing(t *testing.T) {
	t.Parallel()
	var p1, p2, p3 *core.Record
	var compID, pendingID, schedID string
	s := withdrawScenario("POST withdraw-pair deletes no match in a standard league",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 = handlers.MakePairTB(tb, app, "WdrawA")
			p2 = handlers.MakePairTB(tb, app, "WdrawB")
			p3 = handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			compID = comp.Id
			pendingID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			schedID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled").Id
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range []string{pendingID, schedID} {
			assert.Equal(tb, "final", loadMatch(tb, app, id).GetString("status"), "match %s", id)
		}
		assert.Zero(tb, releaseEventCount(tb, app, compID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_StandardSecondWithdrawalKeepsWalkover: the second pair of a walkover withdraws.
// The match stays the first withdrawal's result, so the earlier-withdrawn pair does not win.
func TestWithdrawPair_StandardSecondWithdrawalKeepsWalkover(t *testing.T) {
	t.Parallel()
	var pB *core.Record
	var matchID, scores string
	s := withdrawScenario("POST withdraw-pair keeps a walkover whose opponent already withdrew",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			pA := handlers.MakePairTB(tb, app, "WdrawA")
			pB = handlers.MakePairTB(tb, app, "WdrawB")
			pC := handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pA, pB, pC})
			comp.Set("withdrawn_pairs", []string{pA.Id})
			require.NoError(tb, app.Save(comp))
			m := handlers.MakeMatchTB(tb, app, comp.Id, pA.Id, pB.Id, "final")
			league.ApplyWalkoverResult(m, pB.Id, league.WalkoverScore(comp))
			require.NoError(tb, app.Save(m))
			matchID, scores = m.Id, m.GetString("scores")
			return comp.Id, pB.Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m := loadMatch(tb, app, matchID)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, pB.Id, m.GetString("winner"))
		assert.Equal(tb, "walkover", m.GetString("review_type"))
		assert.Equal(tb, scores, m.GetString("scores"))
		assert.Empty(tb, resultEventTexts(tb, app, matchID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffWalkoverWinnerWithdrawsRoundOneStands: a final round-one walkover stands when
// its winner withdraws; the unplayed round-two match gives the opponent a walkover.
func TestWithdrawPair_PlayoffWalkoverWinnerWithdrawsRoundOneStands(t *testing.T) {
	t.Parallel()
	var pairs [4]*core.Record
	var firstID, nextID string
	s := withdrawScenario("POST withdraw-pair keeps a final round-one walkover and walks over round two",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			for i := range pairs {
				pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("WdrawW%d", i+1))
			}
			comp := makePlayoffComp(tb, app, pairs[:], nil)
			comp.Set("withdrawn_pairs", []string{pairs[0].Id})
			require.NoError(tb, app.Save(comp))
			first := handlers.MakeMatchTB(tb, app, comp.Id, pairs[0].Id, pairs[1].Id, "final")
			first.Set("round_number", 1)
			league.ApplyWalkoverResult(first, pairs[1].Id, league.WalkoverScore(comp))
			require.NoError(tb, app.Save(first))
			next := handlers.MakeMatchTB(tb, app, comp.Id, pairs[1].Id, pairs[2].Id, "pending")
			next.Set("round_number", 2)
			require.NoError(tb, app.Save(next))
			firstID, nextID = first.Id, next.Id
			return comp.Id, pairs[1].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		first := loadMatch(tb, app, firstID)
		assert.Equal(tb, "final", first.GetString("status"))
		assert.Equal(tb, pairs[1].Id, first.GetString("winner"))
		assert.Equal(tb, "walkover", first.GetString("review_type"))
		assert.Equal(tb, "0-6 0-6", first.GetString("scores"))
		assert.Empty(tb, resultEventTexts(tb, app, firstID))

		next := loadMatch(tb, app, nextID)
		assert.Equal(tb, "final", next.GetString("status"))
		assert.Equal(tb, pairs[2].Id, next.GetString("winner"))
		assert.Equal(tb, "walkover", next.GetString("review_type"))
		assert.Equal(tb, []string{
			"Incomparecencia — WdrawW2 se ha retirado de la competición",
		}, resultEventTexts(tb, app, nextID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_PlayoffPlayedNextRoundStays: the round-two match of a walkover winner already
// played (final) when that winner withdraws. Both the round-one walkover and the played match stand.
func TestWithdrawPair_PlayoffPlayedNextRoundStays(t *testing.T) {
	t.Parallel()
	var pairs [4]*core.Record
	var firstID, nextID string
	s := withdrawScenario("POST withdraw-pair leaves a played round-two match when its round-one walkover is voided",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			for i := range pairs {
				pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("WdrawE%d", i+1))
			}
			comp := makePlayoffComp(tb, app, pairs[:], nil)
			comp.Set("withdrawn_pairs", []string{pairs[0].Id})
			require.NoError(tb, app.Save(comp))
			first := handlers.MakeMatchTB(tb, app, comp.Id, pairs[0].Id, pairs[1].Id, "final")
			first.Set("round_number", 1)
			league.ApplyWalkoverResult(first, pairs[1].Id, league.WalkoverScore(comp))
			require.NoError(tb, app.Save(first))
			firstID = first.Id
			next := handlers.MakeMatchTB(tb, app, comp.Id, pairs[1].Id, pairs[2].Id, "final")
			next.Set("round_number", 2)
			next.Set("scores", "6-2 6-3")
			next.Set("winner", pairs[1].Id)
			require.NoError(tb, app.Save(next))
			nextID = next.Id
			return comp.Id, pairs[1].Id
		})
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		first := loadMatch(tb, app, firstID)
		assert.Equal(tb, "final", first.GetString("status"))
		assert.Equal(tb, pairs[1].Id, first.GetString("winner"))
		assert.Equal(tb, "walkover", first.GetString("review_type"))
		assert.Empty(tb, resultEventTexts(tb, app, firstID))

		next := loadMatch(tb, app, nextID)
		assert.Equal(tb, "final", next.GetString("status"))
		assert.Equal(tb, pairs[1].Id, next.GetString("winner"))
		assert.Equal(tb, "6-2 6-3", next.GetString("scores"))
		assert.Empty(tb, next.GetString("review_type"))
		assert.Empty(tb, resultEventTexts(tb, app, nextID))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// flashMessage is the decoded text of the flash cookie a response sets, or "" when it sets none.
func flashMessage(tb testing.TB, res *http.Response) string {
	tb.Helper()
	for _, c := range res.Cookies() {
		if c.Name == "flash_msg" {
			msg, err := url.QueryUnescape(c.Value)
			require.NoError(tb, err)
			return msg
		}
	}
	return ""
}

// TestWithdrawPair_RefusesPairOutsideCompetition: a pair that is not in the competition cannot be withdrawn.
func TestWithdrawPair_RefusesPairOutsideCompetition(t *testing.T) {
	t.Parallel()
	var matchID string
	var compID string
	var outsiderID string
	s := withdrawScenario("POST withdraw-pair refuses a pair outside the competition",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 := handlers.MakePairTB(tb, app, "WdrawA")
			p2 := handlers.MakePairTB(tb, app, "WdrawB")
			outsider := handlers.MakePairTB(tb, app, "WdrawOut")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			matchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
			compID = comp.Id
			outsiderID = outsider.Id
			return comp.Id, outsider.Id
		})
	s.ExpectedStatus = http.StatusOK
	s.ExpectedContent = []string{"La pareja no pertenece a esta competición"}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Empty(tb, res.Header.Get("HX-Redirect"))
		assert.Equal(tb, "pending", loadMatch(tb, app, matchID).GetString("status"))
		comp, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.NotContains(tb, comp.GetStringSlice("withdrawn_pairs"), outsiderID)
	}
	s.Test(t)
}

// TestWithdrawPair_FlashCountsOnlyWalkovers: a standard league counts only the walkovers it records,
// and names no released or voided matches.
func TestWithdrawPair_FlashCountsOnlyWalkovers(t *testing.T) {
	t.Parallel()
	s := withdrawScenario("POST withdraw-pair flash counts only the recorded walkovers",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 := handlers.MakePairTB(tb, app, "WdrawA")
			p2 := handlers.MakePairTB(tb, app, "WdrawB")
			p3 := handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "WdrawA retirada (2 partidos registrados por incomparecencia)", flashMessage(tb, res))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_FlashCountsReleasedMatches: a leveled league names the matches it released.
func TestWithdrawPair_FlashCountsReleasedMatches(t *testing.T) {
	t.Parallel()
	s := withdrawScenario("POST withdraw-pair flash names the released leveled matches",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 := handlers.MakePairTB(tb, app, "WdrawA")
			p2 := handlers.MakePairTB(tb, app, "WdrawB")
			p3 := handlers.MakePairTB(tb, app, "WdrawC")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3)
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, "scheduled")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "WdrawA retirada (0 partidos registrados por incomparecencia, 2 partidos sin resultado liberados)",
			flashMessage(tb, res))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_FlashSingularWalkover: one recorded walkover is named in the singular.
func TestWithdrawPair_FlashSingularWalkover(t *testing.T) {
	t.Parallel()
	s := withdrawScenario("POST withdraw-pair flash names one recorded walkover in the singular",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 := handlers.MakePairTB(tb, app, "WdrawA")
			p2 := handlers.MakePairTB(tb, app, "WdrawB")
			p3 := handlers.MakePairTB(tb, app, "WdrawC")
			comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3})
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "WdrawA retirada (1 partido registrado por incomparecencia)", flashMessage(tb, res))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestWithdrawPair_FlashSingularReleasedMatch: one released leveled match is named in the singular.
func TestWithdrawPair_FlashSingularReleasedMatch(t *testing.T) {
	t.Parallel()
	s := withdrawScenario("POST withdraw-pair flash names one released leveled match in the singular",
		func(tb testing.TB, app *tests.TestApp) (string, string) {
			p1 := handlers.MakePairTB(tb, app, "WdrawA")
			p2 := handlers.MakePairTB(tb, app, "WdrawB")
			p3 := handlers.MakePairTB(tb, app, "WdrawC")
			comp := leveledWithdrawComp(tb, app, p1, p2, p3)
			handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			return comp.Id, p1.Id
		})
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "WdrawA retirada (0 partidos registrados por incomparecencia, 1 partido sin resultado liberado)",
			flashMessage(tb, res))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// -- Task 8: leveled-league fields (target_matches, open_assignments) -------

func TestLeveledFieldsPersistOnCreate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions persists target_matches and open_assignments",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=LeveledNew&type=league&target_matches=4&open_assignments=2")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'LeveledNew'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		assert.Equal(tb, 4, comps[0].GetInt("target_matches"))
		assert.Equal(tb, 2, comps[0].GetInt("open_assignments"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + newestCompetitionID(app) })
	s.Test(t)
}

func TestLeveledFieldsPersistOnUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id} persists target_matches and open_assignments",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := handlers.MakeCompetitionTB(tb, app, "league", nil)
		compID = comp.Id
		s.URL = "/admin/competitions/" + comp.Id
		s.Body = strings.NewReader("name=LeveledUpd&type=league&target_matches=3&open_assignments=2")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, 3, c.GetInt("target_matches"))
		assert.Equal(tb, 2, c.GetInt("open_assignments"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// TestUpdateCompetition_KeepsTargetMatchesWhenFixturesExist pins that saving
// the edit form on a leveled competition that already has fixtures must not
// touch target_matches — the field is rendered disabled once fixtures exist
// (competition-detail.html), so the browser never submits it; reading it
// with a default here would silently zero it out and demote the competition
// to a plain round-robin league. Fails on the pre-fix code, which always
// read target_matches from the form and defaulted to 0 when absent.
func TestUpdateCompetition_KeepsTargetMatchesWhenFixturesExist(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id} with fixtures keeps target_matches unchanged",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair1 := handlers.MakePairTB(tb, app, "TM A")
		pair2 := handlers.MakePairTB(tb, app, "TM B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{pair1, pair2})
		comp.Set("target_matches", 10)
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, pair1.Id, pair2.Id, "pending")
		compID = comp.Id

		s.URL = "/admin/competitions/" + comp.Id
		// No target_matches field — matches the disabled-input form the
		// browser actually submits once fixtures exist.
		s.Body = strings.NewReader("name=RenamedWithFixtures&type=league")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		assert.Equal(tb, "RenamedWithFixtures", c.GetString("name"))
		assert.Equal(tb, 10, c.GetInt("target_matches"), "target_matches must survive an edit once fixtures exist")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestLeveledFieldsEmptyDefaultsToZero(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions — empty leveled fields default to 0",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=LeveledZero&type=league")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		comps, err := app.FindRecordsByFilter("competitions", "name = 'LeveledZero'", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, comps, 1)
		assert.Equal(tb, 0, comps[0].GetInt("target_matches"))
		assert.Equal(tb, 0, comps[0].GetInt("open_assignments"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + newestCompetitionID(app) })
	s.Test(t)
}

func TestLeveledFieldsInvalidRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions — non-numeric target_matches rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"debe ser un número"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=BadTarget&type=league&target_matches=abc")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestLeveledFieldsNegativeRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions — negative target_matches rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no puede ser negativo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=NegTarget&type=league&target_matches=-1")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestLeveledPlayTwiceRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions — play_twice with target_matches>0 rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Una liga nivelada no puede ser a doble vuelta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=PlayTwiceLvl&type=league&target_matches=4&play_twice=on")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestLeveledOpenGTTargetRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions — open_assignments > target_matches rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Partidos abiertos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.URL = "/admin/competitions"
		s.Body = strings.NewReader("name=OpenGTTarget&type=league&target_matches=2&open_assignments=5")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestLeveledChangedWithMatchesRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/competitions/{id} — changing target_matches with existing matches rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No se puede cambiar con el calendario generado"},
	}
	var compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "LkA")
		p2 := handlers.MakePairTB(tb, app, "LkB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("target_matches", 3)
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		// Create a match so HasFixtures = true
		handlers.MakeMatchTB(tb, app, compID, p1.Id, p2.Id, "pending")
		s.URL = "/admin/competitions/" + compID
		s.Body = strings.NewReader("name=LkComp&type=league&target_matches=4")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestRoundRobinDefaultMatchCount(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST generate — round-robin (no target_matches) produces n*(n-1)/2 matches",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	var n int
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		n = 5
		pairs := make([]*core.Record, n)
		for i := range pairs {
			pairs[i] = handlers.MakePairTB(tb, app, "RR")
		}
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		compID = comp.Id
		s.URL = "/admin/competitions/" + compID + "/generate"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		matches, err := app.FindRecordsByFilter("matches",
			"competition = {:c}", "", 0, 0, map[string]any{"c": compID})
		require.NoError(tb, err)
		expected := n * (n - 1) / 2
		assert.Equal(tb, expected, len(matches), "round-robin: n*(n-1)/2 matches")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

// -- Level + balls handlers --------------------------------------------------

func TestSetPairLevel(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/pairs/{id}/level saves the level",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "LvA")
		pairID = p1.Id
		s.URL = "/admin/pairs/" + pairID + "/level"
		s.Body = strings.NewReader("level=advanced")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("pairs", pairID)
		require.NoError(tb, err)
		assert.Equal(tb, "advanced", p.GetString("level"))
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/pairs"))
	s.Test(t)
}

func TestSetPairLevel_Locked(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/pairs/{id}/level — locked when pair has leveled matches",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No se puede cambiar el nivel"},
	}
	var pairID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "LkLvA")
		p2 := handlers.MakePairTB(tb, app, "LkLvB")
		p3 := handlers.MakePairTB(tb, app, "LkLvC")
		p4 := handlers.MakePairTB(tb, app, "LkLvD")
		pairID = p1.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		comp.Set("target_matches", 2)
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/admin/pairs/" + pairID + "/level"
		s.Body = strings.NewReader("level=advanced")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestToggleBalls(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/balls toggles balls_status",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, pairID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		adminID = admin.Id
		p1 := handlers.MakePairTB(tb, app, "BlA")
		p2 := handlers.MakePairTB(tb, app, "BlB")
		pairID = p1.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + compID + "/balls"
		s.Body = strings.NewReader("pair_id=" + pairID)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		status := map[string]bool{}
		require.NoError(tb, c.UnmarshalJSONField("balls_status", &status))
		assert.True(tb, status[pairID])
		by := map[string]string{}
		require.NoError(tb, c.UnmarshalJSONField("balls_delivered_by", &by))
		assert.Equal(tb, adminID, by[pairID])
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestToggleBallsAll(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/balls-all sets every pair delivered",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID string
	var p1ID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "BlAllA")
		p2 := handlers.MakePairTB(tb, app, "BlAllB")
		p1ID, p2ID = p1.Id, p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		compID = comp.Id
		s.URL = "/admin/competitions/" + compID + "/balls-all"
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		status := map[string]bool{}
		require.NoError(tb, c.UnmarshalJSONField("balls_status", &status))
		assert.True(tb, status[p1ID])
		assert.True(tb, status[p2ID])
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}

func TestRemovePair_ClearsBolas(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/competitions/{id}/remove-pair clears balls entries",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "RmBlA")
		p2 := handlers.MakePairTB(tb, app, "RmBlB")
		p1ID = p1.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("balls_status", map[string]bool{p1.Id: true, p2.Id: true})
		comp.Set("balls_delivered_at", map[string]string{p1.Id: time.Now().Format(time.RFC3339)})
		comp.Set("balls_delivered_by", map[string]string{p1.Id: admin.Id})
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		s.URL = "/admin/competitions/" + compID + "/remove-pair"
		s.Body = strings.NewReader("pair_id=" + p1ID)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		c, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		status := map[string]bool{}
		require.NoError(tb, c.UnmarshalJSONField("balls_status", &status))
		_, stillThere := status[p1ID]
		assert.False(tb, stillThere, "removed pair must not remain in balls_status")
		at := map[string]string{}
		require.NoError(tb, c.UnmarshalJSONField("balls_delivered_at", &at))
		_, atThere := at[p1ID]
		assert.False(tb, atThere, "removed pair must not remain in balls_delivered_at")
	}
	handlers.ExpectRedirect(s, func(core.App) string { return competitionDetailURL(s.URL) })
	s.Test(t)
}
