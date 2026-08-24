package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerPreCreate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/players/pre-create creates player with password link",
		Method:          http.MethodPost,
		URL:             "/admin/players/pre-create",
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-success", "reset-password"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
		s.Body = strings.NewReader("email=newplayer@test.local&display_name=New+Player")
	}
	s.Test(t)
}

func TestPlayerUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/players/{id} updates player",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var playerID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		player := makeUserTB(tb, app, "Old Name", "")
		playerID = player.Id
		s.URL = "/admin/players/" + player.Id
		s.Body = strings.NewReader("display_name=New+Name&role=player")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		p, err := app.FindRecordById("users", playerID)
		require.NoError(tb, err)
		assert.Equal(tb, "New Name", p.GetString("display_name"))
	}
	s.Test(t)
}

func TestPairsCreate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/pairs creates pair",
		Method:         http.MethodPost,
		URL:            "/admin/pairs",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := makeUserTB(tb, app, "Pair P1", "")
		p2 := makeUserTB(tb, app, "Pair P2", "")
		s.Body = strings.NewReader(fmt.Sprintf("name=Test+Pair&player1=%s&player2=%s", p1.Id, p2.Id))
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestPairsUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/pairs/{id} updates pair",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		pair := makePairTB(tb, app, "Old Pair")
		s.URL = "/admin/pairs/" + pair.Id
		s.Body = strings.NewReader("name=Updated+Pair")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestVenuesCreate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/venues creates venue",
		Method:         http.MethodPost,
		URL:            "/admin/venues",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("name=Club+Padel&address=Calle+Test+1&courts=4")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestVenuesUpdate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/venues/{id} updates venue",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		venue := makeVenueTB(tb, app, "Old Venue")
		s.URL = "/admin/venues/" + venue.Id
		s.Body = strings.NewReader("name=New+Venue&address=New+Address&courts=6")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestVenuesDelete(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/venues/{id}/delete removes venue",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var venueID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		venue := makeVenueTB(tb, app, "Delete Me")
		venueID = venue.Id
		s.URL = "/admin/venues/" + venue.Id + "/delete"
		hdrs := authHeaders(tb, admin)
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("venues", venueID)
		assert.Error(tb, err)
	}
	s.Test(t)
}

func TestInvitationsRevoke(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations/{id}/revoke removes invitation",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var inviteID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		inviteID = invite.Id
		s.URL = "/admin/invitations/" + invite.Id + "/revoke"
		hdrs := authHeaders(tb, admin)
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("invitations", inviteID)
		assert.Error(tb, err)
	}
	s.Test(t)
}

func TestPlayerProfileCompetitionStatsSortedDeterministic(t *testing.T) {
	t.Parallel()
	names := []string{"Zebra Cup", "Alpha League", "Middle Tournament"}
	sorted := []string{"Alpha League", "Middle Tournament", "Zebra Cup"}

	for iter := 0; iter < 20; iter++ {
		s := &tests.ApiScenario{
			TestAppFactory:  testAppFactory,
			Name:            fmt.Sprintf("GET /player/{id} competition stats sorted (iter %d)", iter),
			Method:          http.MethodGet,
			ExpectedStatus:  200,
			ExpectedContent: sorted,
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupPublicRoutes(tb, app, e)
			user := makeUserTB(tb, app, "Sort Player", "")
			partner := makeUserTB(tb, app, "Sort Partner", "")

			pairCol, _ := app.FindCollectionByNameOrId("pairs")
			matchCol, _ := app.FindCollectionByNameOrId("matches")

			compCol, _ := app.FindCollectionByNameOrId("competitions")
			for i, name := range names {
				pair := core.NewRecord(pairCol)
				pair.Set("name", fmt.Sprintf("SortPair%d", i))
				pair.Set("player1", user.Id)
				pair.Set("player2", partner.Id)
				require.NoError(tb, app.Save(pair))

				opponent := makePairTB(tb, app, fmt.Sprintf("SortOpp%d", i))

				comp := core.NewRecord(compCol)
				comp.Set("name", name)
				comp.Set("type", "league")
				comp.Set("active", true)
				comp.Set("pairs", []string{pair.Id, opponent.Id})
				require.NoError(tb, app.Save(comp))

				m := core.NewRecord(matchCol)
				m.Set("competition", comp.Id)
				m.Set("pair1", pair.Id)
				m.Set("pair2", opponent.Id)
				m.Set("status", "final")
				m.Set("scores", "6-3 6-4")
				m.Set("winner", pair.Id)
				m.Set("round_number", 1)
				require.NoError(tb, app.Save(m))
			}
			s.URL = "/player/" + user.Id
			s.Headers = authHeaders(tb, user)
		}
		s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
			body, err := io.ReadAll(res.Body)
			require.NoError(tb, err)
			html := string(body)
			statsStart := strings.Index(html, "Por competición")
			require.NotEqual(tb, -1, statsStart, "expected competition stats section")
			statsSection := html[statsStart:]
			prev := -1
			for _, name := range sorted {
				pos := strings.Index(statsSection, name)
				require.NotEqual(tb, -1, pos, "expected %q in stats section", name)
				assert.Greater(tb, pos, prev, "competition %q should appear after previous", name)
				prev = pos
			}
		}
		s.Test(t)
	}
}
