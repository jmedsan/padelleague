package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

const (
	msgWithdrawn  = "Tu pareja se ha retirado de esta competición"
	msgNotCaptain = "Solo el capitán puede registrar resultados"
)

// guardFixture is the match a player POST route acts on: the actor is
// pair1's player1, the rival is pair2's player1.
type guardFixture struct {
	pair1, pair2, comp, match *core.Record
	actor, rival              string
}

// playerRoute is one player POST route with the guards it must enforce.
// setup returns the URL and form body for a request that succeeds when
// no guard fires.
type playerRoute struct {
	name             string
	captainGated     bool
	withdrawnBlocked bool
	setup            func(tb testing.TB, app core.App, f guardFixture) (url, body string)
}

func playerRoutes() []playerRoute {
	return []playerRoute{
		{
			name: "match submit", captainGated: true, withdrawnBlocked: true,
			setup: func(_ testing.TB, _ core.App, f guardFixture) (string, string) {
				return "/match/" + f.match.Id + "/submit", "scores=6-3+6-4"
			},
		},
		{
			name: "match correct", captainGated: true, withdrawnBlocked: true,
			setup: func(tb testing.TB, app core.App, f guardFixture) (string, string) {
				f.match.Set("submitted_by", f.actor)
				f.match.Set("submitted_at", time.Now().UTC().Format(time.RFC3339))
				require.NoError(tb, app.Save(f.match))
				makeResultProposal(tb, app, f.match.Id, f.actor, "6-3 6-4")
				return "/match/" + f.match.Id + "/correct", "scores=6-4+6-3"
			},
		},
		{
			name: "match report-unplayed", captainGated: true, withdrawnBlocked: true,
			setup: func(_ testing.TB, _ core.App, f guardFixture) (string, string) {
				return "/match/" + f.match.Id + "/report-unplayed", "reason=no+vinieron"
			},
		},
		{
			name: "match cancel-date", captainGated: false, withdrawnBlocked: true,
			setup: func(_ testing.TB, _ core.App, f guardFixture) (string, string) {
				return "/match/" + f.match.Id + "/cancel-date", "reason=lluvia"
			},
		},
		{
			// Chat stays open to withdrawn players by design.
			name: "thread message", captainGated: false, withdrawnBlocked: false,
			setup: func(_ testing.TB, _ core.App, f guardFixture) (string, string) {
				return "/match/" + f.match.Id + "/thread/message", "content=hola&type=chat"
			},
		},
		{
			name: "thread proposal", captainGated: false, withdrawnBlocked: true,
			setup: func(_ testing.TB, _ core.App, f guardFixture) (string, string) {
				return "/match/" + f.match.Id + "/thread/proposal",
					"date=2026-09-15&time=18:00&venue_text=Club+Test"
			},
		},
		{
			name: "thread respond to date proposal", captainGated: false, withdrawnBlocked: true,
			setup: func(tb testing.TB, app core.App, f guardFixture) (string, string) {
				msg := makeSchedulingProposal(tb, app, f.match.Id, f.rival)
				return fmt.Sprintf("/match/%s/thread/proposal/%s/respond", f.match.Id, msg.Id), "action=accept"
			},
		},
		{
			name: "thread respond to result proposal", captainGated: true, withdrawnBlocked: true,
			setup: func(tb testing.TB, app core.App, f guardFixture) (string, string) {
				msg := makeResultProposal(tb, app, f.match.Id, f.rival, "6-3 6-4")
				return fmt.Sprintf("/match/%s/thread/proposal/%s/respond", f.match.Id, msg.Id), "action=accept"
			},
		},
		{
			name: "thread withdraw proposal", captainGated: false, withdrawnBlocked: true,
			setup: func(tb testing.TB, app core.App, f guardFixture) (string, string) {
				msg := makeSchedulingProposal(tb, app, f.match.Id, f.actor)
				return fmt.Sprintf("/match/%s/thread/proposal/%s/withdraw", f.match.Id, msg.Id), ""
			},
		},
		{
			name: "thread reject-and-counter", captainGated: false, withdrawnBlocked: true,
			setup: func(tb testing.TB, app core.App, f guardFixture) (string, string) {
				msg := makeSchedulingProposal(tb, app, f.match.Id, f.rival)
				return fmt.Sprintf("/match/%s/thread/proposal/%s/reject-and-counter", f.match.Id, msg.Id),
					"date=2026-09-16&time=19:00&venue_text=Club+Test"
			},
		},
	}
}

// guardScenario mutates the fixture so one guard should fire, and names the
// alert text expected when the route enforces that guard.
type guardScenario struct {
	name    string
	blocked func(r playerRoute) bool
	alert   string
	apply   func(tb testing.TB, app core.App, f guardFixture)
}

func guardScenarios() []guardScenario {
	return []guardScenario{
		{
			name:    "participant passes",
			blocked: func(playerRoute) bool { return false },
			apply:   func(testing.TB, core.App, guardFixture) {},
		},
		{
			name:    "withdrawn pair",
			blocked: func(r playerRoute) bool { return r.withdrawnBlocked },
			alert:   msgWithdrawn,
			apply: func(tb testing.TB, app core.App, f guardFixture) {
				f.comp.Set("withdrawn_pairs", []string{f.pair1.Id})
				require.NoError(tb, app.Save(f.comp))
			},
		},
		{
			name:    "non-captain",
			blocked: func(r playerRoute) bool { return r.captainGated },
			alert:   msgNotCaptain,
			apply: func(tb testing.TB, app core.App, f guardFixture) {
				f.pair1.Set("captain", f.pair1.GetString("player2"))
				require.NoError(tb, app.Save(f.pair1))
			},
		},
	}
}

// TestPlayerRouteGuards enumerates every player POST route and pins whether
// it is captain-gated and whether a withdrawn pair is blocked.
func TestPlayerRouteGuards(t *testing.T) {
	t.Parallel()
	for _, route := range playerRoutes() {
		for _, sc := range guardScenarios() {
			t.Run(route.name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				s := &tests.ApiScenario{
					TestAppFactory: testAppFactory,
					Name:           route.name + " / " + sc.name,
					Method:         http.MethodPost,
					ExpectedStatus: http.StatusNoContent,
				}
				if sc.blocked(route) {
					s.ExpectedStatus = http.StatusOK
					s.ExpectedContent = []string{"alert-error", sc.alert}
				} else {
					s.NotExpectedContent = []string{"alert-error"}
				}
				s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
					setupAllRoutes(tb, app, e)
					f := newGuardFixture(tb, app, route.name)
					url, body := route.setup(tb, app, f)
					sc.apply(tb, app, f)
					s.URL = url
					s.Body = strings.NewReader(body)
					user, err := app.FindRecordById("users", f.actor)
					require.NoError(tb, err)
					hdrs := authHeaders(tb, user)
					hdrs["Content-Type"] = "application/x-www-form-urlencoded"
					s.Headers = hdrs
				}
				s.Test(t)
			})
		}
	}
}

func newGuardFixture(tb testing.TB, app core.App, label string) guardFixture {
	tb.Helper()
	p1 := makePairTB(tb, app, "Guard "+label+" A")
	p2 := makePairTB(tb, app, "Guard "+label+" B")
	comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
	match.Set("date", "2026-09-01")
	match.Set("time", "18:00")
	match.Set("club", "Padel 360")
	require.NoError(tb, app.Save(match))
	return guardFixture{
		pair1: p1, pair2: p2, comp: comp, match: match,
		actor: p1.GetString("player1"), rival: p2.GetString("player1"),
	}
}

func makeSchedulingProposal(tb testing.TB, app core.App, matchID, authorID string) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("author", authorID)
	msg.Set("type", "scheduling_proposal")
	msg.Set("proposal_data", `{"date":"2026-09-15","time":"18:00","venue_name":"Club Test","venue_id":"","venue_text":""}`)
	msg.Set("proposal_status", "pending")
	require.NoError(tb, app.Save(msg))
	return msg
}
