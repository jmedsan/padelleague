package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// succeeded counts the responses that redirected with 204 (the handlers'
// success reply; an alert page is a 200, even when it also carries a redirect).
func succeeded(rs []*httptest.ResponseRecorder) int {
	n := 0
	for _, r := range rs {
		if r.Code == http.StatusNoContent && r.Header().Get("HX-Redirect") != "" {
			n++
		}
	}
	return n
}

// onePerMatch asserts that, of the two racers per match (consecutive in rs),
// exactly one succeeded.
func onePerMatch(tb testing.TB, rs []*httptest.ResponseRecorder, what string) {
	tb.Helper()
	require.Len(tb, rs, 2*raceMatches)
	for i := 0; i < len(rs); i += 2 {
		assert.Equal(tb, 1, succeeded(rs[i:i+2]), "match %d: exactly one admin may %s", i/2, what)
	}
}

// Two admins approving the same walkover must apply the penalty once.
func TestWalkoverApprove_TwoAdmins_PenaltyOnce(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "two admins approve one walkover",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			comp, err := app.FindRecordById("competitions", fx[0].match.GetString("competition"))
			require.NoError(tb, err)
			comp.Set("default_penalty", 3)
			require.NoError(tb, app.Save(comp))
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for _, f := range fx {
				f.match.Set("review_type", "walkover")
				require.NoError(tb, app.Save(f.match))
				path := "/admin/disputes/" + f.match.Id + "/walkover-approve"
				form := "winner=" + f.match.GetString("pair1")
				racers = append(racers, racer{a1, path, form}, racer{a2, path, form})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "approve the walkover")
			pens, err := app.FindRecordsByFilter("penalties", "competition = {:c}", "", 0, 0,
				map[string]any{"c": fx[0].match.GetString("competition")})
			require.NoError(tb, err)
			assert.Len(tb, pens, raceMatches, "one penalty per walkover, never two")
		})
}

// Two admins resolving the same dispute: one resolution, one timeline entry.
func TestDisputesResolve_TwoAdmins_ResolvedOnce(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "two admins resolve one dispute",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for _, f := range fx {
				f.match.Set("status", "disputed")
				require.NoError(tb, app.Save(f.match))
				path := "/admin/disputes/" + f.match.Id + "/resolve"
				racers = append(racers, racer{a1, path, "score=6-3+6-4"}, racer{a2, path, "score=3-6+4-6"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "resolve the dispute")
			for _, f := range fx {
				entries, err := app.FindRecordsByFilter("match_messages",
					"match = {:m} && type = 'result_event'", "", 0, 0, map[string]any{"m": f.match.Id})
				require.NoError(tb, err)
				assert.Len(tb, entries, 1, "one resolution entry in the timeline")
			}
		})
}

// An admin releasing a leveled match while a player accepts its result: the
// release must not delete a match that just became final.
func TestAdminReleaseVsAccept_Concurrent_NeverDeletesAFinalMatch(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "release races the accept of a result",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			comp, err := app.FindRecordById("competitions", fx[0].match.GetString("competition"))
			require.NoError(tb, err)
			pairs := comp.GetStringSlice("pairs")
			for _, n := range []string{"Race C", "Race D"} {
				pairs = append(pairs, handlers.MakePairTB(tb, app, n).Id)
			}
			comp.Set("pairs", pairs)
			comp.Set("target_matches", 1)
			require.NoError(tb, app.Save(comp))
			require.True(tb, league.IsLeveled(comp))
			admin := makeAdminUser(tb, app)
			var racers []racer
			for _, f := range fx {
				prop := handlers.MakeResultProposal(tb, app, f.match.Id, f.u1.Id, "6-3 6-4")
				racers = append(racers,
					racer{admin, "/match/" + f.match.Id + "/release", ""},
					racer{f.u2, "/match/" + f.match.Id + "/thread/proposal/" + prop.Id + "/respond", "action=accept"})
			}
			return racers
		},
		func(tb testing.TB, _ *tests.TestApp, _ []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "win: release or accept")
		})
}

// Two admins voiding the same penalty: the second must be told it is already void.
func TestVoidPenalty_TwoAdmins_VoidedOnce(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "two admins void one penalty",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			compID := fx[0].match.GetString("competition")
			path := "/admin/competitions/" + compID + "/penalty"
			var racers []racer
			for _, f := range fx {
				pen, err := league.ApplyPenalty(app, league.PenaltyInput{
					CompetitionID: compID, PairID: f.match.GetString("pair1"), Reason: "r", AdminID: a1.Id, Amount: 1})
				require.NoError(tb, err)
				form := "action=remove&penalty_id=" + pen.Id + "&void_reason=x"
				racers = append(racers, racer{a1, path, form}, racer{a2, path, form})
			}
			return racers
		},
		func(tb testing.TB, _ *tests.TestApp, _ []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "void the penalty")
		})
}

// Each admin move is valid against the dates the other has not saved yet; the
// bracket order must hold once both have run.
func TestAdminOverride_PlayoffDates_Concurrent_KeepBracketOrder(t *testing.T) {
	t.Parallel()
	raceScenario(t, "two admins move playoff dates of consecutive rounds",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			comp, err := app.FindRecordById("competitions", fx[0].match.GetString("competition"))
			require.NoError(tb, err)
			comp.Set("type", "playoff")
			require.NoError(tb, app.Save(comp))
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for i := 0; i+1 < len(fx); i += 2 {
				early, late := fx[i].match, fx[i+1].match
				early.Set("round_number", 1)
				early.Set("date", "2026-10-10")
				late.Set("round_number", 2)
				late.Set("date", "2026-10-20")
				require.NoError(tb, app.Save(early))
				require.NoError(tb, app.Save(late))
				racers = append(racers,
					racer{a1, "/match/" + early.Id + "/admin-override", "date=2026-10-18"},
					racer{a2, "/match/" + late.Id + "/admin-override", "date=2026-10-16"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for i := 0; i+1 < len(fx); i += 2 {
				early, err := app.FindRecordById("matches", fx[i].match.Id)
				require.NoError(tb, err)
				late, err := app.FindRecordById("matches", fx[i+1].match.Id)
				require.NoError(tb, err)
				assert.False(tb, early.GetDateTime("date").Time().After(late.GetDateTime("date").Time()),
					"pair %d: round 1 (%s) must not be after round 2 (%s)", i/2, early.GetString("date"), late.GetString("date"))
			}
		})
}
