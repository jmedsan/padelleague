package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

func TestNewCompetitionView(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	p1 := handlers.MakePairTB(t, app, "CV A")
	p2 := handlers.MakePairTB(t, app, "CV B")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	cv := handlers.NewCompetitionView(app, comp, handlers.AdminSummary)
	assert.Equal(t, handlers.AdminSummary, cv.Mode)
	assert.Equal(t, comp.GetString("name"), cv.Name)
	assert.Equal(t, 2, cv.PairsCount)
	assert.Equal(t, 1, cv.TotalMatches)
	assert.Equal(t, 0, cv.PlayedMatches)
	assert.Equal(t, 1, cv.PendingCount)
	assert.Equal(t, "/admin/competitions/"+comp.Id, cv.URL)

	cvPlayer := handlers.NewCompetitionView(app, comp, handlers.PlayerRow)
	assert.Equal(t, "/competition/"+comp.Id, cvPlayer.URL)
}

func TestNewHomeCompetitionView(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	svc := league.New(app, nil)
	comp := handlers.MakeCompetitionTB(t, app, "league", nil)

	cv := handlers.NewHomeCompetitionView(svc, comp, 3, nil)
	assert.Equal(t, handlers.PlayerRow, cv.Mode)
	assert.Equal(t, 3, cv.PendingCount)
	assert.Equal(t, "/competition/"+comp.Id, cv.URL)
	assert.Nil(t, cv.Standing, "no standings computed yet (no pairs/matches)")
}

func TestNewHomeCompetitionView_Standing(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	svc := league.New(app, nil)
	p1 := handlers.MakePairTB(t, app, "CVStandA")
	p2 := handlers.MakePairTB(t, app, "CVStandB")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	m := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "final")
	m.Set("scores", "6-3 6-4")
	m.Set("winner", p1.Id)
	require.NoError(t, app.Save(m))

	playerPairIDs := map[string]struct{}{p1.Id: {}}
	cv := handlers.NewHomeCompetitionView(svc, comp, 0, playerPairIDs)
	require.NotNil(t, cv.Standing, "player's pair is in the computed standings")
	assert.Equal(t, 1, cv.Standing.Position, "winner tops a 2-pair table")
	assert.Equal(t, 3, cv.Standing.Points, "3 points for a win")
}

func TestNewHomeCompetitionView_PlayoffHasNoStanding(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	svc := league.New(app, nil)
	p1 := handlers.MakePairTB(t, app, "CVPlayoffA")
	comp := handlers.MakeCompetitionTB(t, app, "playoff", []*core.Record{p1})

	playerPairIDs := map[string]struct{}{p1.Id: {}}
	cv := handlers.NewHomeCompetitionView(svc, comp, 0, playerPairIDs)
	assert.Nil(t, cv.Standing, "playoffs don't compute league standings")
}

func TestCompetitionCardPlayerRowHasNoPairStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
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
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CV Render A")
		p2 := handlers.MakePairTB(tb, app, "CV Render B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "CV Render League")
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = handlers.AuthHeaders(tb, player)
	}
	s.Test(t)
}

func TestCompetitionCardAdminSummaryHasStats(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
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
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "CV Admin A")
		p2 := handlers.MakePairTB(tb, app, "CV Admin B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "CV Admin League")
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestNewCompetitionView_CountsUnconfirmedResultAsPlayed(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	p1 := handlers.MakePairTB(t, app, "CVP A")
	p2 := handlers.MakePairTB(t, app, "CVP B")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	proposed := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")
	createResultProposal(t, app, proposed.Id, p1.GetString("player1"), "6-3 6-4")
	handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")

	cv := handlers.NewCompetitionView(app, comp, handlers.AdminSummary)
	assert.Equal(t, 2, cv.TotalMatches)
	assert.Equal(t, 1, cv.PlayedMatches, "the proposed match counts as played")
	assert.True(t, cv.HasProvisional)
	assert.Equal(t, 1, cv.PendingCount, "an unconfirmed match counts as played, not pending")
}

func TestNewCompetitionView_ConfirmedOnlyIsNotFlagged(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	p1 := handlers.MakePairTB(t, app, "CVQ A")
	p2 := handlers.MakePairTB(t, app, "CVQ B")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	cv := handlers.NewCompetitionView(app, comp, handlers.AdminSummary)
	assert.False(t, cv.HasProvisional)
}

func TestNewHomeCompetitionView_StandingFlagsUnconfirmedResult(t *testing.T) {
	t.Parallel()
	app := handlers.NewTestApp(t)
	svc := league.New(app, nil)
	p1 := handlers.MakePairTB(t, app, "CVS A")
	p2 := handlers.MakePairTB(t, app, "CVS B")
	comp := handlers.MakeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	m := handlers.MakeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")
	createResultProposal(t, app, m.Id, p1.GetString("player1"), "6-1 6-2")

	cv := handlers.NewHomeCompetitionView(svc, comp, 1, map[string]struct{}{p1.Id: {}})
	require.NotNil(t, cv.Standing)
	assert.Equal(t, 3, cv.Standing.Points)
	assert.True(t, cv.Standing.HasProvisional)
}
