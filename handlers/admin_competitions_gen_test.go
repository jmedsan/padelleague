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
