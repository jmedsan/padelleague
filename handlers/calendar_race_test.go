package handlers_test

import (
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

// calendarComps seeds raceMatches league competitions over four pairs. With
// withDraft each already holds a draft calendar: one pending match.
func calendarComps(tb testing.TB, app *tests.TestApp, withDraft bool) []*core.Record {
	tb.Helper()
	pairs := []*core.Record{
		handlers.MakePairTB(tb, app, "Cal A"), handlers.MakePairTB(tb, app, "Cal B"),
		handlers.MakePairTB(tb, app, "Cal C"), handlers.MakePairTB(tb, app, "Cal D"),
	}
	var comps []*core.Record
	for range raceMatches {
		comp := handlers.MakeCompetitionTB(tb, app, "league", pairs)
		if withDraft {
			handlers.MakeMatchTB(tb, app, comp.Id, pairs[0].Id, pairs[1].Id, "pending")
			comp.Set("calendar_status", "draft")
			require.NoError(tb, app.Save(comp))
		}
		comps = append(comps, comp)
	}
	return comps
}

func compMatchCount(tb testing.TB, app core.App, compID string) int {
	tb.Helper()
	ms, err := app.FindRecordsByFilter("matches", "competition = {:c}", "", 0, 0, map[string]any{"c": compID})
	require.NoError(tb, err)
	return len(ms)
}

// Two admins publishing the same draft: one publication, one notification each.
func TestPublishCalendar_TwoAdmins_PublishedOnce(t *testing.T) {
	t.Parallel()
	var comps []*core.Record
	raceScenarioResponses(t, "two admins publish one calendar",
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture) []racer {
			comps = calendarComps(tb, app, true)
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for _, c := range comps {
				path := "/admin/competitions/" + c.Id + "/publish"
				racers = append(racers, racer{a1, path, ""}, racer{a2, path, ""})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "publish the calendar")
			for _, c := range comps {
				fresh, err := app.FindRecordById("competitions", c.Id)
				require.NoError(tb, err)
				assert.Equal(tb, "published", fresh.GetString("calendar_status"))
			}
			players := map[string]struct{}{}
			for _, pid := range comps[0].GetStringSlice("pairs") {
				for _, uid := range league.PlayersForPair(app, pid) {
					players[uid] = struct{}{}
				}
			}
			all, err := app.FindRecordsByFilter("notifications", "type = 'calendar_published'", "", 0, 0)
			require.NoError(tb, err)
			assert.Len(tb, all, raceMatches*len(players), "every player is notified once per published calendar")
		})
}

// Publishing a draft while another admin deletes it must never leave a
// published calendar with no matches, and exactly one of the two may win.
func TestPublishVsDeleteCalendar_NeverPublishesAnEmptyCalendar(t *testing.T) {
	t.Parallel()
	var comps []*core.Record
	raceScenarioResponses(t, "publish races delete on a draft calendar",
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture) []racer {
			comps = calendarComps(tb, app, true)
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for _, c := range comps {
				base := "/admin/competitions/" + c.Id
				racers = append(racers, racer{a1, base + "/publish", ""}, racer{a2, base + "/delete-calendar", ""})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "publish or delete the draft")
			for _, c := range comps {
				fresh, err := app.FindRecordById("competitions", c.Id)
				require.NoError(tb, err)
				switch fresh.GetString("calendar_status") {
				case "published":
					assert.Equal(tb, 1, compMatchCount(tb, app, c.Id), "a published calendar keeps its matches")
				case "none":
					assert.Zero(tb, compMatchCount(tb, app, c.Id), "a deleted calendar has no matches")
				default:
					tb.Errorf("competition %s ended as %q", c.Id, fresh.GetString("calendar_status"))
				}
			}
		})
}

// Two admins generating the same competition's calendar at once must produce
// one calendar, not two stacked on top of each other.
func TestGenerateFixtures_TwoAdmins_OneCalendar(t *testing.T) {
	t.Parallel()
	var comps []*core.Record
	raceScenarioResponses(t, "two admins generate one calendar",
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture) []racer {
			comps = calendarComps(tb, app, false)
			a1, a2 := makeAdminUser(tb, app), makeAdminUser(tb, app)
			var racers []racer
			for _, c := range comps {
				path := "/admin/competitions/" + c.Id + "/generate"
				racers = append(racers, racer{a1, path, ""}, racer{a2, path, ""})
			}
			return racers
		},
		func(tb testing.TB, app *tests.TestApp, _ []raceFixture, rs []*httptest.ResponseRecorder) {
			onePerMatch(tb, rs, "generate the calendar")
			for _, c := range comps {
				assert.Equal(tb, 6, compMatchCount(tb, app, c.Id), "4 pairs, single round robin: 6 matches")
			}
		})
}
