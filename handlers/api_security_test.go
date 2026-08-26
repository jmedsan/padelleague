package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the PocketBase auto-generated record REST API directly
// (not the app's custom routes) to prove that a non-admin player — or an
// unauthenticated client — cannot mutate match results, escalate their own
// role, create accounts, delete matches, forge match messages, or inject
// notifications. Each corresponds to a live exploit found in the cycle-4
// review; the R-82 lock migration must keep every one at 403.

func jsonHeaders(extra map[string]string) map[string]string {
	h := map[string]string{"Content-Type": "application/json"}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func TestAPIMatchUpdateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	var matchID, winnerPairID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot PATCH a match via the record API",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"status":"final","scores":"6-0 6-0"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		winnerPairID = p1.Id
		s.URL = "/api/collections/matches/records/" + matchID
		s.Body = strings.NewReader(`{"status":"final","winner":"` + winnerPairID + `","scores":"6-0 6-0"}`)
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", m.GetString("status"))
		assert.Empty(tb, m.GetString("winner"))
	}
	s.Test(t)
}

func TestAPIMatchUpdateForbiddenForAnon(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "unauthenticated client cannot PATCH a match via the record API",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"scores":"0-6 0-6"}`),
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/api/collections/matches/records/" + matchID
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "pending", m.GetString("status"))
	}
	s.Test(t)
}

func TestAPIMatchDeleteForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	var matchID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot DELETE a match via the record API",
		Method:          http.MethodDelete,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		matchID = match.Id
		s.URL = "/api/collections/matches/records/" + matchID
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, player)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err, "match must still exist")
	}
	s.Test(t)
}

func TestAPIUserRoleEscalationForbidden(t *testing.T) {
	t.Parallel()
	var playerID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot self-escalate role to admin via the record API",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"role":"admin"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		player := makeUserTB(tb, app, "Player", "")
		playerID = player.Id
		s.URL = "/api/collections/users/records/" + playerID
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		u, err := app.FindRecordById("users", playerID)
		require.NoError(tb, err)
		assert.Equal(tb, "player", u.GetString("role"), "role must remain player")
	}
	s.Test(t)
}

func TestAPIAnonUserCreateForbidden(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "unauthenticated client cannot create a user (incl. admin) via the record API",
		Method:          http.MethodPost,
		URL:             "/api/collections/users/records",
		Body:            strings.NewReader(`{"email":"attacker@evil.com","password":"hunter2hunter2","passwordConfirm":"hunter2hunter2","display_name":"Attacker","role":"admin"}`),
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {}
	s.Test(t)
}

func TestAPIMatchMessageCreateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot create a match_messages record (forge admin_action) via the record API",
		Method:          http.MethodPost,
		URL:             "/api/collections/match_messages/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Body = strings.NewReader(`{"match":"` + match.Id + `","type":"admin_action","content":"forged","author":"` + player.Id + `"}`)
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPINotificationCreateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot inject a notification via the record API",
		Method:          http.MethodPost,
		URL:             "/api/collections/notifications/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		player := makeUserTB(tb, app, "Player", "")
		victim := makeUserTB(tb, app, "Victim", "")
		s.Body = strings.NewReader(`{"user":"` + victim.Id + `","type":"general","title":"phish","body":"click here"}`)
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPIInvitationListBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot list invitations via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/invitations/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		creator := makeUserTB(tb, app, "Creator", "")
		makeInvitationTB(tb, app, creator.Id, time.Time{})
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestAPIInvitationViewBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot view an invitation via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		creator := makeUserTB(tb, app, "Creator", "")
		inv := makeInvitationTB(tb, app, creator.Id, time.Time{})
		s.URL = "/api/collections/invitations/records/" + inv.Id
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestAPIMatchMessageListBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant player cannot list match_messages via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/match_messages/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		msgCol, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(msgCol)
		msg.Set("match", match.Id)
		msg.Set("type", "chat")
		msg.Set("content", "secret chat")
		author, _ := app.FindRecordById("users", p1.GetString("player1"))
		msg.Set("author", author.Id)
		require.NoError(tb, app.Save(msg))

		outsider := makeUserTB(tb, app, "Outsider", "")
		s.Headers = authHeaders(tb, outsider)
	}
	s.Test(t)
}

func TestAPIMatchMessageViewBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant player cannot view a match_message via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		msgCol, err := app.FindCollectionByNameOrId("match_messages")
		require.NoError(tb, err)
		msg := core.NewRecord(msgCol)
		msg.Set("match", match.Id)
		msg.Set("type", "chat")
		msg.Set("content", "secret chat")
		author, _ := app.FindRecordById("users", p1.GetString("player1"))
		msg.Set("author", author.Id)
		require.NoError(tb, app.Save(msg))
		s.URL = "/api/collections/match_messages/records/" + msg.Id

		outsider := makeUserTB(tb, app, "Outsider", "")
		s.Headers = authHeaders(tb, outsider)
	}
	s.Test(t)
}

// Positive control: the lockdown must lock only WRITES. An authenticated
// participant can still READ a match through the record API (ViewRule is
// unchanged), proving the migration did not over-reach into reads.
func TestAPIMatchViewStillWorksForParticipant(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "authed player can still GET a match via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{`"collectionName":"matches"`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/api/collections/matches/records/" + match.Id
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}
