package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Cluster 1: Home shows only player's competitions, sets nextMatch ---

func TestHomeGen2_OnlyPlayerCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "home shows only competitions the player is in",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"MyComp"},
	}
	var otherCompName string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "MyPair")
		otherPair := makePairTB(tb, app, "OtherPair")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, otherPair})
		comp.Set("name", "MyComp")
		require.NoError(tb, app.Save(comp))

		alienP1 := makePairTB(tb, app, "AlienA")
		alienP2 := makePairTB(tb, app, "AlienB")
		alienComp := makeCompetitionTB(tb, app, "league", []*core.Record{alienP1, alienP2})
		alienComp.Set("name", "AlienComp")
		require.NoError(tb, app.Save(alienComp))
		otherCompName = "AlienComp"

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, otherCompName, "alien competition must not appear")
	}
	s.Test(t)
}

func TestHomeGen2_NextMatchFromFirstPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "home sets nextMatch from first pending match",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NMOpp", "Sin programar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "NMPair")
		oppPair := makePairTB(tb, app, "NMOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// --- Cluster 2: Pending match counting and detail cap ---

func TestHomeGen2_PendingCountAndDetailCap(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "home counts pending correctly and caps details at 5",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"7 partidos pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "CapPair")
		opponents := make([]*core.Record, 7)
		for i := range opponents {
			opponents[i] = makePairTB(tb, app, fmt.Sprintf("CapOpp%d", i))
		}
		allPairs := append([]*core.Record{myPair}, opponents...)
		comp := makeCompetitionTB(tb, app, "league", allPairs)
		for _, opp := range opponents {
			makeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")
		}
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "7 partidos pendientes")
		assert.NotContains(tb, body, "CapOpp5", "6th pending match should not appear in details (cap=5)")
		assert.NotContains(tb, body, "CapOpp6", "7th pending match should not appear in details (cap=5)")
	}
	s.Test(t)
}

// --- Cluster 3: Proposal schedule status ---

func TestHomeGen2_AcceptedProposalShowsConfirmado(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "accepted proposal shows Confirmado with date",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Confirmado", "2026-09-15 18:00", "Padel 360"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "SchAccPair")
		oppPair := makePairTB(tb, app, "SchAccOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, match.Id, user.Id, "accepted",
			`{"date":"2026-09-15","time":"18:00","venue_name":"Padel 360"}`)

		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_PendingProposalShowsPropuestaEnviada(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "pending proposal shows Propuesta enviada with date",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Propuesta enviada", "2026-10-01 20:00"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "SchPenPair")
		oppPair := makePairTB(tb, app, "SchPenOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, match.Id, user.Id, "pending",
			`{"date":"2026-10-01","time":"20:00","venue_text":"Mi club"}`)

		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// --- Cluster 4: Proposer team check (respond_proposal action) ---

func TestHomeGen2_OpponentProposalCreatesAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "opponent proposal creates respond action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Propuesta de horario pendiente", "Acciones pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "RActMy")
		oppPair := makePairTB(tb, app, "RActOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		oppUser := oppPair.GetString("player1")
		createProposal(tb, app, match.Id, oppUser, "pending",
			`{"date":"2026-09-20","time":"19:00"}`)

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_OwnProposalNoAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "own proposal does not create respond action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "OwnPropMy")
		oppPair := makePairTB(tb, app, "OwnPropOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		myUser := myPair.GetString("player1")
		createProposal(tb, app, match.Id, myUser, "pending",
			`{"date":"2026-09-20","time":"19:00"}`)

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Propuesta de horario pendiente",
			"own proposal should not create a respond action")
	}
	s.Test(t)
}

// --- Cluster 5: Score confirm team check ---

func TestHomeGen2_OpponentScoreShowsConfirmAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "opponent score shows confirm action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Confirmar resultado: 6-3 6-4", "Acciones pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "ConfMy")
		oppPair := makePairTB(tb, app, "ConfOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})

		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")
		fresh, err := app.FindRecordById("matches", match.Id)
		require.NoError(tb, err)
		fresh.Set("status", "confirmed")
		fresh.Set("scores", "6-3 6-4")
		fresh.Set("submitted_by", oppPair.GetString("player1"))
		require.NoError(tb, app.Save(fresh))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_OwnScoreNoConfirmAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "own score shows no confirm action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "OwnConfMy")
		oppPair := makePairTB(tb, app, "OwnConfOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})

		match := makeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")
		fresh, err := app.FindRecordById("matches", match.Id)
		require.NoError(tb, err)
		fresh.Set("status", "confirmed")
		fresh.Set("scores", "6-3 6-4")
		fresh.Set("submitted_by", myPair.GetString("player1"))
		require.NoError(tb, app.Save(fresh))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Confirmar resultado",
			"own score should not show confirm action")
	}
	s.Test(t)
}

// --- Cluster 6: Penalty flag in competition page ---

func TestCompetitionGen2_WithPenaltyShowsPenColumn(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "competition with penalty shows Pen column",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Pen."},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PenA")
		p2 := makePairTB(tb, app, "PenB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("penalty_points", fmt.Sprintf(`{"%s": 1}`, p1.Id))
		require.NoError(tb, app.Save(comp))

		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionGen2_NoPenaltyHidesPenColumn(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "competition without penalty hides Pen column",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"NoPenA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NoPenA")
		p2 := makePairTB(tb, app, "NoPenB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Pen.", "no penalty column when no penalties")
	}
	s.Test(t)
}

// --- Cluster 7: firstIncompleteRound sets AutoExpandRound ---

func TestCompetitionGen2_AutoExpandRound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "auto-expand targets first incomplete round",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornada 2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ExpA")
		p2 := makePairTB(tb, app, "ExpB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		m2 := makeMatchTB(tb, app, comp.Id, p2.Id, p1.Id, "pending")
		m2.Set("round_number", 2)
		require.NoError(tb, app.Save(m2))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "checked", "incomplete round should have checked attribute")
	}
	s.Test(t)
}

// --- Cluster 8: buildCompPairs from playoff (no standings) ---

func TestCompetitionGen2_CompPairsFromPlayoff(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "playoff competition populates CompPairs from match names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Comparar parejas", "POffA", "POffB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "POffA")
		p2 := makePairTB(tb, app, "POffB")
		comp := makeCompetitionTB(tb, app, "playoff", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

// --- Helpers ---

func createProposal(tb testing.TB, app core.App, matchID, authorID, status, proposalData string) {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("type", "scheduling_proposal")
	msg.Set("author", authorID)
	msg.Set("proposal_status", status)
	msg.Set("proposal_data", proposalData)
	require.NoError(tb, app.Save(msg))
}

func makeFinalMatchTB(tb testing.TB, app core.App, compID, p1ID, p2ID, score, winnerID string) {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(tb, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", "final")
	record.Set("scores", score)
	record.Set("winner", winnerID)
	record.Set("round_number", 1)
	require.NoError(tb, app.Save(record))
}

func TestHomeWithCompetitionData(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET / with active competition shows home data",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Home A", "Home B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Home A")
		p2 := makePairTB(tb, app, "Home B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Pending match — triggers buildNextMatch, buildHomeCompetition
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Confirmed match — triggers findUnconfirmedScores
		confirmed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		confirmed.Set("scores", "6-3 6-4")
		confirmed.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(confirmed))

		// Final match — triggers findRecentResults
		col, _ := app.FindCollectionByNameOrId("matches")
		final := core.NewRecord(col)
		final.Set("competition", comp.Id)
		final.Set("pair1", p1.Id)
		final.Set("pair2", p2.Id)
		final.Set("status", "final")
		final.Set("scores", "6-2 6-1")
		final.Set("winner", p1.Id)
		final.Set("round_number", 1)
		require.NoError(tb, app.Save(final))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPageWithMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /competition/{id} with matches shows pair names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Comp A", "Comp B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Comp A")
		p2 := makePairTB(tb, app, "Comp B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeWithScheduledMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET / with pending proposal and scheduled match",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Sched A"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Sched A")
		p2 := makePairTB(tb, app, "Sched B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Match with scheduled date and a pending proposal
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-20 19:00:00.000Z")
		match.Set("time", "19:00")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))

		// Add a pending proposal from opponent
		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p2.GetString("player1"))
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", map[string]any{
			"date": "2026-09-25", "time": "20:00", "venue_name": "Wurko",
		})
		msg.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(msg))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}
