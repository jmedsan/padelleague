package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// raceMatches is how many matches each race scenario runs side by side.
const raceMatches = 40

// raceFixture is one match with both pairs' first players, seeded in a test app.
type raceFixture struct {
	match  *core.Record
	u1, u2 *core.Record
}

// raceScenario runs setup, fires the racers it returns together, then runs check.
// The scenario is a bare health request: the races happen in BeforeTestFunc.
func raceScenario(t *testing.T, name string,
	setup func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer,
	check func(tb testing.TB, app *tests.TestApp, fx []raceFixture),
) {
	t.Helper()
	raceScenarioResponses(t, name, setup,
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture, _ []*httptest.ResponseRecorder) {
			check(tb, app, fx)
		})
}

// raceScenarioResponses is raceScenario whose check also gets each racer's
// response, in the order setup returned the racers.
func raceScenarioResponses(t *testing.T, name string,
	setup func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer,
	check func(tb testing.TB, app *tests.TestApp, fx []raceFixture, rs []*httptest.ResponseRecorder),
) {
	t.Helper()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            name,
		Method:          http.MethodGet,
		URL:             "/api/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"API is healthy"},
	}
	var fixtures []raceFixture
	var responses []*httptest.ResponseRecorder
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		p1 := handlers.MakePairTB(tb, app, "Race A")
		p2 := handlers.MakePairTB(tb, app, "Race B")
		comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		u1, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		u2, err := app.FindRecordById("users", p2.GetString("player1"))
		require.NoError(tb, err)
		for range raceMatches {
			m := handlers.MakeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
			m.Set("date", "2026-09-01")
			m.Set("club", "Padel 360")
			require.NoError(tb, app.Save(m))
			fixtures = append(fixtures, raceFixture{m, u1, u2})
		}
		racers := setup(tb, app, fixtures)
		mux, err := e.Router.BuildMux()
		require.NoError(tb, err)
		responses = fireCollect(tb, mux, racers)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		check(tb, app, fixtures, responses)
	}
	s.Test(t)
}

// schedulingProposal seeds a pending date proposal authored by author.
func schedulingProposal(tb testing.TB, app core.App, matchID, authorID, date string) *core.Record {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(tb, err)
	pd, err := json.Marshal(league.ProposalData{Date: date, Time: "18:00", VenueName: "Padel 360", VenueText: "Padel 360"})
	require.NoError(tb, err)
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("author", authorID)
	rec.Set("type", "scheduling_proposal")
	rec.Set("proposal_data", string(pd))
	rec.Set("proposal_status", "pending")
	require.NoError(tb, app.Save(rec))
	return rec
}

func respondPath(matchID, msgID string) string {
	return "/match/" + matchID + "/thread/proposal/" + msgID + "/respond"
}

func reload(tb testing.TB, app core.App, collection, id string) *core.Record {
	tb.Helper()
	rec, err := app.FindRecordById(collection, id)
	require.NoError(tb, err)
	return rec
}

// Accepting a result while the rival asks for arbitration must not let the
// accept's stale copy of the match wipe the arbitration request.
func TestResultAcceptVsArbitration_Concurrent_NeverLosesTheArbitration(t *testing.T) {
	t.Parallel()
	raceScenario(t, "accept result vs arbitration",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for _, f := range fx {
				prop := handlers.MakeResultProposal(tb, app, f.match.Id, f.u1.Id, "6-3 6-4")
				racers = append(racers,
					racer{f.u2, respondPath(f.match.Id, prop.Id), "action=accept"},
					racer{f.u2, "/match/" + f.match.Id + "/arbitration", "category=result&notes=No+jugamos+as%C3%AD"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for _, f := range fx {
				m := reload(tb, app, "matches", f.match.Id)
				entries, err := app.FindRecordsByFilter("match_messages",
					"match = {:mid} && content ~ 'solicitó arbitraje'", "", 0, 0, map[string]any{"mid": f.match.Id})
				require.NoError(tb, err)
				assert.Equal(tb, len(entries) > 0, m.GetString("arbitration") != "",
					"match %s (status %s): a recorded arbitration request must survive, and none may appear unrecorded", m.Id, m.GetString("status"))
			}
		})
}

// A result accept racing a date accept must keep the final result.
func TestResultAcceptVsScheduleAccept_Concurrent_KeepsTheFinalResult(t *testing.T) {
	t.Parallel()
	raceScenario(t, "accept result vs accept date",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for _, f := range fx {
				res := handlers.MakeResultProposal(tb, app, f.match.Id, f.u1.Id, "6-3 6-4")
				date := schedulingProposal(tb, app, f.match.Id, f.u1.Id, "2030-05-01")
				racers = append(racers,
					racer{f.u2, respondPath(f.match.Id, res.Id), "action=accept"},
					racer{f.u2, respondPath(f.match.Id, date.Id), "action=accept"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for _, f := range fx {
				m := reload(tb, app, "matches", f.match.Id)
				results, err := app.FindRecordsByFilter("match_messages",
					"match = {:mid} && type = 'result_submission' && proposal_status = 'accepted'", "", 0, 0, map[string]any{"mid": f.match.Id})
				require.NoError(tb, err)
				if len(results) > 0 {
					assert.Equal(tb, "final", m.GetString("status"), "match %s: an accepted result must stay final", m.Id)
					assert.Equal(tb, "6-3 6-4", m.GetString("scores"), "match %s: the accepted score must survive", m.Id)
				}
			}
		})
}

// Two pending date proposals accepted at once: only one may win.
func TestScheduleAcceptVsAccept_Concurrent_OnlyOneWins(t *testing.T) {
	t.Parallel()
	raceScenario(t, "accept two date proposals",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for _, f := range fx {
				a := schedulingProposal(tb, app, f.match.Id, f.u1.Id, "2030-05-01")
				b := schedulingProposal(tb, app, f.match.Id, f.u1.Id, "2030-05-02")
				racers = append(racers,
					racer{f.u2, respondPath(f.match.Id, a.Id), "action=accept"},
					racer{f.u2, respondPath(f.match.Id, b.Id), "action=accept"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for _, f := range fx {
				accepted, err := app.FindRecordsByFilter("match_messages",
					"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'", "", 0, 0, map[string]any{"mid": f.match.Id})
				require.NoError(tb, err)
				assert.Len(tb, accepted, 1, "match %s: exactly one date proposal may be accepted", f.match.Id)
			}
		})
}

// Accepting a date proposal while the rival rejects it (plain or with a
// counter) must leave the match and the proposal telling the same story.
func TestScheduleAcceptVsReject_Concurrent_StaysConsistent(t *testing.T) {
	t.Parallel()
	const counterForm = "date=2030-06-01&time=19:00&venue_text=Wurko&rejection_reason=Otro"
	var proposals []string
	raceScenario(t, "accept vs reject a date proposal",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for i, f := range fx {
				p := schedulingProposal(tb, app, f.match.Id, f.u1.Id, "2030-05-01")
				proposals = append(proposals, p.Id)
				reject := racer{f.u2, respondPath(f.match.Id, p.Id), "action=reject&rejection_reason=Otro"}
				if i%2 == 1 {
					reject = racer{f.u2, "/match/" + f.match.Id + "/thread/proposal/" + p.Id + "/reject-and-counter", counterForm}
				}
				racers = append(racers, racer{f.u2, respondPath(f.match.Id, p.Id), "action=accept"}, reject)
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for i, f := range fx {
				m := reload(tb, app, "matches", f.match.Id)
				p := reload(tb, app, "match_messages", proposals[i])
				assert.Equal(tb, p.GetString("proposal_status") == "accepted", m.GetString("status") == "scheduled",
					"match %s: scheduled exactly when the proposal ended accepted (proposal %s, match %s)",
					m.Id, p.GetString("proposal_status"), m.GetString("status"))
			}
		})
}
