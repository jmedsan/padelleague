package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// Cluster 1: Home shows only player's competitions, sets nextMatch

// Contact section: footer shows WhatsApp/email links when app_settings has
// them set, hides the section entirely when both are empty.

func TestHomeGen2_ContactSectionShowsWhenSet(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "home footer shows the contact section when WhatsApp and email are set",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`href="https://wa.me/34612345678"`,
			`href="mailto:admin@example.com"`,
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		settings, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
		require.NoError(tb, err)
		require.Len(tb, settings, 1)
		settings[0].Set("contact_whatsapp", "+34612345678")
		settings[0].Set("contact_email", "admin@example.com")
		require.NoError(tb, app.Save(settings[0]))

		user := handlers.MakeUserTB(tb, app, "Contact Player", "")
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_ContactSectionHiddenWhenEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "home footer hides the contact section when both WhatsApp and email are empty",
		Method:             http.MethodGet,
		URL:                "/",
		ExpectedStatus:     200,
		NotExpectedContent: []string{"Contacto:"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "No Contact Player", "")
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_OnlyPlayerCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "home shows only competitions the player is in",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"MyComp"},
	}
	var otherCompName string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "MyPair")
		otherPair := handlers.MakePairTB(tb, app, "OtherPair")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, otherPair})
		comp.Set("name", "MyComp")
		require.NoError(tb, app.Save(comp))

		alienP1 := handlers.MakePairTB(tb, app, "AlienA")
		alienP2 := handlers.MakePairTB(tb, app, "AlienB")
		alienComp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{alienP1, alienP2})
		alienComp.Set("name", "AlienComp")
		require.NoError(tb, app.Save(alienComp))
		otherCompName = "AlienComp"

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, otherCompName, "alien competition must not appear")
	}
	s.Test(t)
}

func TestHomeGen2_NextMatchFromFirstPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "home synthesizes organize action for unscheduled next match",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NMOpp", "Propón una fecha"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "NMPair")
		oppPair := handlers.MakePairTB(tb, app, "NMOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_NextMatchNotDuplicatedInUpcoming(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "next-match action's opponent is not also duplicated in the competition card's pending list",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NMDupOpp", "Propón una fecha"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "NMDupPair")
		oppPair := handlers.MakePairTB(tb, app, "NMDupOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Equal(tb, 2, strings.Count(body, "NMDupOpp"),
			"opponent name should appear in the action card and the upcoming section")
	}
	s.Test(t)
}

// Cluster 2: Pending match counting

func TestHomeGen2_PendingCount(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "home counts pending matches correctly",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"7 partidos por jugar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "CapPair")
		opponents := make([]*core.Record, 7)
		for i := range opponents {
			opponents[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("CapOpp%d", i))
		}
		allPairs := append([]*core.Record{myPair}, opponents...)
		comp := handlers.MakeCompetitionTB(tb, app, "league", allPairs)
		for _, opp := range opponents {
			handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")
		}
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "7 partidos por jugar")
		assert.Contains(tb, body, "upcoming-matches", "upcoming section should render for pending matches")
	}
	s.Test(t)
}

// Cluster 3: Proposal schedule status

func TestHomeGen2_AcceptedProposalShowsConfirmado(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "accepted proposal shows confirmed badge in upcoming",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"upcoming-matches", "Confirmada", "Padel 360"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "SchAccPair")
		oppPair := handlers.MakePairTB(tb, app, "SchAccOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, match.Id, user.Id, "accepted",
			`{"date":"2026-09-15","time":"18:00","venue_name":"Padel 360"}`)

		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_PendingProposalShowsPropuestaEnviada(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "pending proposal shows proposed badge in upcoming",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"upcoming-matches", "Propuesta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "SchPenPair")
		oppPair := handlers.MakePairTB(tb, app, "SchPenOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, match.Id, user.Id, "pending",
			`{"date":"2026-10-01","time":"20:00","venue_text":"Mi club"}`)

		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// Cluster 4: Proposer team check (respond_proposal action)

func TestHomeGen2_OpponentProposalCreatesAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "opponent proposal creates respond action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Propuesta de horario pendiente", "Acciones pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "RActMy")
		oppPair := handlers.MakePairTB(tb, app, "RActOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		oppUser := oppPair.GetString("player1")
		createProposal(tb, app, match.Id, oppUser, "pending",
			`{"date":"2026-09-20","time":"19:00"}`)

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_OwnProposalNoAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "own proposal does not create respond action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "OwnPropMy")
		oppPair := handlers.MakePairTB(tb, app, "OwnPropOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")

		myUser := myPair.GetString("player1")
		createProposal(tb, app, match.Id, myUser, "pending",
			`{"date":"2026-09-20","time":"19:00"}`)

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Propuesta de horario pendiente",
			"own proposal should not create a respond action")
	}
	s.Test(t)
}

// Cluster 5: Score confirm team check

func TestHomeGen2_OpponentScoreShowsConfirmAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "opponent score shows confirm action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Responder resultado: 6-3 6-4", "Acciones pendientes"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "ConfMy")
		oppPair := handlers.MakePairTB(tb, app, "ConfOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})

		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")
		fresh, err := app.FindRecordById("matches", match.Id)
		require.NoError(tb, err)
		fresh.Set("status", "confirmed")
		fresh.Set("scores", "6-3 6-4")
		fresh.Set("submitted_by", oppPair.GetString("player1"))
		require.NoError(tb, app.Save(fresh))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeGen2_OwnScoreNoConfirmAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "own score shows no confirm action",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "OwnConfMy")
		oppPair := handlers.MakePairTB(tb, app, "OwnConfOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})

		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, oppPair.Id, "pending")
		fresh, err := app.FindRecordById("matches", match.Id)
		require.NoError(tb, err)
		fresh.Set("status", "confirmed")
		fresh.Set("scores", "6-3 6-4")
		fresh.Set("submitted_by", myPair.GetString("player1"))
		require.NoError(tb, app.Save(fresh))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Responder resultado",
			"own score should not show respond action")
	}
	s.Test(t)
}

// Cluster 6: Penalty flag in competition page

func TestCompetitionGen2_WithPenaltyShowsPenColumn(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "competition with penalty shows Pen column",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Pen"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PenA")
		p2 := handlers.MakePairTB(tb, app, "PenB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakePenaltyTB(tb, app, comp.Id, p1.Id, 1, "Prueba", "", false)

		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionGen2_NoPenaltyHidesPenColumn(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "competition without penalty hides Pen column",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"NoPenA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoPenA")
		p2 := handlers.MakePairTB(tb, app, "NoPenB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, ">Pen</th>", "no penalty column header when no penalties")
	}
	s.Test(t)
}

// TestCompetitionGen2_PublishedNoPlayShowsStandingsEmptyState verifies a
// published league with zero played matches still shows the Clasificación
// tab (matching the always-shown admin card) with the same empty-state text
// admin uses, instead of hiding the tab entirely.
func TestCompetitionGen2_PublishedNoPlayShowsStandingsEmptyState(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "published league, no played matches, shows Clasificación tab + empty state",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`aria-label="Clasificación"`,
			"No hay datos de clasificación todavía",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "EmptyStA")
		p2 := handlers.MakePairTB(tb, app, "EmptyStB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// TestCompetitionGen2_ProvisionalResultShowsInStandingsWithTooltip verifies
// a pending result proposal counts in the Clasificación right away, with a
// tooltip on both involved pairs' rows warning the result isn't confirmed.
func TestCompetitionGen2_ProvisionalResultShowsInStandingsWithTooltip(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "pending result proposal shows in standings with a tooltip",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"ProvA",
			`data-tip="Incluye resultados sin confirmar"`,
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ProvA")
		p2 := handlers.MakePairTB(tb, app, "ProvB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		createResultProposal(tb, app, m.Id, p1.GetString("player1"), "6-3 6-4")

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// TestCompetitionGen2_NoAnnouncementsShowsEmptyState verifies the Avisos
// tab always shows for a player, even with zero announcements — matching
// admin's always-shown card, same class as Clasificación/Documentos.
func TestCompetitionGen2_NoAnnouncementsShowsEmptyState(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "no announcements shows Avisos tab + empty state",
		Method:         http.MethodGet,
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`aria-label="Avisos"`,
			"No hay avisos todavía",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoAnnA")
		p2 := handlers.MakePairTB(tb, app, "NoAnnB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// TestCompetitionGen2_DraftCalendarHidesStandingsTab verifies a draft
// calendar hides the Clasificación tab entirely from a non-admin viewer —
// players must not see anything before the admin publishes.
func TestCompetitionGen2_DraftCalendarHidesStandingsTab(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "draft calendar hides Clasificación tab",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"DraftStA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "DraftStA")
		p2 := handlers.MakePairTB(tb, app, "DraftStB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("calendar_status", "draft")
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, `aria-label="Clasificación"`, "draft calendar must hide the Clasificación tab from a player")
	}
	s.Test(t)
}

// Cluster 7: handlers.FirstIncompleteRound sets AutoExpandRound

func TestCompetitionGen2_AutoExpandRound(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "auto-expand targets first incomplete round",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornada 2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ExpA")
		p2 := handlers.MakePairTB(tb, app, "ExpB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		makeFinalMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "6-3 6-4", p1.Id)

		m2 := handlers.MakeMatchTB(tb, app, comp.Id, p2.Id, p1.Id, "pending")
		m2.Set("round_number", 2)
		require.NoError(tb, app.Save(m2))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
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

// createResultProposal creates a pending result_submission proposal — the
// live, not-yet-accepted state provisional standings must count.
func createResultProposal(tb testing.TB, app core.App, matchID, authorID, scores string) {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	msg := core.NewRecord(col)
	msg.Set("match", matchID)
	msg.Set("type", "result_submission")
	msg.Set("author", authorID)
	msg.Set("proposal_status", "pending")
	msg.Set("content", scores)
	pd, _ := json.Marshal(map[string]string{"scores": scores})
	msg.Set("proposal_data", string(pd))
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
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET / with active competition shows home data",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Home A", "Home B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Home A")
		p2 := handlers.MakePairTB(tb, app, "Home B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Pending match — triggers buildNextMatch, buildHomeCompetition
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Confirmed match — triggers findUnconfirmedScores
		confirmed := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
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
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetitionPageWithMatches(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /competition/{id} with matches shows pair names",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Comp A", "Comp B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Comp A")
		p2 := handlers.MakePairTB(tb, app, "Comp B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHomeWithScheduledMatch(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET / with pending proposal and scheduled match",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Sched A"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Sched A")
		p2 := handlers.MakePairTB(tb, app, "Sched B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Match with scheduled date and a pending proposal
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
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
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestHome_RecentResultsNonEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "home page shows recent results for final matches newest-first",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Mis últimos partidos", "6-1 6-2", "6-3 6-4"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Result A")
		p2 := handlers.MakePairTB(tb, app, "Result B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Liga Recientes")
		require.NoError(tb, app.Save(comp))

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
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "6-1 6-2", "first final match score must appear")
		assert.Contains(tb, body, "6-3 6-4", "second final match score must appear")
		assert.Contains(tb, body, "Mis últimos partidos", "recent results heading must appear")
		assert.Contains(tb, body, "Liga Recientes", "L1: competition name must appear on each recent match row")
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
		TestAppFactory: handlers.TestAppFactory,
		Name:           "unified actions: dispute > organize; scheduled in upcoming",
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
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "UrgPair")
		disputeOpp := handlers.MakePairTB(tb, app, "DisputeOpp")
		playOpp := handlers.MakePairTB(tb, app, "PlayOpp")
		orgOpp := handlers.MakePairTB(tb, app, "OrgOpp")

		allPairs := []*core.Record{myPair, disputeOpp, playOpp, orgOpp}
		comp := handlers.MakeCompetitionTB(tb, app, "league", allPairs)
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		// Disputed match (should be hero)
		disputeMatch := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, disputeOpp.Id, "disputed")
		_ = disputeMatch

		// Match with accepted proposal (play task)
		playMatch := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, playOpp.Id, "pending")
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		createProposal(tb, app, playMatch.Id, user.Id, "accepted",
			`{"date":"2026-08-20","time":"19:00","venue_name":"Padel 360"}`)

		// Pending match with no proposal (organize task — deadline well past)
		orgMatch := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, orgOpp.Id, "pending")
		orgMatch.Set("round_number", 1)
		require.NoError(tb, app.Save(orgMatch))

		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "bg-error/10", "dispute action must use error accent")
		assert.Contains(tb, body, "PlayOpp", "scheduled match must appear in upcoming")
		assert.Contains(tb, body, "OrgOpp", "organize task must appear")
		assert.Contains(tb, body, "Organiza antes del", "organize deadline")
	}
	s.Test(t)
}

func TestHome_OrganizeWarningBadges(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "organize tasks show correct warning accent",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Organiza antes del"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "WarnPair")
		opp := handlers.MakePairTB(tb, app, "WarnOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, opp})
		comp.Set("start_date", "2026-06-01 00:00:00.000Z")
		comp.Set("end_date", "2026-07-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		// Round 1 of 1 — deadline is end_date (2026-07-01), well past
		match := handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")
		match.Set("round_number", 1)
		require.NoError(tb, app.Save(match))

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "bg-error/10", "overdue organize action must use error accent")
		assert.Contains(tb, body, "Organiza antes del", "deadline text must appear")
	}
	s.Test(t)
}

// Cluster: Admin dashboard (T4)

func TestHome_AdminSetupChecklist(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
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
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "SetupP1")
		p2 := handlers.MakePairTB(tb, app, "SetupP2")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		comp.Set("name", "SetupComp")
		require.NoError(tb, app.Save(comp))

		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Activar", "not ready — missing fixtures and dates")
	}
	s.Test(t)
}

func TestHome_AdminSetupReady(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin sees Activar when setup complete",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Activar", "ReadyComp"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ReadyP1")
		p2 := handlers.MakePairTB(tb, app, "ReadyP2")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		comp.Set("name", "ReadyComp")
		comp.Set("start_date", "2026-09-01 00:00:00.000Z")
		comp.Set("end_date", "2026-12-01 00:00:00.000Z")
		require.NoError(tb, app.Save(comp))
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminAlerts(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
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
		setupProductionRoutes(tb, app, e)

		dispP1 := handlers.MakePairTB(tb, app, "AlertDispP1")
		dispP2 := handlers.MakePairTB(tb, app, "AlertDispP2")
		ovdP1 := handlers.MakePairTB(tb, app, "AlertOvdP1")
		ovdP2 := handlers.MakePairTB(tb, app, "AlertOvdP2")

		allPairs := []*core.Record{dispP1, dispP2, ovdP1, ovdP2}
		comp := handlers.MakeCompetitionTB(tb, app, "league", allPairs)
		comp.Set("start_date", "2026-05-01 00:00:00.000Z")
		comp.Set("end_date", "2026-06-01 00:00:00.000Z")
		comp.Set("arrange_grace_days", 3)
		comp.Set("rounds", 1)
		comp.Set("recovery_days", 9999) // stay out of the finished-by-date phase
		require.NoError(tb, app.Save(comp))

		disputed := handlers.MakeMatchTB(tb, app, comp.Id, dispP1.Id, dispP2.Id, "disputed")
		disputed.Set("scores", "6-3 6-4")
		disputed.Set("submitted_by", dispP1.GetString("player1"))
		disputed.Set("disputed_by", dispP2.GetString("player1"))
		disputed.Set("disputed_scores", "6-4 6-3")
		disputed.Set("dispute_notes", "Marcador incorrecto")
		require.NoError(tb, app.Save(disputed))
		handlers.MakeMatchTB(tb, app, comp.Id, ovdP1.Id, ovdP2.Id, "pending")

		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminWalkoverAlert(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
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
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Walkover A")
		p2 := handlers.MakePairTB(tb, app, "Walkover B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Walkover League")
		require.NoError(tb, app.Save(comp))

		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		match.Set("walkover_requested_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(match))

		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminBootstrap_TrueWithZeroCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin bootstrap card shown when zero competitions exist",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"bootstrap-create", "Crea tu primera competición"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestHome_AdminBootstrap_FalseWithOneCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "admin bootstrap card hidden when a competition exists",
		Method:          http.MethodGet,
		URL:             "/admin/competitions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Competiciones"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "BootP1")
		p2 := handlers.MakePairTB(tb, app, "BootP2")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("active", false)
		require.NoError(tb, app.Save(comp))
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "bootstrap-create", "bootstrap card must not appear when competitions exist")
	}
	s.Test(t)
}

func TestHome_AdminRedirectsToCompetitions(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "admin GET / redirects to the single admin landing page",
		Method:         http.MethodGet,
		URL:            "/",
		ExpectedStatus: http.StatusFound,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/admin/competitions", res.Header.Get("Location"))
	}
	s.Test(t)
}

func TestHome_NonAdminSeesNoAdminCards(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "non-admin sees no bootstrap or playoff prompt cards",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "P3Pair")
		oppPair := handlers.MakePairTB(tb, app, "P3Opp")
		handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, oppPair})
		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "bootstrap-create", "non-admin must not see bootstrap card")
		assert.NotContains(tb, body, "playoff-prompt", "non-admin must not see playoff prompt")
		assert.NotContains(tb, body, "/admin/competitions", "non-admin must not see admin nav")
	}
	s.Test(t)
}

func TestHome_NoDatesGracefulDegradation(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "no competition dates = no organize tasks, graceful",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"NoDtPair"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		myPair := handlers.MakePairTB(tb, app, "NoDtPair")
		opp := handlers.MakePairTB(tb, app, "NoDtOpp")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{myPair, opp})
		// No start_date/end_date set
		handlers.MakeMatchTB(tb, app, comp.Id, myPair.Id, opp.Id, "pending")

		user, _ := app.FindRecordById("users", myPair.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Organiza antes del", "no deadline without dates")
		assert.NotContains(tb, body, "Vencido", "no warning without dates")
	}
	s.Test(t)
}

// Cluster: Document gate + Documentos tab

func TestCompetition_GateRendersForUnackedMandatory(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "competition page renders gate when mandatory doc unacked",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "GateA")
		p2 := handlers.MakePairTB(tb, app, "GateB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Reglamento", true, "https://example.com/regla")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Reglamento")
		assert.NotContains(tb, body, "Jornadas", "gate page must not show the normal tabs")
	}
	s.Test(t)
}

// gateAdminPlayerTB builds a league with one unacked mandatory document where
// p1's first player also holds the admin role: an admin who plays.
func gateAdminPlayerTB(tb testing.TB, app *tests.TestApp, prefix string) (comp, match, adminPlayer *core.Record) {
	p1 := handlers.MakePairTB(tb, app, prefix+"A")
	p2 := handlers.MakePairTB(tb, app, prefix+"B")
	comp = handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
	doc := handlers.MakeDocumentTB(tb, app, "Reglamento", true, "https://example.com/regla")
	comp.Set("documents", []string{doc.Id})
	require.NoError(tb, app.Save(comp))
	match = handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
	adminPlayer, err := app.FindRecordById("users", p1.GetString("player1"))
	require.NoError(tb, err)
	adminPlayer.Set("roles", []string{"player", "admin"})
	require.NoError(tb, app.Save(adminPlayer))
	return comp, match, adminPlayer
}

// Admin-as-player purity: in player view the admin is gated like any player.
func TestCompetition_GateAppliesToAdminInPlayerView(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "competition page gates an admin who plays, in player view",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos obligatorios", "Reglamento"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		comp, _, adminPlayer := gateAdminPlayerTB(tb, app, "GateAPV")
		s.URL = "/competition/" + comp.Id
		hdrs := handlers.AuthHeaders(tb, adminPlayer)
		hdrs["Cookie"] = "view_as=player"
		s.Headers = hdrs
	}
	s.Test(t)
}

// The admin view is never gated, even for an admin who plays in the league.
func TestCompetition_NoGateInAdminView(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "competition page does not gate the admin view",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Jornadas"},
		NotExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		comp, _, adminPlayer := gateAdminPlayerTB(tb, app, "GateAAV")
		s.URL = "/competition/" + comp.Id
		s.Headers = handlers.AuthHeaders(tb, adminPlayer)
	}
	s.Test(t)
}

// A match action by an admin in player view hits the same gate as a player's:
// redirected to the competition, nothing written.
func TestMatchAction_GateAppliesToAdminInPlayerView(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "thread message from an admin who plays, in player view, is doc-gated",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	var matchID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		comp, match, adminPlayer := gateAdminPlayerTB(tb, app, "GateAPM")
		matchID, compID = match.Id, comp.Id
		s.URL = "/match/" + match.Id + "/thread/message"
		s.Body = strings.NewReader("content=Hola&type=chat")
		hdrs := handlers.AuthHeaders(tb, adminPlayer)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		hdrs["Cookie"] = "view_as=player"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/competition/"+compID, res.Header.Get("Location"))
		msgs, err := app.FindRecordsByFilter("match_messages", "match = {:mid} && type = 'chat'", "", 0, 0, map[string]any{"mid": matchID})
		require.NoError(tb, err)
		assert.Empty(tb, msgs)
	}
	s.Test(t)
}

func TestCompetition_AcceptDocsThenNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST accept-docs records ack; next GET shows normal page",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var compID, userID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AckA")
		p2 := handlers.MakePairTB(tb, app, "AckB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Reglamento", true, "https://example.com/regla")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		compID = comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		userID = user.Id
		s.URL = "/competition/" + comp.Id + "/accept-docs"
		s.Headers = handlers.AuthHeaders(tb, user)
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
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "Documentos tab shows Leído badge for an acked doc, not for an unacked one",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "LeidoA")
		p2 := handlers.MakePairTB(tb, app, "LeidoB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		ackedDoc := handlers.MakeDocumentTB(tb, app, "Reglamento Leído", true, "https://example.com/reglamento")
		unackedDoc := handlers.MakeDocumentTB(tb, app, "Normativa Sin Leer", false, "https://example.com/normativa")
		comp.Set("documents", []string{ackedDoc.Id, unackedDoc.Id})
		require.NoError(tb, app.Save(comp))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))

		ack, err := league.FindOrNewAck(app, comp.Id, user.Id)
		require.NoError(tb, err)
		ack.Set("documents", []string{ackedDoc.Id})
		require.NoError(tb, app.Save(ack))

		s.URL = "/competition/" + comp.Id
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
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
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "gate re-renders after a new mandatory doc is attached post-ack",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "ReGateA")
		p2 := handlers.MakePairTB(tb, app, "ReGateB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc1 := handlers.MakeDocumentTB(tb, app, "Reglamento", true, "https://example.com/r1")
		comp.Set("documents", []string{doc1.Id})
		require.NoError(tb, app.Save(comp))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))

		ack, err := league.FindOrNewAck(app, comp.Id, user.Id)
		require.NoError(tb, err)
		ack.Set("documents", []string{doc1.Id})
		require.NoError(tb, app.Save(ack))

		doc2 := handlers.MakeDocumentTB(tb, app, "Tarifas", true, "https://example.com/t")
		comp.Set("documents", []string{doc1.Id, doc2.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

func TestCompetition_NoMandatoryNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "no mandatory docs means no gate — normal competition page",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoGateA")
		p2 := handlers.MakePairTB(tb, app, "NoGateB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Info", false, "https://example.com/info")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Documentos obligatorios", "no gate for non-mandatory docs")
	}
	s.Test(t)
}

func TestCompetition_DocumentosTabShown(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "competition page shows Documentos tab when docs attached",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Documentos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TabA")
		p2 := handlers.MakePairTB(tb, app, "TabB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Info", false, "https://example.com/info")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Documentos")
		assert.Contains(tb, body, "Info")
	}
	s.Test(t)
}

func TestCompetition_NonParticipantNoGate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "non-participant sees normal page even with mandatory docs",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Jornadas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "PartA")
		p2 := handlers.MakePairTB(tb, app, "PartB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		doc := handlers.MakeDocumentTB(tb, app, "Reglamento", true, "https://example.com/r")
		comp.Set("documents", []string{doc.Id})
		require.NoError(tb, app.Save(comp))

		outsider := handlers.MakeUserTB(tb, app, "Outsider", "")
		s.URL = "/competition/" + comp.Id
		s.Headers = handlers.AuthHeaders(tb, outsider)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "Documentos obligatorios")
	}
	s.Test(t)
}

// TestBuildBracket_Slot verifies the per-round CSS slot height used by the
// bracket connector lines doubles each round, so a round-2 match's slot
// always spans the midpoint of its two round-1 feeder matches.
func TestBuildBracket_Slot(t *testing.T) {
	t.Parallel()
	rounds := []handlers.RoundView{
		{RoundNumber: 1, Matches: []handlers.MatchCard{
			{Pair1Name: "Team A", Pair2Name: "Team B"},
			{Pair1Name: "Team C", Pair2Name: "Team D"},
		}},
		{RoundNumber: 2, Matches: []handlers.MatchCard{
			{},
		}},
	}

	bracket := handlers.BuildBracket(rounds, 2)
	require.Len(t, bracket, 2)

	assert.Equal(t, 0, bracket[0].Index)
	assert.Equal(t, 84, bracket[0].Slot)
	assert.Equal(t, 1, bracket[1].Index)
	assert.Equal(t, 168, bracket[1].Slot)
}

func TestHome_MandatoryDocShowsAction(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "mandatory doc pending shows a docs action on home",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Liga Dale Fuerte"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OnbA")
		p2 := handlers.MakePairTB(tb, app, "OnbB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

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
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Lee los documentos", "a docs home action must flag the unacked mandatory doc")
	}
	s.Test(t)
}

func TestBuildHomeActions_AllKindsMap(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga", RoundNumber: 1},
		{Kind: league.TaskOrganize, MatchID: "m2", Opponent: "Rival2", CompetitionName: "Liga", RoundNumber: 2, Warning: league.WarnUrgent, Description: "Organiza antes del 15/03"},
	}
	pending := []handlers.PendingAction{
		{MatchID: "m4", Opponent: "Rival4", ActionType: "confirm_score", Description: "6-4 6-3"},
		{MatchID: "m5", Opponent: "Rival5", ActionType: "respond_result", Description: "pendiente"},
		{MatchID: "m6", Opponent: "Rival6", ActionType: "respond_proposal", Description: "Propuesta de horario pendiente"},
	}
	actions := handlers.BuildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 5)

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
}

func TestBuildHomeActions_DedupByMatchID(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskOrganize, MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga", Warning: league.WarnHeadsUp, Description: "Organiza"},
	}
	pending := []handlers.PendingAction{
		{MatchID: "m1", Opponent: "Rival", ActionType: "confirm_score", Description: "6-4 6-3"},
	}
	actions := handlers.BuildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 1, "same MatchID should dedup to one action")
	assert.Equal(t, "confirm", actions[0].Kind, "confirm beats organize in priority")
}

func TestBuildHomeActions_OrderingPriority(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "R1", CompetitionName: "L", RoundNumber: 1},
		{Kind: league.TaskOrganize, MatchID: "m4", Opponent: "R4", CompetitionName: "L", Warning: league.WarnUrgent, Description: "Organiza"},
	}
	pending := []handlers.PendingAction{
		{MatchID: "m2", Opponent: "R2", ActionType: "confirm_score", Description: "6-4 6-3"},
	}
	actions := handlers.BuildHomeActions(tasks, pending, nil, nil)
	require.Len(t, actions, 3)
	assert.Equal(t, "dispute", actions[0].Kind)
	assert.Equal(t, "confirm", actions[1].Kind)
	assert.Equal(t, "organize", actions[2].Kind)
}

func TestBuildHomeActions_DocsRankedAboveOrganize(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskOrganize, MatchID: "m1", Opponent: "R1", CompetitionName: "L", Warning: league.WarnUrgent, Description: "Organiza"},
	}
	docs := []handlers.DocsAction{{CompID: "c1", CompName: "Liga Docs"}}
	actions := handlers.BuildHomeActions(tasks, nil, nil, docs)
	require.Len(t, actions, 2)
	assert.Equal(t, "docs", actions[0].Kind, "docs must rank above organize")
	assert.Equal(t, "organize", actions[1].Kind)
	assert.Equal(t, "Lee los documentos", actions[0].Title)
	assert.Equal(t, "Liga Docs", actions[0].Detail)
	assert.Equal(t, "/competition/c1", actions[0].URL)
	assert.Equal(t, "warning", actions[0].Accent)
}

func TestBuildHomeActions_NextMatchSynthesized(t *testing.T) {
	next := &handlers.NextMatch{
		MatchID: "m1", Opponent: "Rival", CompetitionName: "Liga",
		RoundNumber: 2, ScheduleStatus: "unscheduled",
	}
	actions := handlers.BuildHomeActions(nil, nil, next, nil)
	require.Len(t, actions, 1)
	assert.Equal(t, "organize", actions[0].Kind, "unscheduled handlers.NextMatch synthesizes organize")
	assert.Contains(t, actions[0].Title, "Propón")

	next.ScheduleStatus = "confirmed"
	next.ProposedDate = "2026-03-15 18:00"
	actions = handlers.BuildHomeActions(nil, nil, next, nil)
	require.Len(t, actions, 0, "confirmed match needs no action card — it shows in upcoming only")
}

func TestBuildHomeActions_TaskPlayDropped(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskPlay, MatchID: "m1", Opponent: "R", CompetitionName: "L", RoundNumber: 1},
	}
	actions := handlers.BuildHomeActions(tasks, nil, nil, nil)
	require.Len(t, actions, 0, "TaskPlay must be dropped — confirmed matches show in upcoming only")
}

func TestBuildHomeActions_NextMatchDedupWithTask(t *testing.T) {
	tasks := []league.PlayerTask{
		{Kind: league.TaskDispute, MatchID: "m1", Opponent: "R", CompetitionName: "L", RoundNumber: 1},
	}
	next := &handlers.NextMatch{MatchID: "m1", Opponent: "R", CompetitionName: "L", ScheduleStatus: "confirmed"}
	actions := handlers.BuildHomeActions(tasks, nil, next, nil)
	require.Len(t, actions, 1, "handlers.NextMatch deduped with existing task")
	assert.Equal(t, "dispute", actions[0].Kind, "dispute wins over play from handlers.NextMatch")
}

// --- Leveled competition page ---

// makeLeveledCompTB creates a leveled competition. At minimum 3 pairs are
// needed since IsLeveled requires target < len(pairs)-1.
func makeLeveledCompTB(t testing.TB, app core.App, pairs []*core.Record) *core.Record {
	t.Helper()
	comp := handlers.MakeCompetitionTB(t, app, "league", pairs)
	comp.Set("target_matches", 1)
	comp.Set("open_assignments", 1)
	require.NoError(t, app.Save(comp))
	return comp
}

func TestLeveledCompetitionPage_GroupTitles(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page shows Jornada and Jugados groups",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"LvlPairA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "LvlPairA")
		p2 := handlers.MakePairTB(tb, app, "LvlPairB")
		p3 := handlers.MakePairTB(tb, app, "LvlPairC")
		// target=1 < len(pairs)-1=2 → IsLeveled=true
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})

		// One pending (slot 1) and one finalized match.
		mPending := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		mPending.Set("slot", 1)
		require.NoError(tb, app.Save(mPending))
		mFinal := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p3.Id, league.StatusFinal)
		mFinal.Set("finalized_at", "2026-09-10 12:00:00.000Z")
		mFinal.Set("result", "6-2 6-1")
		require.NoError(tb, app.Save(mFinal))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Jornada 1", "must show Jornada 1 group for a slot-1 pending match")
		assert.Contains(tb, body, "septiembre 2026", "must show played month group")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_TabLabel(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page tab label is Partidos not Jornadas",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"TabPairA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "TabPairA")
		p2 := handlers.MakePairTB(tb, app, "TabPairB")
		p3 := handlers.MakePairTB(tb, app, "TabPairC")
		// target=1 < 3-1=2 → IsLeveled=true
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Partidos", "tab label must be Partidos for leveled")
		assert.NotContains(tb, body, "aria-label=\"Jornadas\"", "must not have Jornadas tab for leveled")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_OwnPairDefault(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page defaults to own pair when no pair URL key",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"OwnPair"},
	}
	var myPairName, oppPairName string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "OwnPair")
		p2 := handlers.MakePairTB(tb, app, "OppPair")
		p3 := handlers.MakePairTB(tb, app, "AlienPair")
		myPairName = "OwnPair"
		oppPairName = "OppPair"
		// 3 pairs, target=1 < 3-1=2 → IsLeveled=true
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})

		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending") // my match
		handlers.MakeMatchTB(tb, app, comp.Id, p2.Id, p3.Id, "pending") // alien match — p3 not mine

		s.URL = "/competition/" + comp.Id // no ?pair= key
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		// The viewer's pair match should appear; the alien-only match should not.
		assert.Contains(tb, body, myPairName, "own pair match must appear")
		// AlienPair appears in the dropdown options but must NOT appear in a match card.
		// Match cards contain href="/match/" links — count them to verify only 1 match shown.
		assert.Equal(tb, 1, strings.Count(body, `href="/match/`), "only own-pair match card must appear")
		_ = oppPairName
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_PairAll(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page with ?pair=all shows all matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"AllPA"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "AllPA")
		p2 := handlers.MakePairTB(tb, app, "AllPB")
		p3 := handlers.MakePairTB(tb, app, "AllPC")
		// target=1 < 3-1=2 → IsLeveled=true
		comp := makeLeveledCompTB(tb, app, []*core.Record{p1, p2, p3})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		handlers.MakeMatchTB(tb, app, comp.Id, p2.Id, p3.Id, "pending") // p3 not in viewer's pair

		s.URL = "/competition/" + comp.Id + "?pair=all"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "AllPA", "AllPA match must appear")
		assert.Contains(tb, body, "AllPC", "AllPC match must appear with ?pair=all")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_InfoMessage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page shows info message when pair has pending < target",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"InfoMsg0"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		pairs := make([]*core.Record, 5)
		for i := range pairs {
			pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("InfoMsg%d", i))
		}
		p1 := pairs[0]
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		comp.Set("target_matches", 3) // 3 < 5-1=4 → IsLeveled=true
		comp.Set("open_assignments", 2)
		require.NoError(tb, app.Save(comp))

		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, pairs[1].Id, "pending")
		m.Set("slot", 1)
		require.NoError(tb, app.Save(m))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Tienes 1 partido por jugar. Cuando lo termines se te asignará el siguiente.", "must show pending-count message")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_InfoMessage_ZeroPending(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page shows zero-pending variant between matches",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ZeroPend0"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		pairs := make([]*core.Record, 5)
		for i := range pairs {
			pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("ZeroPend%d", i))
		}
		p1 := pairs[0]
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		comp.Set("target_matches", 3) // 3 < 5-1=4 → IsLeveled=true
		comp.Set("open_assignments", 2)
		require.NoError(tb, app.Save(comp))
		// No matches at all yet for this pair — total=0 < target=3, pending=0.

		s.URL = "/competition/" + comp.Id + "?pair=" + p1.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "No tienes partidos por jugar. Se te asignará el siguiente en breve.", "must show zero-pending message")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_InfoMessage_HiddenWhenComplete(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page hides info message when pair reached target",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Done0"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		pairs := make([]*core.Record, 5)
		for i := range pairs {
			pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("Done%d", i))
		}
		p1 := pairs[0]
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		comp.Set("target_matches", 1) // 1 < 5-1=4 → IsLeveled=true
		comp.Set("open_assignments", 1)
		require.NoError(tb, app.Save(comp))

		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, pairs[1].Id, league.StatusFinal)
		m.Set("finalized_at", "2026-09-10 12:00:00.000Z")
		m.Set("result", "6-2 6-1")
		require.NoError(tb, app.Save(m))

		s.URL = "/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "por jugar", "must not show info message when pair schedule is complete")
	}
	s.Test(t)
}

func TestLeveledCompetitionPage_InfoMessage_HiddenForAllPairsView(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "leveled competition page hides info message on ?pair=all",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"AllHide0"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		pairs := make([]*core.Record, 5)
		for i := range pairs {
			pairs[i] = handlers.MakePairTB(tb, app, fmt.Sprintf("AllHide%d", i))
		}
		p1 := pairs[0]
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		comp.Set("target_matches", 3) // 3 < 5-1=4 → IsLeveled=true
		comp.Set("open_assignments", 2)
		require.NoError(tb, app.Save(comp))

		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, pairs[1].Id, "pending")
		m.Set("slot", 1)
		require.NoError(tb, app.Save(m))

		s.URL = "/competition/" + comp.Id + "?pair=all"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.NotContains(tb, body, "por jugar", "must not show info message on all-pairs view")
	}
	s.Test(t)
}

// TestHome_RecentResultsIncludeUnconfirmedWithWarning verifies the home
// "Mis últimos partidos" list shows a result still waiting for the rival,
// newest first, with the shared warning on that row only.
func TestHome_RecentResultsIncludeUnconfirmedWithWarning(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "home recent results include an unconfirmed result with the warning",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Mis últimos partidos", "6-3 6-4", "6-1 6-2"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Unconf A")
		p2 := handlers.MakePairTB(tb, app, "Unconf B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		confirmed := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		confirmed.Set("scores", "6-1 6-2")
		confirmed.Set("winner", p1.Id)
		confirmed.Set("date", "2026-01-05")
		require.NoError(tb, app.Save(confirmed))

		proposed := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "scheduled")
		proposed.Set("date", "2026-01-20")
		require.NoError(tb, app.Save(proposed))
		createResultProposal(tb, app, proposed.Id, p1.GetString("player1"), "6-3 6-4")

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		start := strings.Index(body, "Mis últimos partidos")
		require.NotEqual(tb, -1, start)
		list := body[start:]
		assert.Equal(tb, 1, strings.Count(list, `data-testid="provisional-warning"`), "only the unconfirmed row warns")
		assert.Less(tb, strings.Index(list, "6-3 6-4"), strings.Index(list, "6-1 6-2"), "the later unconfirmed result sorts first")
		assert.Less(tb, strings.Index(list, "6-3 6-4"), strings.Index(list, `data-testid="provisional-warning"`), "the warning belongs to the unconfirmed row")
	}
	s.Test(t)
}

// TestHome_RecentResultsKeepsFiveNewest verifies the list is capped at the
// five most recent results, the oldest one dropping off.
func TestHome_RecentResultsKeepsFiveNewest(t *testing.T) {
	t.Parallel()
	scores := []string{"6-0 6-0", "6-1 6-0", "6-2 6-0", "6-3 6-0", "6-4 6-0", "7-5 6-0"}
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "home recent results keep the five newest",
		Method:             http.MethodGet,
		URL:                "/",
		ExpectedStatus:     200,
		ExpectedContent:    scores[1:],
		NotExpectedContent: []string{scores[0]},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Cap A")
		p2 := handlers.MakePairTB(tb, app, "Cap B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		for i, score := range scores {
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
			m.Set("scores", score)
			m.Set("winner", p1.Id)
			m.Set("date", fmt.Sprintf("2026-02-%02d", i+1))
			require.NoError(tb, app.Save(m))
		}
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}
