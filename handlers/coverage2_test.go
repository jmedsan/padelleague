package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterWithValidToken(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /register?token=valid shows form with email",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Crear cuenta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		s.URL = "/register?token=" + invite.GetString("token")
	}
	s.Test(t)
}

func TestRegisterWithExpiredToken(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /register?token=expired shows invalid",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Invitacion no valida"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(-1*time.Hour))
		s.URL = "/register?token=" + invite.GetString("token")
	}
	s.Test(t)
}

func TestPlayerProfileWithMatches(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /player/{id} with match history shows stats",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Stats A"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Stats A")
		p2 := makePairTB(tb, app, "Stats B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Create several final matches with winners for streak calculations
		for i := 0; i < 3; i++ {
			col, _ := app.FindCollectionByNameOrId("matches")
			m := core.NewRecord(col)
			m.Set("competition", comp.Id)
			m.Set("pair1", p1.Id)
			m.Set("pair2", p2.Id)
			m.Set("status", "final")
			m.Set("scores", "6-3 6-4")
			m.Set("winner", p1.Id)
			m.Set("round_number", i+1)
			require.NoError(tb, app.Save(m))
		}
		// One loss
		col, _ := app.FindCollectionByNameOrId("matches")
		m := core.NewRecord(col)
		m.Set("competition", comp.Id)
		m.Set("pair1", p1.Id)
		m.Set("pair2", p2.Id)
		m.Set("status", "final")
		m.Set("scores", "3-6 4-6")
		m.Set("winner", p2.Id)
		m.Set("round_number", 4)
		require.NoError(tb, app.Save(m))

		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.URL = "/player/" + user.Id
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadWithMessages(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /match/{id}/thread with messages renders them",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"thread-messages"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ThrMsg A")
		p2 := makePairTB(tb, app, "ThrMsg B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Add chat messages
		col, _ := app.FindCollectionByNameOrId("match_messages")
		for i, content := range []string{"Hola equipo", "Cuando jugamos?"} {
			msg := core.NewRecord(col)
			msg.Set("match", match.Id)
			msg.Set("author", p1.GetString("player1"))
			msg.Set("type", "chat")
			msg.Set("content", content)
			require.NoError(tb, app.Save(msg))
			_ = i
		}

		// Add a scheduling proposal
		proposal := core.NewRecord(col)
		proposal.Set("match", match.Id)
		proposal.Set("author", p1.GetString("player1"))
		proposal.Set("type", "scheduling_proposal")
		proposal.Set("proposal_data", map[string]any{
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club Padel",
		})
		proposal.Set("proposal_status", "pending")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestThreadMessagesWithData(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /match/{id}/thread-messages with messages",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Hola equipo"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ThrData A")
		p2 := makePairTB(tb, app, "ThrData B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "chat")
		msg.Set("content", "Hola equipo")
		require.NoError(tb, app.Save(msg))

		// Add a proposal too
		proposal := core.NewRecord(col)
		proposal.Set("match", match.Id)
		proposal.Set("author", p2.GetString("player1"))
		proposal.Set("type", "scheduling_proposal")
		proposal.Set("proposal_data", map[string]any{
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club",
		})
		proposal.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(proposal))

		s.URL = "/match/" + match.Id + "/thread-messages"
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.Test(t)
}

func TestProposalChangeDecision(t *testing.T) {
	s := &tests.ApiScenario{
		Name:           "POST change-decision reverts accepted to rejected",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var msgID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "ChgDec A")
		p2 := makePairTB(tb, app, "ChgDec B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		col, _ := app.FindCollectionByNameOrId("match_messages")
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", p1.GetString("player1"))
		msg.Set("type", "scheduling_proposal")
		msg.Set("proposal_data", map[string]any{
			"date": "2026-09-20", "time": "19:00", "venue_name": "Club",
		})
		msg.Set("proposal_status", "accepted")
		require.NoError(tb, app.Save(msg))
		msgID = msg.Id

		s.URL = fmt.Sprintf("/match/%s/thread/proposal/%s/change-decision", match.Id, msg.Id)
		opponent, _ := app.FindRecordById("users", p2.GetString("player1"))
		s.Headers = authHeaders(tb, opponent)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		m, err := app.FindRecordById("match_messages", msgID)
		require.NoError(tb, err)
		assert.Equal(tb, "rejected", m.GetString("proposal_status"))
	}
	s.Test(t)
}

func TestHomeWithScheduledMatch(t *testing.T) {
	s := &tests.ApiScenario{
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

func TestRegisterSubmitPasswordMismatch(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "POST /register with mismatched passwords",
		Method:          http.MethodPost,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"no coinciden"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		invite := makeInvitationTB(tb, app, admin.Id, time.Now().Add(24*time.Hour))
		body := fmt.Sprintf("token=%s&email=reg@test.local&display_name=Test&password=abc123456&password_confirm=xyz123456", invite.GetString("token"))
		s.Body = strings.NewReader(body)
		s.Headers = map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	}
	s.Test(t)
}

func TestAdminCompetitionDetailWithData(t *testing.T) {
	s := &tests.ApiScenario{
		Name:            "GET /admin/competitions/{id} with matches and disputes",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Detail A", "Detail B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		admin := makeAdminUserTB(tb, app)
		p1 := makePairTB(tb, app, "Detail A")
		p2 := makePairTB(tb, app, "Detail B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Pending match
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")

		// Confirmed match (stale)
		confirmed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "confirmed")
		confirmed.Set("scores", "6-3 6-4")
		confirmed.Set("submitted_by", p1.GetString("player1"))
		require.NoError(tb, app.Save(confirmed))

		// Disputed match
		disputed := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		disputed.Set("scores", "6-3 6-4")
		disputed.Set("dispute_notes", "Score wrong")
		require.NoError(tb, app.Save(disputed))

		// Final match
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

		s.URL = "/admin/competitions/" + comp.Id
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}
