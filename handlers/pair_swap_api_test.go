package handlers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// swapFixture is a pending match between two pairs where admin later replaces
// pair A's player1 with a new user. The message author (orig) is no longer in
// the pair once swap runs; the replacement must act as pair A, never as the
// author's own pair's rival.
type swapFixture struct {
	pairA, pairB, comp, match *core.Record
	orig                      string
	rival                     string
}

func newSwapFixture(tb testing.TB, app core.App, tag string) swapFixture {
	tb.Helper()
	a := handlers.MakePairTB(tb, app, tag+" A")
	b := handlers.MakePairTB(tb, app, tag+" B")
	comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{a, b})
	match := handlers.MakeMatchTB(tb, app, comp.Id, a.Id, b.Id, "pending")
	return swapFixture{
		pairA: a, pairB: b, comp: comp, match: match,
		orig: a.GetString("player1"), rival: b.GetString("player1"),
	}
}

// swap replaces pair A's player1 and returns the new user.
func (f swapFixture) swap(tb testing.TB, app core.App) *core.Record {
	tb.Helper()
	repl := handlers.MakeUserTB(tb, app, "Replacement "+f.pairA.Id, "repl-"+f.pairA.Id+"@test.local")
	f.pairA.Set("player1", repl.Id)
	require.NoError(tb, app.Save(f.pairA))
	return repl
}

func formHeaders(tb testing.TB, user *core.Record) map[string]string {
	tb.Helper()
	hdrs := handlers.AuthHeaders(tb, user)
	hdrs["Content-Type"] = "application/x-www-form-urlencoded"
	return hdrs
}

func TestPairSwap_ReplacementCanWithdrawOwnPairsProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "replacement withdraws the proposal made before the swap",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		f := newSwapFixture(tb, app, "SwWd")
		msg := handlers.MakeProposal(tb, app, f.match.Id, f.orig)
		msgID = msg.Id
		repl := f.swap(tb, app)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", f.match.Id, msg.Id)
		s.Headers = formHeaders(tb, repl)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		msg, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "withdrawn", msg.GetString("proposal_status"))
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestPairSwap_ReplacementCannotCounterOwnPairsProposal(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "replacement cannot counter the proposal made by their own pair",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propia propuesta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		f := newSwapFixture(tb, app, "SwCt")
		msg := handlers.MakeProposal(tb, app, f.match.Id, f.orig)
		repl := f.swap(tb, app)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/reject-and-counter", f.match.Id, msg.Id)
		s.Body = strings.NewReader("date=2027-09-16&time=19:00&venue_text=Club+Test")
		s.Headers = formHeaders(tb, repl)
	}
	s.Test(t)
}

func TestPairSwap_RivalCanStillCounterProposalAfterSwap(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "rival counters a proposal whose author left the pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		f := newSwapFixture(tb, app, "SwRv")
		msg := handlers.MakeProposal(tb, app, f.match.Id, f.orig)
		f.swap(tb, app)
		rival, err := app.FindRecordById("users", f.rival)
		require.NoError(tb, err)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/reject-and-counter", f.match.Id, msg.Id)
		s.Body = strings.NewReader("date=2027-09-16&time=19:00&venue_text=Club+Test")
		s.Headers = formHeaders(tb, rival)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) + "?scroll=mensajes" })
	s.Test(t)
}

func TestPairSwap_ReplacementCanCorrectOwnPairsSubmission(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "replacement corrects the result their pair submitted before the swap",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		f := newSwapFixture(tb, app, "SwCr")
		matchID = f.match.Id
		f.match.Set("status", "scheduled")
		f.match.Set("submitted_by", f.orig)
		f.match.SetRaw("submitted_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
		require.NoError(tb, app.Save(f.match))
		handlers.MakeResultProposal(tb, app, f.match.Id, f.orig, "6-3 6-4")
		repl := f.swap(tb, app)
		s.URL = "/match/" + f.match.Id + "/correct"
		s.Body = strings.NewReader("scores=6-4+6-3")
		s.Headers = formHeaders(tb, repl)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		pending, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
			"", 0, 0, map[string]any{"mid": matchID})
		require.NoError(tb, err)
		require.Len(tb, pending, 1)
		assert.Equal(tb, "6-4 6-3", league.ParseProposalData(pending[0].GetString("proposal_data")).Scores)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestPairSwap_ReplacementCannotAcceptOwnPairsResult(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "replacement cannot accept the result their pair proposed",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"propia propuesta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		f := newSwapFixture(tb, app, "SwAc")
		msg := handlers.MakeResultProposal(tb, app, f.match.Id, f.orig, "6-3 6-4")
		repl := f.swap(tb, app)
		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/respond", f.match.Id, msg.Id)
		s.Body = strings.NewReader("action=accept")
		s.Headers = formHeaders(tb, repl)
	}
	s.Test(t)
}

func TestPairSwap_HomeActionsFollowThePairNotTheAuthor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		propose   func(tb testing.TB, app core.App, f swapFixture)
		viewer    func(f swapFixture, repl *core.Record) string
		wantShown bool
		text      string
	}{
		{
			name:      "scheduling proposal is not an action for the replacement",
			propose:   func(tb testing.TB, app core.App, f swapFixture) { handlers.MakeProposal(tb, app, f.match.Id, f.orig) },
			viewer:    func(_ swapFixture, repl *core.Record) string { return repl.Id },
			wantShown: false, text: "Propuesta de horario pendiente",
		},
		{
			name:      "scheduling proposal is an action for the rival",
			propose:   func(tb testing.TB, app core.App, f swapFixture) { handlers.MakeProposal(tb, app, f.match.Id, f.orig) },
			viewer:    func(f swapFixture, _ *core.Record) string { return f.rival },
			wantShown: true, text: "Propuesta de horario pendiente",
		},
		{
			name: "result proposal is not an action for the replacement",
			propose: func(tb testing.TB, app core.App, f swapFixture) {
				handlers.MakeResultProposal(tb, app, f.match.Id, f.orig, "6-3 6-4")
			},
			viewer:    func(_ swapFixture, repl *core.Record) string { return repl.Id },
			wantShown: false, text: "Responder resultado: 6-3 6-4",
		},
		{
			name: "result proposal is an action for the rival",
			propose: func(tb testing.TB, app core.App, f swapFixture) {
				handlers.MakeResultProposal(tb, app, f.match.Id, f.orig, "6-3 6-4")
			},
			viewer:    func(f swapFixture, _ *core.Record) string { return f.rival },
			wantShown: true, text: "Responder resultado: 6-3 6-4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory: handlers.TestAppFactory,
				Name:           tc.name,
				Method:         http.MethodGet,
				URL:            "/",
				ExpectedStatus: 200,
			}
			if tc.wantShown {
				s.ExpectedContent = []string{tc.text}
			} else {
				s.ExpectedContent = []string{"Liga Dale Fuerte"}
				s.NotExpectedContent = []string{tc.text}
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				f := newSwapFixture(tb, app, "SwHm")
				tc.propose(tb, app, f)
				repl := f.swap(tb, app)
				user, err := app.FindRecordById("users", tc.viewer(f, repl))
				require.NoError(tb, err)
				s.Headers = handlers.AuthHeaders(tb, user)
			}
			s.Test(t)
		})
	}
}
