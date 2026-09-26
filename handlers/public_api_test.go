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

func TestHomeWithAuth(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET / with auth returns home with player name",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Home Player"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Home Player", "")
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHome_DraftCalendarSuppressesMatchDerivedData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "GET / shows the competition card but no upcoming matches while the calendar is draft",
		Method:             http.MethodGet,
		URL:                "/",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Draft Home League", "0 partidos pendientes"},
		NotExpectedContent: []string{"Próximos partidos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftHomeA")
		p2 := handlers.MakePairTB(tb, app, "DraftHomeB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Draft Home League")
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} shows pair names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"CompA", "CompB", "Clasificación"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CompA")
		p2 := handlers.MakePairTB(tb, app, "CompB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-3")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPage_DraftCalendarHidesRoundsAndStandings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "GET /competition/{id} shows no rounds/standings while the calendar is draft",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"aún no está publicado"},
		NotExpectedContent: []string{"J1", "6-3"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftCompA")
		p2 := handlers.MakePairTB(tb, app, "DraftCompB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		m.Set("scores", "6-3 6-3")
		m.Set("winner", p1.Id)
		require.NoError(tb, app.Save(m))
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionDefaultShowsAllPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} with no pair filter shows every pair's matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"MineA", "MineB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "MineA")
		p2 := handlers.MakePairTB(tb, app, "MineB")
		p3 := handlers.MakePairTB(tb, app, "OtherC")
		p4 := handlers.MakePairTB(tb, app, "OtherD")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		matchLinks := strings.Count(body, "href=\"/match/")
		assert.Equal(tb, 2, matchLinks, "both matches shown by default")
		assert.Contains(tb, body, "Todas las parejas")
	}
	s.Test(t)
}

func TestCompetitionPairFilterShowsOnlyThatPair(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id}?pair=<id> shows only that pair's matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	var wantMatchID, otherMatchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "FilterA")
		p2 := handlers.MakePairTB(tb, app, "FilterB")
		p3 := handlers.MakePairTB(tb, app, "FilterC")
		p4 := handlers.MakePairTB(tb, app, "FilterD")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2, p3, p4})
		wantMatchID = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending").Id
		otherMatchID = handlers.MakeMatchTB(tb, app, comp.Id, p3.Id, p4.Id, "pending").Id
		s.URL = "/competition/" + comp.Id + "?pair=" + p1.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		matchLinks := strings.Count(body, "href=\"/match/")
		assert.Equal(tb, 1, matchLinks, "only the filtered pair's match")
		assert.Contains(tb, body, "/match/"+wantMatchID)
		assert.NotContains(tb, body, "/match/"+otherMatchID)
	}
	s.Test(t)
}

func TestCompetitionPairFilterInvalidIDIgnored(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id}?pair=<invalid> falls back to showing all pairs",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "InvA")
		p2 := handlers.MakePairTB(tb, app, "InvB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/competition/" + comp.Id + "?pair=nonexistent-pair-id"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		matchLinks := strings.Count(body, "href=\"/match/")
		assert.Equal(tb, 1, matchLinks, "invalid pair id ignored, falls back to showing all (only one match exists)")
		assert.Contains(tb, body, "Todas las parejas")
	}
	s.Test(t)
}

func TestCompetitionPairOptionsOrder(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} lists own pair(s) first, others alphabetically",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Todas las parejas", "Otras parejas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		own := handlers.MakePairTB(tb, app, "Zeta Own Pair")
		zebra := handlers.MakePairTB(tb, app, "Zebra Pair")
		alpha := handlers.MakePairTB(tb, app, "Ábaco Pair")
		mid := handlers.MakePairTB(tb, app, "Medio Pair")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{own, zebra, alpha, mid})
		handlers.MakeMatchTB(tb, app, comp.Id, own.Id, zebra.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", own.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		optAll := strings.Index(body, "Todas las parejas")
		optOwn := strings.Index(body, "★ Zeta Own Pair")
		optAbaco := strings.Index(body, ">Ábaco Pair<")
		optMedio := strings.Index(body, ">Medio Pair<")
		optZebra := strings.Index(body, ">Zebra Pair<")
		require.Greater(tb, optOwn, -1, "own pair option with star must be present")
		require.Greater(tb, optAbaco, -1)
		require.Greater(tb, optMedio, -1)
		require.Greater(tb, optZebra, -1)
		assert.True(tb, optAll < optOwn, "Todas las parejas comes before the own pair")
		assert.True(tb, optOwn < optAbaco, "own pair comes before others")
		assert.True(tb, optAbaco < optMedio, "others sorted alphabetically (accent-folded)")
		assert.True(tb, optMedio < optZebra, "others sorted alphabetically")
	}
	s.Test(t)
}

func TestPlayerProfilePage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /player/{id} shows display name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Profile Viewer"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Profile Viewer", "")
		s.URL = "/player/" + user.Id
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestICalMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /ical/match/{id} with date returns ics",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "IcalA")
		p2 := handlers.MakePairTB(tb, app, "IcalB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("time", "18:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestICalCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /ical/competition/{id} with auth returns ics",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "IcalCompA")
		p2 := handlers.MakePairTB(tb, app, "IcalCompB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("time", "18:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestProfileCompletePage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /profile/complete shows name form",
		Method:          http.MethodGet,
		URL:             "/profile/complete",
		ExpectedStatus:  200,
		ExpectedContent: []string{"display_name"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Profile User", "")
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestProfileCompleteSubmit(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /profile/complete sets display name",
		Method:         http.MethodPost,
		URL:            "/profile/complete",
		Body:           strings.NewReader("display_name=NuevoNombre&gender=female"),
		ExpectedStatus: 302,
	}
	var userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Old Name", "")
		userID = user.Id
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		u, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Equal(tb, "NuevoNombre", u.GetString("display_name"))
		assert.Equal(tb, "/", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestCompetitionNotFound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} with bad ID returns error",
		Method:          http.MethodGet,
		ExpectedStatus:  404,
		ExpectedContent: []string{"no encontrada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "NotFound Viewer", "")
		s.URL = "/competition/nonexistent_id"
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPlayoffNoStandings(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} playoff has no standings",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"PlayA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PlayA")
		p2 := handlers.MakePairTB(tb, app, "PlayB")
		comp := handlers.MakeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionArchivedShowsAwards(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} archived shows awards section",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Archivada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ArcA")
		p2 := handlers.MakePairTB(tb, app, "ArcB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}
