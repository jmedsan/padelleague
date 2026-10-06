package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

const proposalForm = "date=2030-06-01&time=19:00&venue_text=Wurko"

// Withdrawing a date proposal while the rival accepts it must not turn an
// accepted proposal into a withdrawn one under a match that stays scheduled.
func TestWithdrawVsAcceptProposal_Concurrent_StaysConsistent(t *testing.T) {
	t.Parallel()
	var proposals []string
	raceScenario(t, "withdraw vs accept a date proposal",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for _, f := range fx {
				p := schedulingProposal(tb, app, f.match.Id, f.u1.Id, "2030-05-01")
				proposals = append(proposals, p.Id)
				racers = append(racers,
					racer{f.u1, "/match/" + f.match.Id + "/thread/proposal/" + p.Id + "/withdraw", ""},
					racer{f.u2, respondPath(f.match.Id, p.Id), "action=accept"})
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

// A date proposal posted while an admin finalizes the match must not outlive
// the decision: a final match holds no pending date proposal.
func TestPostProposalVsAdminResult_Concurrent_NoPendingProposalOnAFinalMatch(t *testing.T) {
	t.Parallel()
	raceScenario(t, "post a date proposal while the admin sets the result",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			admin := makeAdminUser(tb, app)
			var racers []racer
			for _, f := range fx {
				racers = append(racers,
					racer{f.u1, "/match/" + f.match.Id + "/thread/proposal", proposalForm},
					racer{admin, "/match/" + f.match.Id + "/admin-override", "scores=6-3+6-4"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			for _, f := range fx {
				require.Equal(tb, "final", reload(tb, app, "matches", f.match.Id).GetString("status"))
				pending, err := app.FindRecordsByFilter("match_messages",
					"match = {:m} && type = 'scheduling_proposal' && proposal_status = 'pending'",
					"", 0, 0, map[string]any{"m": f.match.Id})
				require.NoError(tb, err)
				assert.Empty(tb, pending, "match %s: no pending date proposal on a final match", f.match.Id)
			}
		})
}

// Both pairs cancelling the same confirmed date: one cancellation, one entry.
func TestCancelDate_BothPairs_CancelledOnce(t *testing.T) {
	t.Parallel()
	raceScenarioResponses(t, "both pairs cancel one confirmed date",
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			for _, f := range fx {
				f.match.Set("status", "scheduled")
				f.match.Set("time", "18:00")
				require.NoError(tb, app.Save(f.match))
				path := "/match/" + f.match.Id + "/cancel-date"
				racers = append(racers, racer{f.u1, path, "reason=lluvia"}, racer{f.u2, path, "reason=lesion"})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "cancel the date")
			for _, f := range fx {
				entries, err := app.FindRecordsByFilter("match_messages",
					"match = {:m} && type = 'scheduling_response' && content ~ 'canceló la fecha'",
					"", 0, 0, map[string]any{"m": f.match.Id})
				require.NoError(tb, err)
				assert.Len(tb, entries, 1, "match %s: one cancellation entry", f.match.Id)
			}
		})
}

// Several clicks on "acknowledge documents" leave one ack row per player.
func TestAcceptDocs_Concurrent_OneAckPerPlayer(t *testing.T) {
	t.Parallel()
	const clicks = 8
	raceScenario(t, "acknowledge the documents in parallel",
		func(_ testing.TB, _ *tests.TestApp, fx []raceFixture) []racer {
			var racers []racer
			path := "/competition/" + fx[0].match.GetString("competition") + "/accept-docs"
			for range clicks {
				racers = append(racers, racer{fx[0].u1, path, ""})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, fx []raceFixture) {
			acks, err := app.FindRecordsByFilter("document_acks", "user = {:u} && competition = {:c}", "", 0, 0,
				map[string]any{"u": fx[0].u1.Id, "c": fx[0].match.GetString("competition")})
			require.NoError(tb, err)
			assert.Len(tb, acks, 1)
		})
}

// An admin revoking an invitation while a registration consumes it must not
// delete an invitation that was just used.
func TestInvitationsRevoke_VsUse_NeverDeletesAUsedInvitation(t *testing.T) {
	t.Parallel()
	const invites = 40
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "revoke races the use of an invitation",
		Method:          http.MethodGet,
		URL:             "/api/health",
		ExpectedStatus:  200,
		ExpectedContent: []string{"API is healthy"},
	}
	var ids []string
	used := map[string]bool{}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		var racers []racer
		for range invites {
			inv := handlers.MakeInvitationTB(tb, app, admin.Id, time.Now().Add(time.Hour))
			ids = append(ids, inv.Id)
			racers = append(racers, racer{admin, "/admin/invitations/" + inv.Id + "/revoke", ""})
		}
		mux, err := e.Router.BuildMux()
		require.NoError(tb, err)

		// The "registration": mark each invitation used, in its own transaction.
		var mu sync.Mutex
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, id := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				err := app.RunInTransaction(func(txApp core.App) error {
					inv, err := txApp.FindRecordById("invitations", id)
					if err != nil {
						return err
					}
					inv.Set("status", "used")
					return txApp.Save(inv)
				})
				mu.Lock()
				used[id] = err == nil
				mu.Unlock()
			}()
		}
		close(start)
		fireTogether(tb, mux, racers)
		wg.Wait()
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, id := range ids {
			_, err := app.FindRecordById("invitations", id)
			if used[id] {
				assert.NoError(tb, err, "invitation %s was used, so it must still exist", id)
			}
		}
	}
	s.Test(t)
}
