package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

// racer is one concurrent request: who posts, to which path, with what form.
type racer struct {
	user *core.Record
	path string
	form string
}

// fireTogether releases every racer at the same instant and waits for all.
func fireTogether(tb testing.TB, mux http.Handler, racers []racer) {
	tb.Helper()
	fireCollect(tb, mux, racers)
}

// fireCollect is fireTogether that also returns each racer's response, in order.
func fireCollect(tb testing.TB, mux http.Handler, racers []racer) []*httptest.ResponseRecorder {
	tb.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	out := make([]*httptest.ResponseRecorder, len(racers))
	for i, r := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, r.path, strings.NewReader(r.form))
			for k, v := range handlers.AuthHeaders(tb, r.user) {
				req.Header.Set(k, v)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			<-start
			out[i] = httptest.NewRecorder()
			mux.ServeHTTP(out[i], req)
		}()
	}
	close(start)
	wg.Wait()
	return out
}

func pendingResultAuthors(tb testing.TB, app core.App, matchID string) []string {
	tb.Helper()
	pending, err := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
		"", 0, 0, map[string]any{"mid": matchID})
	require.NoError(tb, err)
	var authors []string
	for _, p := range pending {
		authors = append(authors, p.GetString("author"))
	}
	return authors
}

// Both pairs submitting at once must leave exactly one pending proposal: a
// second one is a deadlock the players cannot resolve by accepting.
func TestMatchSubmit_ConcurrentSides_LeaveOnePendingProposal(t *testing.T) {
	t.Parallel()
	const matches = 12
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "concurrent submits from both pairs and an admin",
		Method:          http.MethodGet,
		URL:             "/api/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"API is healthy"},
	}
	var ids []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "Race A")
		p2 := handlers.MakePairTB(tb, app, "Race B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		u1, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		u2, err := app.FindRecordById("users", p2.GetString("player1"))
		require.NoError(tb, err)

		var racers []racer
		for range matches {
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			m.Set("date", "2026-09-01")
			m.Set("club", "Padel 360")
			require.NoError(tb, app.Save(m))
			ids = append(ids, m.Id)
			path := "/match/" + m.Id + "/submit"
			racers = append(racers,
				racer{u1, path, "scores=6-3+6-4"},
				racer{u2, path, "scores=3-6+4-6"},
				racer{admin, path, "scores=6-0+6-0"})
		}
		mux, err := e.Router.BuildMux()
		require.NoError(tb, err)
		fireTogether(tb, mux, racers)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range ids {
			assert.Len(tb, pendingResultAuthors(tb, app, id), 1,
				"match %s: exactly one side may hold a pending proposal", id)
		}
	}
	s.Test(t)
}

// A rejection (counter-proposal) racing the proposer's own correction must not
// leave two pending proposals either.
func TestResultCounterVsCorrection_Concurrent_LeaveOnePendingProposal(t *testing.T) {
	t.Parallel()
	const matches = 12
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "concurrent counter-proposal and correction",
		Method:          http.MethodGet,
		URL:             "/api/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"API is healthy"},
	}
	var ids []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Cnt A")
		p2 := handlers.MakePairTB(tb, app, "Cnt B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		u1, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		u2, err := app.FindRecordById("users", p2.GetString("player1"))
		require.NoError(tb, err)

		var racers []racer
		for range matches {
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			m.Set("date", "2026-09-01")
			m.Set("club", "Padel 360")
			require.NoError(tb, app.Save(m))
			ids = append(ids, m.Id)
			prop := handlers.MakeResultProposal(tb, app, m.Id, u1.Id, "6-3 6-4")
			racers = append(racers,
				racer{u1, "/match/" + m.Id + "/correct", "scores=6-2+6-2"},
				racer{u2, "/match/" + m.Id + "/thread/proposal/" + prop.Id + "/respond", "action=reject&counter_scores=3-6+4-6&reason=Otro"})
		}
		mux, err := e.Router.BuildMux()
		require.NoError(tb, err)
		fireTogether(tb, mux, racers)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range ids {
			assert.Len(tb, pendingResultAuthors(tb, app, id), 1,
				"match %s: exactly one pending proposal must survive", id)
		}
	}
	s.Test(t)
}
