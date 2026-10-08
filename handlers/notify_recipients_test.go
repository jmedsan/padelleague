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

// assertRecipients asserts that exactly the wanted users hold a notification
// with the title: no extra recipient, none missing.
func assertRecipients(t testing.TB, app core.App, title string, want ...string) {
	t.Helper()
	recs, err := app.FindRecordsByFilter("notifications", "title = {:title}", "", 0, 0, map[string]any{"title": title})
	require.NoError(t, err)
	got := map[string]bool{}
	for _, r := range recs {
		got[r.GetString("user")] = true
	}
	wantSet := map[string]bool{}
	for _, id := range want {
		wantSet[id] = true
	}
	assert.Equalf(t, wantSet, got, "recipients of %q", title)
}

// recipientCase is one match-thread action whose notification must reach
// everyone in the match but the actor, partner included.
type recipientCase struct {
	name string
	// seed prepares the thread and returns the action's URL, form body and actor.
	seed   func(tb testing.TB, app core.App, m *core.Record) (url, body string, actor *core.Record)
	titles []string
}

// Every player-made thread action tells the whole match but the actor, the
// same recipients as a chat message: the actor's own partner must know what
// their pair just did.
func TestThreadActionsNotifyEveryoneButTheActor(t *testing.T) {
	t.Parallel()
	const proposal = "date=2027-09-20&time=19:00&venue_text=Club+Test"
	cases := []recipientCase{
		{
			name: "withdraw a date proposal",
			seed: func(tb testing.TB, app core.App, m *core.Record) (string, string, *core.Record) {
				author := reload(tb, app, "users", firstPlayer(tb, app, m, "pair1"))
				p := schedulingProposal(tb, app, m.Id, author.Id, "2027-09-15")
				return "/match/" + m.Id + "/thread/proposal/" + p.Id + "/withdraw", "", author
			},
			titles: []string{"Propuesta retirada"},
		},
		{
			name: "reject a date proposal",
			seed: func(tb testing.TB, app core.App, m *core.Record) (string, string, *core.Record) {
				p := schedulingProposal(tb, app, m.Id, firstPlayer(tb, app, m, "pair1"), "2027-09-15")
				return respondPath(m.Id, p.Id), "action=reject&rejection_reason=No+puedo", reload(tb, app, "users", firstPlayer(tb, app, m, "pair2"))
			},
			titles: []string{"Propuesta rechazada"},
		},
		{
			name: "reject a date proposal with a counter-proposal",
			seed: func(tb testing.TB, app core.App, m *core.Record) (string, string, *core.Record) {
				p := schedulingProposal(tb, app, m.Id, firstPlayer(tb, app, m, "pair1"), "2027-09-15")
				return "/match/" + m.Id + "/thread/proposal/" + p.Id + "/reject-and-counter",
					proposal + "&rejection_reason=No+puedo", reload(tb, app, "users", firstPlayer(tb, app, m, "pair2"))
			},
			titles: []string{"Propuesta rechazada", "Propuesta de fecha"},
		},
		{
			name: "accept a result proposal",
			seed: func(tb testing.TB, app core.App, m *core.Record) (string, string, *core.Record) {
				p := handlers.MakeResultProposal(tb, app, m.Id, firstPlayer(tb, app, m, "pair1"), "6-3 6-4")
				return respondPath(m.Id, p.Id), "action=accept", reload(tb, app, "users", firstPlayer(tb, app, m, "pair2"))
			},
			titles: []string{"Resultado confirmado"},
		},
		{
			name: "counter a result proposal",
			seed: func(tb testing.TB, app core.App, m *core.Record) (string, string, *core.Record) {
				p := handlers.MakeResultProposal(tb, app, m.Id, firstPlayer(tb, app, m, "pair1"), "6-3 6-4")
				return respondPath(m.Id, p.Id), "action=reject&counter_scores=3-6+4-6", reload(tb, app, "users", firstPlayer(tb, app, m, "pair2"))
			},
			titles: []string{"Contrapropuesta recibida"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runRecipientCase(t, tc) })
	}
}

func runRecipientCase(t *testing.T, tc recipientCase) {
	t.Helper()
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           tc.name,
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var all []string
	var actorID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Rec A")
		p2 := handlers.MakePairTB(tb, app, "Rec B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		m.Set("date", "2026-09-01")
		m.Set("club", "Padel 360")
		require.NoError(tb, app.Save(m))
		all = []string{p1.GetString("player1"), p1.GetString("player2"), p2.GetString("player1"), p2.GetString("player2")}

		url, body, actor := tc.seed(tb, app, m)
		actorID = actor.Id
		s.URL = url
		s.Body = strings.NewReader(body)
		hdrs := handlers.AuthHeaders(tb, actor)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		var want []string
		for _, id := range all {
			if id != actorID {
				want = append(want, id)
			}
		}
		for _, title := range tc.titles {
			assertRecipients(tb, app, title, want...)
		}
	}
	s.Test(t)
}

// firstPlayer returns the user id of player1 in the match's pair ("pair1"/"pair2").
func firstPlayer(tb testing.TB, app core.App, m *core.Record, pairField string) string {
	tb.Helper()
	return reload(tb, app, "pairs", m.GetString(pairField)).GetString("player1")
}
