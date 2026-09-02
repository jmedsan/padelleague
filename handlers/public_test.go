package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// Cluster 1: Home shows only player's competitions, sets nextMatch

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
		Name:            "home synthesizes organize action for unscheduled next match",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NMOpp", "Propón una fecha"},
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

// Cluster 2: Pending match counting and detail cap

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

// Cluster 3: Proposal schedule status

func TestHomeGen2_AcceptedProposalShowsConfirmado(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "accepted proposal shows play action with date and venue",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Próximo partido", "15/09/2026 18:00", "Padel 360"},
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
		Name:            "pending proposal shows play action with date",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Próximo partido", "01/10/2026 20:00"},
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

// Cluster 4: Proposer team check (respond_proposal action)

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

// Cluster 5: Score confirm team check

func TestHomeGen2_OpponentScoreShowsConfirmAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "opponent score shows confirm action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Responder resultado: 6-3 6-4", "Acciones pendientes"},
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
		assert.NotContains(tb, body, "Responder resultado",
			"own score should not show respond action")
	}
	s.Test(t)
}

// Cluster 6: Penalty flag in competition page

func TestCompetitionGen2_WithPenaltyShowsPenColumn(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "competition with penalty shows Pen column",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Pen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PenA")
		p2 := makePairTB(tb, app, "PenB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makePenaltyTB(tb, app, comp.Id, p1.Id, 1, "Prueba", "", false)

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
		assert.NotContains(tb, body, ">Pen</th>", "no penalty column header when no penalties")
	}
	s.Test(t)
}

// Cluster 7: firstIncompleteRound sets AutoExpandRound

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

// Helpers

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

func TestHome_RecentResultsNonEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "home page shows recent results for final matches newest-first",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Mis últimos partidos", "6-1 6-2", "6-3 6-4"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Result A")
		p2 := makePairTB(tb, app, "Result B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		col, _ := app.FindCollectionByNameOrId("matches")
		// Match date deliberately inverted from insertion (creation) order,
		// so the assertion below only passes if the query sorts by -date —
		// sorting by -created would put "6-1 6-2" (created first, dated
		// later) in the wrong position instead.
		dates := []string{"2026-01-20", "2026-01-05"}
		for i, score := range []string{"6-1 6-2", "6-3 6-4"} {
			m := core.NewRecord(col)
			m.Set("competition", comp.Id)
			m.Set("pair1", p1.Id)
			m.Set("pair2", p2.Id)
			m.Set("status", "final")
			m.Set("scores", score)
			m.Set("winner", p1.Id)
			m.Set("round_number", i+1)
			m.Set("date", dates[i])
			require.NoError(tb, app.Save(m))
		}

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "6-1 6-2", "first final match score must appear")
		assert.Contains(tb, body, "6-3 6-4", "second final match score must appear")
		assert.Contains(tb, body, "Mis últimos partidos", "recent results heading must appear")
		newerIdx := strings.Index(body, "6-1 6-2")
		olderIdx := strings.Index(body, "6-3 6-4")
		require.NotEqual(tb, -1, olderIdx)
		require.NotEqual(tb, -1, newerIdx)
		assert.Less(tb, newerIdx, olderIdx, "the more recent match (by date) must render first")
	}
	s.Test(t)
}

// Cluster: Player urgent tasks ranking (T3)

func TestHome_UrgentTasksRanking(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "unified actions: dispute > play > organize ordered",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Disputa abierta",
			"DisputeOpp",
			"Acciones pendientes",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "UrgPair")
		disputeOpp := makePairTB(tb, app, "DisputeOpp")
		playOpp := makePairTB(tb, app, "PlayOpp")
		orgOpp := makePairTB(tb, app, "OrgOpp")

		allPairs := []*core.Record{myPair, disputeOpp, playOpp, orgOpp}
		comp := makeCompetitionTB(tb, app, "league", allPairs)
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		// Disputed match (should be hero)
		disputeMatch := makeMatchTB(tb, app, comp.Id, myPair.Id, disputeOpp.Id, "disputed")
		_ = disputeMatch

		// Match with accepted proposal (play task)
		playMatch := makeMatchTB(tb, app, comp.Id, myPair.Id, playOpp.Id, "pending")
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, playMatch.Id, user.Id, "accepted",
			`{"date":"2026-08-20","time":"19:00","venue_name":"Padel 360"}`)

		// Pending match with no proposal (organize task — deadline well past)
		orgMatch := makeMatchTB(tb, app, comp.Id, myPair.Id, orgOpp.Id, "pending")
		orgMatch.Set("round_number", 1)
		require.NoError(tb, app.Save(orgMatch))

		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "bg-error/10", "dispute action must use error accent")
		assert.Contains(tb, body, "PlayOpp", "play task must appear")
		assert.Contains(tb, body, "Próximo partido", "play task description")
		assert.Contains(tb, body, "OrgOpp", "organize task must appear")
		assert.Contains(tb, body, "Organiza antes del", "organize deadline")
	}
	s.Test(t)
}

func TestHome_OrganizeWarningBadges(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "organize tasks show correct warning accent",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Organiza antes del"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "WarnPair")
		opp := makePairTB(tb, app, "WarnOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, opp})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		// Round 1 of 1 — deadline is end_date (2026-07-01), well past
		match := makeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")
		match.Set("round_number", 1)
		require.NoError(tb, app.Save(match))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "bg-error/10", "overdue organize action must use error accent")
		assert.Contains(tb, body, "Organiza antes del", "deadline text must appear")
	}
	s.Test(t)
}

// Cluster: Admin dashboard (T4)

func TestHome_AdminSetupChecklist(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin sees setup checklist for inactive competition",
		Method:         http.MethodGet,
		URL:            "/admin/competitions",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"SetupComp",
			"Parejas añadidas",
			"Jornadas generadas",
			"Fechas configuradas",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "SetupP1")
		p2 := makePairTB(tb, app, "SetupP2")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		comp.Set("name", "SetupComp")
		require.NoError(tb, app.Save(comp))

		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Activar", "not ready — missing fixtures and dates")
	}
	s.Test(t)
}

func TestHome_AdminSetupReady(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin sees Activar when setup complete",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Activar", "ReadyComp"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ReadyP1")
		p2 := makePairTB(tb, app, "ReadyP2")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		comp.Set("name", "ReadyComp")
		comp.Set("start_date", "2026-09-01 00:00:00.000Z")
		comp.Set("end_date", "2026-12-01 00:00:00.000Z")
		require.NoError(tb, app.Save(comp))
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminAlerts(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin sees an urgent (dispute) alert as a compact row; overdue is not urgent so it's absent",
		Method:         http.MethodGet,
		URL:            "/admin/competitions",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"admin-urgent-items",
			"AlertDispP1",
			"Test Competition",
			"Disputa",
		},
		NotExpectedContent: []string{"AlertOvdP1"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)

		dispP1 := makePairTB(tb, app, "AlertDispP1")
		dispP2 := makePairTB(tb, app, "AlertDispP2")
		ovdP1 := makePairTB(tb, app, "AlertOvdP1")
		ovdP2 := makePairTB(tb, app, "AlertOvdP2")

		allPairs := []*core.Record{dispP1, dispP2, ovdP1, ovdP2}
		comp := makeCompetitionTB(tb, app, "league", allPairs)
		comp.Set("start_date", "2026-05-01 00:00:00.000Z")
		comp.Set("end_date", "2026-06-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		disputed := makeMatchTB(tb, app, comp.Id, dispP1.Id, dispP2.Id, "disputed")
		disputed.Set("scores", "6-3 6-4")
		disputed.Set("submitted_by", dispP1.GetString("player1"))
		disputed.Set("disputed_by", dispP2.GetString("player1"))
		disputed.Set("disputed_scores", "6-4 6-3")
		disputed.Set("dispute_notes", "Marcador incorrecto")
		require.NoError(tb, app.Save(disputed))
		makeMatchTB(tb, app, comp.Id, ovdP1.Id, ovdP2.Id, "pending")

		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminWalkoverAlert(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin sees walkover as an urgent compact row on home",
		Method:         http.MethodGet,
		URL:            "/admin/competitions",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Walkover League",
			"Walkover A",
			"Incomparecencia",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Walkover A")
		p2 := makePairTB(tb, app, "Walkover B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Walkover League")
		require.NoError(tb, app.Save(comp))

		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		match.Set("walkover_requested_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(match))

		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminBootstrap_TrueWithZeroCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin bootstrap card shown when zero competitions exist",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"bootstrap-create", "Crea tu primera competición"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminBootstrap_FalseWithOneCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin bootstrap card hidden when a competition exists",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "BootP1")
		p2 := makePairTB(tb, app, "BootP2")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "bootstrap-create", "bootstrap card must not appear when competitions exist")
	}
	s.Test(t)
}

func TestHome_AdminRedirectsToCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "admin GET / redirects to the single admin landing page",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: http.StatusFound,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestHome_NonAdminSeesNoAdminCards(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-admin sees no bootstrap or playoff prompt cards",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "P3Pair")
		oppPair := makePairTB(tb, app, "P3Opp")
		makeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "bootstrap-create", "non-admin must not see bootstrap card")
		assert.NotContains(tb, body, "playoff-prompt", "non-admin must not see playoff prompt")
		assert.NotContains(tb, body, "/admin/competitions", "non-admin must not see admin nav")
	}
	s.Test(t)
}

func TestHome_NoDatesGracefulDegradation(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "no competition dates = no organize tasks, graceful",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NoDtPair"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		myPair := makePairTB(tb, app, "NoDtPair")
		opp := makePairTB(tb, app, "NoDtOpp")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{myPair, opp})
		// No start_date/end_date set
		makeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Organiza antes del", "no deadline without dates")
		assert.NotContains(tb, body, "Vencido", "no warning without dates")
	}
	s.Test(t)
}

// Cluster: Document gate + Documentos tab

func TestCompetition_GateRendersForUnackedMandatory(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "competition page renders gate when mandatory doc unacked",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "GateA")
		p2 := makePairTB(tb, app, "GateB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Reglamento", true, "https://example.com/regla")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Reglamento")
		assert.NotContains(tb, body, "Jornadas", "gate page must not show the normal tabs")
	}
	s.Test(t)
}

func TestCompetition_AcceptDocsThenNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST accept-docs records ack; next GET shows normal page",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "AckA")
		p2 := makePairTB(tb, app, "AckB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Reglamento", true, "https://example.com/regla")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		compID = comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		userID = user.Id
		s.URL = "/competition/" + comp.Id + "/accept-docs"
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/competition/"+compID, res.Header.Get("HX-Redirect"))

		acks, err := app.FindRecordsByFilter("document_acks",
			"user = {:u} && competition = {:c}", "", 1, 0,
			map[string]any{"u": userID, "c": compID})
		require.NoError(tb, err)
		require.Len(tb, acks, 1, "ack record must exist")
		assert.NotEmpty(tb, acks[0].GetStringSlice("documents"), "acked docs must be recorded")
	}
	s.Test(t)
}

func TestCompetition_DocumentosTabShowsLeidoAfterAck(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "Documentos tab shows Leído badge for an acked doc, not for an unacked one",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "LeidoA")
		p2 := makePairTB(tb, app, "LeidoB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		ackedDoc := makeDocumentTB(tb, app, "Reglamento Leído", true, "https://example.com/reglamento")
		unackedDoc := makeDocumentTB(tb, app, "Normativa Sin Leer", false, "https://example.com/normativa")
		comp.Set("documents", []string{ackedDoc.Id, unackedDoc.Id})
		require.NoError(tb, app.Save(comp))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))

		ack, err := league.FindOrNewAck(app, comp.Id, user.Id)
		require.NoError(tb, err)
		ack.Set("documents", []string{ackedDoc.Id})
		require.NoError(tb, app.Save(ack))

		s.URL = "/competition/" + comp.Id
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		ackedIdx := strings.Index(body, "Reglamento Leído")
		unackedIdx := strings.Index(body, "Normativa Sin Leer")
		require.NotEqual(tb, -1, ackedIdx, "acked doc card present")
		require.NotEqual(tb, -1, unackedIdx, "unacked doc card present")

		leidoIdx := strings.Index(body, "Leído")
		require.NotEqual(tb, -1, leidoIdx, "Leído badge rendered")
		assert.Greater(tb, leidoIdx, ackedIdx, "Leído badge appears after the acked doc's title")
		nextCardIdx := unackedIdx
		if ackedIdx > unackedIdx {
			nextCardIdx = len(body)
		}
		assert.Less(tb, leidoIdx, nextCardIdx, "Leído badge belongs to the acked card, not the unacked one")
	}
	s.Test(t)
}

func TestCompetition_ReGateAfterNewMandatory(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "gate re-renders after a new mandatory doc is attached post-ack",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ReGateA")
		p2 := makePairTB(tb, app, "ReGateB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc1 := makeDocumentTB(tb, app, "Reglamento", true, "https://example.com/r1")
		comp.Set("documents", []string{doc1.Id})
		require.NoError(tb, app.Save(comp))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))

		ack, err := league.FindOrNewAck(app, comp.Id, user.Id)
		require.NoError(tb, err)
		ack.Set("documents", []string{doc1.Id})
		require.NoError(tb, app.Save(ack))

		doc2 := makeDocumentTB(tb, app, "Tarifas", true, "https://example.com/t")
		comp.Set("documents", []string{doc1.Id, doc2.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetition_NoMandatoryNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "no mandatory docs means no gate — normal competition page",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NoGateA")
		p2 := makePairTB(tb, app, "NoGateB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Info", false, "https://example.com/info")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Documentos obligatorios", "no gate for non-mandatory docs")
	}
	s.Test(t)
}

func TestCompetition_DocumentosTabShown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "competition page shows Documentos tab when docs attached",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "TabA")
		p2 := makePairTB(tb, app, "TabB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Info", false, "https://example.com/info")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Documentos")
		assert.Contains(tb, body, "Info")
	}
	s.Test(t)
}

func TestCompetition_NonParticipantNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "non-participant sees normal page even with mandatory docs",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "PartA")
		p2 := makePairTB(tb, app, "PartB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := makeDocumentTB(tb, app, "Reglamento", true, "https://example.com/r")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		outsider := makeUserTB(tb, app, "Outsider", "")
		s.URL = "/competition/" + comp.Id
		s.Headers = authHeaders(tb, outsider)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "Documentos obligatorios")
	}
	s.Test(t)
}

func TestPopulateFeeder(t *testing.T) {
	t.Parallel()

	t.Run("empty slots get feeder labels", func(t *testing.T) {
		m := MatchCard{}
		m.PopulateFeeder(1, 0)
		assert.Equal(t, "Ganador de J1-1", m.Feeder1)
		assert.Equal(t, "Ganador de J1-2", m.Feeder2)
	})

	t.Run("filled slots keep no feeder", func(t *testing.T) {
		m := MatchCard{Pair1Name: "A"}
		m.PopulateFeeder(1, 0)
		assert.Empty(t, m.Feeder1)
		assert.Equal(t, "Ganador de J1-2", m.Feeder2)
	})
}

func TestBuildBracket_PassesThroughFeeders(t *testing.T) {
	t.Parallel()
	rounds := []RoundView{
		{RoundNumber: 1, Matches: []MatchCard{
			{Pair1Name: "Team A", Pair2Name: "Team B"},
			{Pair1Name: "Team C", Pair2Name: "Team D"},
		}},
		{RoundNumber: 2, Matches: []MatchCard{
			{Feeder1: "Ganador de J1-1", Feeder2: "Ganador de J1-2"},
		}},
	}

	bracket := buildBracket(rounds, 2)
	require.Len(t, bracket, 2)

	final := bracket[1].Matches[0]
	assert.Equal(t, "Ganador de J1-1", final.Feeder1)
	assert.Equal(t, "Ganador de J1-2", final.Feeder2)

	semi := bracket[0].Matches[0]
	assert.Empty(t, semi.Feeder1, "round 1 should have no feeders")
	assert.Empty(t, semi.Feeder2, "round 1 should have no feeders")
}

func TestHome_OnboardChecklist_ShownWhenMandatoryDocPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "onboarding checklist hidden when profile done, but a docs action shows on home",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "OnbA")
		p2 := makePairTB(tb, app, "OnbB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		docCol, err := app.FindCollectionByNameOrId("documents")
		require.NoError(tb, err)
		doc := core.NewRecord(docCol)
		doc.Set("title", "Reglamento Test")
		doc.Set("link", "https://example.com")
		doc.Set("is_mandatory", true)
		doc.Set("is_default", false)
		require.NoError(tb, app.Save(doc))

		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "onboard-checklist", "checklist hidden when profile done")
		assert.Contains(tb, body, "Lee los documentos", "a docs home action must flag the unacked mandatory doc")
	}
	s.Test(t)
}

func TestHome_OnboardChecklist_HiddenWhenAllDone(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "onboarding checklist hidden when all steps done",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"PadelLeague"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DoneA")
		p2 := makePairTB(tb, app, "DoneB")
		makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "onboard-checklist", "checklist should be hidden")
		assert.NotContains(tb, body, "Primeros pasos", "checklist heading should be hidden")
	}
	s.Test(t)
}

func TestBuildHomeActions_AllKindsMap(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga", RoundNumber: 1},
		{Kind: league.TaskOrganize, MatchID: "m2", Opponent: "Rival2", CompetitionName: "Liga", RoundNumber: 2, Warning: league.WarnUrgent, Description: "Organiza antes del 15/03"},
		{Kind: league.TaskPlay, MatchID: "m3", Opponent: "Rival3", CompetitionName: "Liga", RoundNumber: 3},
	}
	pending := []PendingAction{
		{MatchID: "m4", Opponent: "Rival4", ActionType: "confirm_score", Description: "6-4 6-3"},
		{MatchID: "m5", Opponent: "Rival5", ActionType: "respond_result", Description: "pendiente"},
		{MatchID: "m6", Opponent: "Rival6", ActionType: "respond_proposal", Description: "Propuesta de horario pendiente"},
	}
	actions := buildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 6)

	kinds := map[string]bool{}
	for _, a := range actions {
		kinds[a.Kind] = true
		assert.NotEmpty(t, a.URL, "URL must be set for %s", a.MatchID)
		assert.NotEmpty(t, a.Accent, "Accent must be set for %s", a.MatchID)
	}
	assert.True(t, kinds["dispute"], "dispute kind must be present")
	assert.True(t, kinds["confirm"], "confirm kind must be present")
	assert.True(t, kinds["respond"], "respond kind must be present")
	assert.True(t, kinds["organize"], "organize kind must be present")
	assert.True(t, kinds["play"], "play kind must be present")
}

func TestBuildHomeActions_DedupByMatchID(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskOrganize, MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga", Warning: league.WarnHeadsUp, Description: "Organiza"},
	}
	pending := []PendingAction{
		{MatchID: "m1", Opponent: "Rival", ActionType: "confirm_score", Description: "6-4 6-3"},
	}
	actions := buildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 1, "same MatchID should dedup to one action")
	assert.Equal(t, "confirm", actions[0].Kind, "confirm beats organize in priority")
}

func TestBuildHomeActions_OrderingPriority(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskPlay, MatchID: "m3", Opponent: "R3", CompetitionName: "L", RoundNumber: 1},
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "R1", CompetitionName: "L", RoundNumber: 1},
		{Kind: league.TaskOrganize, MatchID: "m4", Opponent: "R4", CompetitionName: "L", Warning: league.WarnUrgent, Description: "Organiza"},
	}
	pending := []PendingAction{
		{MatchID: "m2", Opponent: "R2", ActionType: "confirm_score", Description: "6-4 6-3"},
	}
	actions := buildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 4)
	assert.Equal(t, "dispute", actions[0].Kind)
	assert.Equal(t, "confirm", actions[1].Kind)
	assert.Equal(t, "organize", actions[2].Kind)
	assert.Equal(t, "play", actions[3].Kind)
}

func TestBuildHomeActions_DocsRankedAboveOrganize(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskOrganize, MatchID: "m1", Opponent: "R1", CompetitionName: "L", Warning: league.WarnUrgent, Description: "Organiza"},
	}
	docs := []DocsAction{{CompID: "c1", CompName: "Liga Docs"}}
	actions := buildHomeActions(tasks, nil, nil, docs)
	require.Len(t, actions, 2)
	assert.Equal(t, "docs", actions[0].Kind, "docs must rank above organize")
	assert.Equal(t, "organize", actions[1].Kind)
	assert.Equal(t, "Lee los documentos", actions[0].Title)
	assert.Equal(t, "Liga Docs", actions[0].Detail)
	assert.Equal(t, "/competition/c1", actions[0].URL)
	assert.Equal(t, "warning", actions[0].Accent)
}

func TestBuildHomeActions_NextMatchSynthesized(t *testing.T) {
	next := &NextMatch{
		MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga",
		RoundNumber: 2, ScheduleStatus: "unscheduled",
	}
	actions := buildHomeActions(nil, nil, next, nil)
	require.Len(t, actions, 1)
	assert.Equal(t, "organize", actions[0].Kind, "unscheduled NextMatch synthesizes organize")
	assert.Contains(t, actions[0].Title, "Propón")

	next.ScheduleStatus = "confirmed"
	next.ProposedDate = "2026-03-15 18:00"
	actions = buildHomeActions(nil, nil, next, nil)
	require.Len(t, actions, 1)
	assert.Equal(t, "play", actions[0].Kind, "scheduled NextMatch becomes play")
	assert.Contains(t, actions[0].Detail, "15/03/2026 18:00")
}

func TestBuildHomeActions_NextMatchDedupWithTask(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "R", CompetitionName: "L", RoundNumber: 1},
	}
	next := &NextMatch{MatchID: "m1", Opponent: "R", CompetitionName: "L", ScheduleStatus: "confirmed"}
	actions := buildHomeActions(tasks, nil, next, nil)
	require.Len(t, actions, 1, "NextMatch deduped with existing task")
	assert.Equal(t, "dispute", actions[0].Kind, "dispute wins over play from NextMatch")
}
