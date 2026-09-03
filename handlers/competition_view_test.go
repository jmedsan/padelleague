package handlers

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

func TestNewCompetitionView(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePairTB(t, app, "CV A")
	p2 := makePairTB(t, app, "CV B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	cv := NewCompetitionView(app, comp, AdminSummary)
	assert.Equal(t, AdminSummary, cv.Mode)
	assert.Equal(t, comp.GetString("name"), cv.Name)
	assert.Equal(t, 2, cv.PairsCount)
	assert.Equal(t, 1, cv.TotalMatches)
	assert.Equal(t, 0, cv.PlayedMatches)
	assert.Equal(t, 1, cv.PendingCount)
	assert.Equal(t, "/admin/competitions/"+comp.Id, cv.URL)

	cvPlayer := NewCompetitionView(app, comp, PlayerRow)
	assert.Equal(t, "/competition/"+comp.Id, cvPlayer.URL)
}

func TestNewHomeCompetitionView(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := league.New(app, nil)
	comp := makeCompetitionTB(t, app, "league", nil)

	cv := NewHomeCompetitionView(svc, comp, 3, nil)
	assert.Equal(t, PlayerRow, cv.Mode)
	assert.Equal(t, 3, cv.PendingCount)
	assert.Equal(t, "/competition/"+comp.Id, cv.URL)
	assert.Nil(t, cv.Standing, "no standings computed yet (no pairs/matches)")
}

func TestNewHomeCompetitionView_Standing(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := league.New(app, nil)
	p1 := makePairTB(t, app, "CVStandA")
	p2 := makePairTB(t, app, "CVStandB")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	m := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	playerPairIDs := map[string]struct{}{p1.Id: {}}
	cv := NewHomeCompetitionView(svc, comp, 0, playerPairIDs)
	require.NotNil(t, cv.Standing, "player's pair is in the computed standings")
	assert.Equal(t, 1, cv.Standing.Position, "winner tops a 2-pair table")
	assert.Equal(t, 3, cv.Standing.Points, "3 points for a win")
}

func TestNewHomeCompetitionView_PlayoffHasNoStanding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	svc := league.New(app, nil)
	p1 := makePairTB(t, app, "CVPlayoffA")
	comp := makeCompetitionTB(t, app, "playoff", []*core.Record{p1})

	playerPairIDs := map[string]struct{}{p1.Id: {}}
	cv := NewHomeCompetitionView(svc, comp, 0, playerPairIDs)
	assert.Nil(t, cv.Standing, "playoffs don't compute league standings")
}

func TestCompetitionCardPlayerRowHasNoPairStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "player home competition card shows name, no admin stats",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"CV Render League"},
		NotExpectedContent: []string{
			"parejas",
			"progress progress-success",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "CV Render A")
		p2 := makePairTB(tb, app, "CV Render B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "CV Render League")
		require.NoError(tb, app.Save(comp))
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestCompetitionCardAdminSummaryHasStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin dashboard competition card shows stats",
		Method:         http.MethodGet,
		URL:            "/admin/competitions",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			"CV Admin League",
			"parejas",
			"partidos",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "CV Admin A")
		p2 := makePairTB(tb, app, "CV Admin B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "CV Admin League")
		require.NoError(tb, app.Save(comp))
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.Headers = authHeaders(tb, makeAdminUserTB(tb, app))
	}
	s.Test(t)
}
