package handlers_test

import (
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

// Dispute resolve auto-determines winner from score (pair2 wins)

func TestDisputeResolveAutoWinnerPair2(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve auto-determines pair2 as winner",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DispP2A")
		p2 := handlers.MakePairTB(tb, app, "DispP2B")
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=3-6+4-6")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "3-6 4-6", m.GetString("scores"))
		assert.Equal(tb, p2ID, m.GetString("winner"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

// Dispute resolve with invalid score → rejected

func TestDisputeResolveInvalidScore(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/disputes/{id}/resolve rejects invalid score",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Marcador no válido"},
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DispBadScA")
		p2 := handlers.MakePairTB(tb, app, "DispBadScB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		s.Body = strings.NewReader("score=99-99")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"), "status must remain disputed")
		assert.Empty(tb, m.GetString("scores"), "scores must not be set")
	}
	s.Test(t)
}

// Dispute resolve without manual winner (auto-determine from score)
// Covers the else branch at line 71-77 where DetermineWinner is called.

func TestDisputeResolveAutoWinner(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/disputes/{id}/resolve auto-determines winner from score",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DispAutoA")
		p2 := handlers.MakePairTB(tb, app, "DispAutoB")
		p1ID = p1.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/resolve"
		// No winner param — score determines winner (pair1 wins 6-3 6-4)
		s.Body = strings.NewReader("score=6-3+6-4")
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, p1ID, m.GetString("winner"))
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

// Request arbitration (replaces the old walkover-only "report unplayed" flow)

func TestRequestArbitrationNoShow(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/arbitration category=no_show sets walkover review",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, userID, rivalUserID, partnerID, adminID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := handlers.MakeAdminUserTB(tb, app)
		adminID = admin.Id
		p1 := handlers.MakePairTB(tb, app, "RptA")
		p2 := handlers.MakePairTB(tb, app, "RptB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-01")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		userID = user.Id
		partnerID = p1.GetString("player2")
		rivalUserID = p2.GetString("player1")
		s.URL = "/match/" + match.Id + "/arbitration"
		s.Body = strings.NewReader("category=no_show&notes=rival+no+se+presento")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "disputed", m.GetString("status"))
		assert.Equal(tb, "walkover", m.GetString("review_type"))
		assert.Equal(tb, "no_show", m.GetString("arbitration"))
		assert.Equal(tb, userID, m.GetString("arbitration_by"))
		assert.Equal(tb, userID, m.GetString("walkover_requested_by"))
		assert.Contains(tb, m.GetString("dispute_notes"), "no se presento")
		assert.Empty(tb, m.GetString("winner"), "requesting arbitration must not declare a winner")
		assert.Empty(tb, m.GetString("scores"), "requesting arbitration must not set a score")
		assertNotNotified(tb, app, rivalUserID, "Disputa resuelta")
		assertNotNotified(tb, app, partnerID, "Disputa resuelta")

		adminWant := league.Notification{
			Type:     "dispute",
			Title:    "Arbitraje solicitado",
			Body:     "Un jugador ha solicitado arbitraje: Incomparecencia",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		assertNotified(tb, app, adminID, adminWant)
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestRequestArbitration_Idempotent(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/arbitration is a no-op once one is already open",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "IdpA")
		p2 := handlers.MakePairTB(tb, app, "IdpB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("review_type", "walkover")
		match.Set("status", "disputed")
		match.Set("arbitration", "no_show")
		require.NoError(tb, app.Save(match))
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/match/" + match.Id + "/arbitration"
		s.Body = strings.NewReader("category=no_show&notes=de+nuevo")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestRequestArbitrationWrongStatus_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/arbitration on final fails",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya está resuelto"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "RptStatusA")
		p2 := handlers.MakePairTB(tb, app, "RptStatusB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")

		s.URL = "/match/" + m.Id + "/arbitration"
		s.Body = strings.NewReader("category=other&notes=test")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestRequestArbitrationNonParticipant_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "non-participant cannot request arbitration",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No eres participante"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		outsider := handlers.MakeUserTB(tb, app, "Outsider Rpt", "")
		p1 := handlers.MakePairTB(tb, app, "ORptA")
		p2 := handlers.MakePairTB(tb, app, "ORptB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/arbitration"
		s.Body = strings.NewReader("category=other&notes=test")
		hdrs := handlers.AuthHeaders(tb, outsider)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestRequestArbitrationMissingCategory_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "arbitration request without a valid category is refused",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Selecciona un motivo válido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoCatA")
		p2 := handlers.MakePairTB(tb, app, "NoCatB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/match/" + match.Id + "/arbitration"
		s.Body = strings.NewReader("category=bogus&notes=test")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestRequestArbitrationMissingNotes_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "arbitration request without notes is refused",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Explica el motivo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NoNotesA")
		p2 := handlers.MakePairTB(tb, app, "NoNotesB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/match/" + match.Id + "/arbitration"
		s.Body = strings.NewReader("category=other&notes=")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestCloseArbitration(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /match/{id}/arbitration/close clears the open request",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID string
	var players []string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "CloseArbA")
		p2 := handlers.MakePairTB(tb, app, "CloseArbB")
		players = []string{p1.GetString("player1"), p1.GetString("player2"), p2.GetString("player1"), p2.GetString("player2")}
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("arbitration", "scheduling")
		match.Set("arbitration_by", p1.GetString("player1"))
		match.Set("dispute_notes", "no acordamos fecha")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/match/" + match.Id + "/arbitration/close"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Empty(tb, m.GetString("arbitration"))
		assert.Empty(tb, m.GetString("arbitration_by"))
		assert.Equal(tb, "pending", m.GetString("status"), "closing must not otherwise change match status")

		want := league.Notification{
			Type:     "dispute",
			Title:    "Arbitraje cerrado",
			Body:     "El administrador ha cerrado la solicitud de arbitraje: Fecha y hora",
			MatchID:  matchID,
			CompName: "Test Competition",
		}
		for _, uid := range players {
			assertNotified(tb, app, uid, want)
		}
	}
	handlers.ExpectRedirect(s, func(core.App) string { return matchPageURL(s.URL) })
	s.Test(t)
}

func TestCloseArbitration_NoneOpen_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /match/{id}/arbitration/close with none open is refused",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no tiene arbitraje abierto"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "NoArbA")
		p2 := handlers.MakePairTB(tb, app, "NoArbB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/match/" + match.Id + "/arbitration/close"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestCloseArbitrationNonAdmin_Refused(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "non-admin cannot close an arbitration request",
		Method:         http.MethodPost,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "NonAdmA")
		p2 := handlers.MakePairTB(tb, app, "NonAdmB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("arbitration", "scheduling")
		require.NoError(tb, app.Save(match))
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/match/" + match.Id + "/arbitration/close"
		s.Headers = handlers.AuthHeaders(tb, user)
	}
	s.Test(t)
}

// Walkover approve

func TestWalkoverApprove(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/disputes/{id}/walkover-approve finalizes match with penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p1ID, p2ID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "WoApA")
		p2 := handlers.MakePairTB(tb, app, "WoApB")
		p1ID = p1.Id
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))
		assert.Equal(tb, "6-0 6-0", m.GetString("scores"))
		assert.Equal(tb, p1ID, m.GetString("winner"))

		rows, err := app.FindRecordsByFilter("penalties",
			"competition = {:c} && pair = {:p} && voided = false", "", 0, 0,
			map[string]any{"c": compID, "p": p2ID})
		require.NoError(tb, err)
		require.Len(tb, rows, 1, "losing pair must have penalty applied")
		assert.Equal(tb, 5.0, rows[0].GetFloat("amount"))
		assert.Equal(tb, "Incomparecencia aprobada", rows[0].GetString("reason"))
		assert.NotEmpty(tb, rows[0].GetString("applied_by"), "walkover penalty must record approving admin")

		compRec, err := app.FindRecordById("competitions", compID)
		require.NoError(tb, err)
		want := league.Notification{
			Type:     "general",
			Title:    "Incomparecencia aprobada",
			Body:     "El administrador ha aprobado la incomparecencia",
			MatchID:  matchID,
			CompName: compRec.GetString("name"),
		}
		for _, uid := range league.PlayersForPair(app, p1ID) {
			assertNotified(tb, app, uid, want)
		}
		for _, uid := range league.PlayersForPair(app, p2ID) {
			assertNotified(tb, app, uid, want)
		}

		penaltyWant := league.Notification{
			Type:     "penalty",
			Title:    "Penalización aplicada",
			Body:     "5 puntos — Incomparecencia aprobada",
			Link:     "/competition/" + compID,
			CompName: "Test Competition",
		}
		for _, uid := range league.PlayersForPair(app, p2ID) {
			assertNotified(tb, app, uid, penaltyWant)
		}
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

func TestWalkoverApprove_ZeroPenalty_NoPenaltyApplied(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/disputes/{id}/walkover-approve with default_penalty=0 skips penalty",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var matchID, p2ID, compID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "WoZeroA")
		p2 := handlers.MakePairTB(tb, app, "WoZeroB")
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 0)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		compID = comp.Id
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"))

		rows, err := app.FindRecordsByFilter("penalties",
			"competition = {:c} && pair = {:p}", "", 0, 0,
			map[string]any{"c": compID, "p": p2ID})
		require.NoError(tb, err)
		assert.Empty(tb, rows, "no penalty entry must be recorded when default_penalty is 0")
	}
	handlers.ExpectRedirect(s, func(app core.App) string { return "/admin/competitions/" + matchCompetitionID(app, s.URL) })
	s.Test(t)
}

func TestWalkoverApprove_PenaltyFails_AlertsAdmin(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve surfaces penalty save failure",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no se pudo aplicar la penalización"},
	}
	var matchID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "WoFailA")
		p2 := handlers.MakePairTB(tb, app, "WoFailB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		require.NoError(tb, app.Save(comp))
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("review_type", "walkover")
		require.NoError(tb, app.Save(match))
		matchID = match.Id

		app.OnRecordCreate("penalties").BindFunc(func(_ *core.RecordEvent) error {
			return fmt.Errorf("simulated DB failure for penalty save")
		})

		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("matches", matchID)
		require.NoError(tb, err)
		assert.Equal(tb, "final", m.GetString("status"), "match must still finalize even if the penalty save fails")
	}
	s.Test(t)
}

func TestWalkoverApprove_AlreadyFinal_RejectedNoDoublePenalty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve on an already-final match is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"ya está resuelto"},
	}
	var compID, p2ID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "WoRepA")
		p2 := handlers.MakePairTB(tb, app, "WoRepB")
		p2ID = p2.Id
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("default_penalty", 5)
		comp.Set("walkover_score", "6-0 6-0")
		handlers.MakePenaltyTB(tb, app, comp.Id, p2.Id, 5, "Incomparecencia aprobada", admin.Id, false)
		compID = comp.Id
		// Simulates the state right after a first, successful approval.
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "final")
		match.Set("review_type", "walkover")
		match.Set("scores", "6-0 6-0")
		match.Set("winner", p1.Id)
		require.NoError(tb, app.Save(match))

		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		rows, err := app.FindRecordsByFilter("penalties",
			"competition = {:c} && pair = {:p} && voided = false", "", 0, 0,
			map[string]any{"c": compID, "p": p2ID})
		require.NoError(tb, err)
		require.Len(tb, rows, 1, "penalty must stay at the original amount, not doubled")
		assert.Equal(tb, 5.0, rows[0].GetFloat("amount"))
	}
	s.Test(t)
}

func TestWalkoverApprove_NotWalkover(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /admin/disputes/{id}/walkover-approve rejects non-walkover",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"no es una solicitud de incomparecencia"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "WoNonA")
		p2 := handlers.MakePairTB(tb, app, "WoNonB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.URL = "/admin/disputes/" + match.Id + "/walkover-approve"
		s.Body = strings.NewReader("winner=" + p1.Id)
		hdrs := handlers.AuthHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.Test(t)
}

func TestAdminOutstandingPage_NonEmpty(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/outstanding with a pending match",
		Method:          http.MethodGet,
		URL:             "/admin/outstanding",
		ExpectedStatus:  200,
		ExpectedContent: []string{"outstanding-list", "OutPageA", "OutPageB"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "OutPageA")
		p2 := handlers.MakePairTB(tb, app, "OutPageB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}

func TestAdminDisputesPage(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/disputes with disputed match",
		Method:          http.MethodGet,
		URL:             "/admin/disputes",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Disputas"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		p1 := handlers.MakePairTB(tb, app, "DispPageA")
		p2 := handlers.MakePairTB(tb, app, "DispPageB")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}
