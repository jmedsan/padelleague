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
		Body:            strings.NewReader(`{"roles":["admin"]}`),
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
		assert.Contains(tb, u.GetStringSlice("roles"), "player", "roles must still contain player")
		assert.NotContains(tb, u.GetStringSlice("roles"), "admin", "roles must not contain admin")
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
		Body:            strings.NewReader(`{"email":"attacker@evil.com","password":"hunter2hunter2","passwordConfirm":"hunter2hunter2","display_name":"Attacker","roles":["admin"]}`),
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

// All writes to pairs/competitions/venues/invitations go through server-side
// app.Save in admin handlers; the record API is locked to superuser-only.
func TestAPIAdminCannotCreateCompetitionViaRecordAPI(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin cannot create a competition via the record API (writes are server-side only)",
		Method:          http.MethodPost,
		URL:             "/api/collections/competitions/records",
		Body:            strings.NewReader(`{"name":"Test","type":"league"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		admin := makeAdminUserTB(tb, app)
		s.Headers = jsonHeaders(authHeaders(tb, admin))
	}
	s.Test(t)
}

// R-179 F1: the users.roles field is hidden from the record API, so an
// authenticated player cannot enumerate another user's admin/player roles.
func TestAPIUserRolesHiddenFromPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "player cannot read another user's roles via the record API (roles hidden)",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		NotExpectedContent: []string{`"roles"`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		target, err := app.FindRecordById("users", p2.GetString("player1"))
		require.NoError(tb, err)
		s.URL = "/api/collections/users/records/" + target.Id
		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

// All writes to pairs go through server-side app.Save in admin handlers; the
// record API is locked to superuser-only.
func TestAPIPairCreateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot create a pair via the record API",
		Method:          http.MethodPost,
		URL:             "/api/collections/pairs/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		player := makeUserTB(tb, app, "Player", "")
		s.Body = strings.NewReader(`{"name":"Forged","player1":"` + player.Id + `","player2":"` + player.Id + `"}`)
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPIPairUpdateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	var pairID string
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot update a pair via the record API",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"name":"Renamed"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		pairID = p1.Id
		s.URL = "/api/collections/pairs/records/" + pairID
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPIPairDeleteForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot delete a pair via the record API",
		Method:          http.MethodDelete,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		s.URL = "/api/collections/pairs/records/" + p1.Id
		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

// All writes to venues go through server-side app.Save in admin handlers; the
// record API is locked to superuser-only.
func TestAPIVenueCreateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot create a venue via the record API",
		Method:          http.MethodPost,
		URL:             "/api/collections/venues/records",
		Body:            strings.NewReader(`{"name":"Forged Venue"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPIVenueUpdateForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot update a venue via the record API",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"name":"Renamed"}`),
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		venue := makeVenueTB(tb, app, "Padel 360")
		s.URL = "/api/collections/venues/records/" + venue.Id
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = jsonHeaders(authHeaders(tb, player))
	}
	s.Test(t)
}

func TestAPIVenueDeleteForbiddenForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot delete a venue via the record API",
		Method:          http.MethodDelete,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		venue := makeVenueTB(tb, app, "Padel 360")
		s.URL = "/api/collections/venues/records/" + venue.Id
		player := makeUserTB(tb, app, "Player", "")
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

// push_subscriptions holds per-device webpush endpoint + auth keys — the most
// sensitive per-user data of any collection. ListRule/ViewRule scope to
// `user = @request.auth.id`; this pins that another player cannot enumerate
// or view a victim's subscription.
func TestAPIPushSubscriptionListExcludesOtherUsers(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot list another user's push subscriptions via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/push_subscriptions/records",
		ExpectedStatus:  200,
		ExpectedContent: []string{`"totalItems":0`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		col, err := app.FindCollectionByNameOrId("push_subscriptions")
		require.NoError(tb, err)
		sub := core.NewRecord(col)
		sub.Set("user", victim.Id)
		sub.Set("endpoint", "https://push.example/victim-endpoint")
		sub.Set("p256dh", "victim-p256dh")
		sub.Set("auth", "victim-auth")
		require.NoError(tb, app.Save(sub))

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

func TestAPIPushSubscriptionViewBlockedForOtherUser(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot view another user's push subscription via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"\"status\":404"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		col, err := app.FindCollectionByNameOrId("push_subscriptions")
		require.NoError(tb, err)
		sub := core.NewRecord(col)
		sub.Set("user", victim.Id)
		sub.Set("endpoint", "https://push.example/victim-endpoint")
		sub.Set("p256dh", "victim-p256dh")
		sub.Set("auth", "victim-auth")
		require.NoError(tb, app.Save(sub))
		s.URL = "/api/collections/push_subscriptions/records/" + sub.Id

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

// documents (the reference-document library) are readable by any
// authenticated user; ListRule ("@request.auth.id != ”") filters an
// unauthenticated request's results to zero rather than 403ing the whole
// request (PocketBase's ListRule is a row filter, not an access gate).
func TestAPIDocumentListEmptyForAnon(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "unauthenticated client sees zero documents via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/documents/records",
		ExpectedStatus:  200,
		ExpectedContent: []string{`"totalItems":0`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		makeDocumentTB(tb, app, "Reglamento", true, "https://example.com/rules.pdf")
	}
	s.Test(t)
}

// document_acks previously (R-review history) allowed any authenticated user
// to list every user's acknowledgement records; it is now scoped to
// `user = @request.auth.id`. This pins the fix against regression.
func TestAPIDocumentAckListExcludesOtherUsers(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot list another user's document_acks via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/document_acks/records",
		ExpectedStatus:  200,
		ExpectedContent: []string{`"totalItems":0`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		comp := makeCompetitionTB(tb, app, "league", nil)
		col, err := app.FindCollectionByNameOrId("document_acks")
		require.NoError(tb, err)
		ack := core.NewRecord(col)
		ack.Set("user", victim.Id)
		ack.Set("competition", comp.Id)
		require.NoError(tb, app.Save(ack))

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

func TestAPIDocumentAckViewBlockedForOtherUser(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot view another user's document_acks via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"\"status\":404"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		comp := makeCompetitionTB(tb, app, "league", nil)
		col, err := app.FindCollectionByNameOrId("document_acks")
		require.NoError(tb, err)
		ack := core.NewRecord(col)
		ack.Set("user", victim.Id)
		ack.Set("competition", comp.Id)
		require.NoError(tb, app.Save(ack))
		s.URL = "/api/collections/document_acks/records/" + ack.Id

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

// search_history — a player's search queries could reveal who/what they were
// investigating (opponents, disputes); ListRule scopes to
// `user = @request.auth.id`.
func TestAPISearchHistoryListExcludesOtherUsers(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot list another user's search history via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/search_history/records",
		ExpectedStatus:  200,
		ExpectedContent: []string{`"totalItems":0`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		col, err := app.FindCollectionByNameOrId("search_history")
		require.NoError(tb, err)
		entry := core.NewRecord(col)
		entry.Set("user", victim.Id)
		entry.Set("query", "secret opponent scouting")
		require.NoError(tb, app.Save(entry))

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

func TestAPISearchHistoryViewBlockedForOtherUser(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot view another user's search history via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"\"status\":404"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		victim := makeUserTB(tb, app, "Victim", "")
		col, err := app.FindCollectionByNameOrId("search_history")
		require.NoError(tb, err)
		entry := core.NewRecord(col)
		entry.Set("user", victim.Id)
		entry.Set("query", "secret opponent scouting")
		require.NoError(tb, app.Save(entry))
		s.URL = "/api/collections/search_history/records/" + entry.Id

		attacker := makeUserTB(tb, app, "Attacker", "")
		s.Headers = authHeaders(tb, attacker)
	}
	s.Test(t)
}

// penalties has no explicit ListRule/ViewRule set anywhere in its migration,
// so PocketBase defaults both to nil (superuser-only, fail-closed).
func TestAPIPenaltyListBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot list penalties via the record API",
		Method:          http.MethodGet,
		URL:             "/api/collections/penalties/records",
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makePenaltyTB(tb, app, comp.Id, p1.Id, 3, "Walkover", "", false)

		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestAPIPenaltyViewBlockedForPlayer(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player cannot view a penalty via the record API",
		Method:          http.MethodGet,
		ExpectedStatus:  403,
		ExpectedContent: []string{"superusers"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		p1 := makePairTB(tb, app, "A")
		p2 := makePairTB(tb, app, "B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makePenaltyTB(tb, app, comp.Id, p1.Id, 3, "Walkover", "", false)

		penalties, err := app.FindRecordsByFilter("penalties", "competition = {:comp}", "", 1, 0,
			map[string]any{"comp": comp.Id})
		require.NoError(tb, err)
		require.Len(tb, penalties, 1)
		s.URL = "/api/collections/penalties/records/" + penalties[0].Id

		player, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}
