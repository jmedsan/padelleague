package seed

import (
	"testing"

	"padelleague/hooks"
	"padelleague/league"
	"padelleague/notify"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "padelleague/migrations"
)

func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func TestRun_CreatesUsers(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "seed1@test.local", Password: "pass123456", Collection: "users", Roles: []string{"admin"}, DisplayName: "Seed Admin"},
		{Email: "seed2@test.local", Password: "pass123456", Collection: "users", Roles: []string{"player"}, DisplayName: "Seed Player"},
	})

	u1, err := app.FindAuthRecordByEmail("users", "seed1@test.local")
	require.NoError(t, err)
	assert.Contains(t, u1.GetStringSlice("roles"), "admin")
	assert.Equal(t, "Seed Admin", u1.GetString("display_name"))
	assert.True(t, u1.Verified())

	u2, err := app.FindAuthRecordByEmail("users", "seed2@test.local")
	require.NoError(t, err)
	assert.Contains(t, u2.GetStringSlice("roles"), "player")
}

func TestRun_SkipsEmptyEmailOrPassword(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "", Password: "pass123456", Collection: "users", Roles: []string{"player"}},
		{Email: "nopass@test.local", Password: "", Collection: "users", Roles: []string{"player"}},
	})

	_, err := app.FindAuthRecordByEmail("users", "nopass@test.local")
	assert.Error(t, err, "user with empty password should not be created")
}

func TestRun_SkipsExistingUser(t *testing.T) {
	app := newTestApp(t)

	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	existing := core.NewRecord(col)
	existing.Set("email", "existing@test.local")
	existing.Set("display_name", "Original")
	existing.Set("roles", []string{"player"})
	existing.SetPassword("original123456")
	existing.SetVerified(true)
	require.NoError(t, app.Save(existing))

	Run(app, []User{
		{Email: "existing@test.local", Password: "newpass123456", Collection: "users", Roles: []string{"admin"}, DisplayName: "Changed"},
	})

	u, err := app.FindAuthRecordByEmail("users", "existing@test.local")
	require.NoError(t, err)
	assert.Contains(t, u.GetStringSlice("roles"), "player", "existing user should not be overwritten")
	assert.Equal(t, "Original", u.GetString("display_name"), "existing display_name should not change")
}

func TestRun_InvalidCollection(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "bad@test.local", Password: "pass123456", Collection: "nonexistent", Roles: []string{"player"}},
	})

	// Should not panic; user should not exist anywhere.
	_, err := app.FindAuthRecordByEmail("users", "bad@test.local")
	assert.Error(t, err)
}

func TestRun_MissingRequiredFields_LogsAndContinues(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "norole@test.local", Password: "pass123456", Collection: "users"},
		{Email: "good@test.local", Password: "pass123456", Collection: "users", Roles: []string{"player"}, DisplayName: "Good"},
	})

	_, err := app.FindAuthRecordByEmail("users", "norole@test.local")
	assert.Error(t, err, "user with missing required fields should not be created")

	u, err := app.FindAuthRecordByEmail("users", "good@test.local")
	require.NoError(t, err)
	assert.Contains(t, u.GetStringSlice("roles"), "player", "subsequent valid user should still be created")
}

func seedUser(t *testing.T, app core.App, email, role, displayName string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("email", email)
	rec.Set("roles", []string{role})
	rec.Set("display_name", displayName)
	rec.SetPassword("testpass123456")
	rec.SetVerified(true)
	require.NoError(t, app.Save(rec))
	return rec
}

func seedPair(t *testing.T, app core.App, name string, p1, p2 *core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("name", name)
	rec.Set("player1", p1.Id)
	rec.Set("player2", p2.Id)
	require.NoError(t, app.Save(rec))
	return rec
}

func seedMatch(t *testing.T, app core.App, compID, p1ID, p2ID string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("competition", compID)
	rec.Set("pair1", p1ID)
	rec.Set("pair2", p2ID)
	rec.Set("status", "pending")
	rec.Set("round_number", 1)
	require.NoError(t, app.Save(rec))
	return rec
}

func seedCompetition(t *testing.T, app core.App, pairIDs []string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("name", "Test Comp")
	rec.Set("type", "league")
	rec.Set("active", true)
	rec.Set("pairs", pairIDs)
	require.NoError(t, app.Save(rec))
	return rec
}

func seedVenue(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("venues")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("name", name)
	require.NoError(t, app.Save(rec))
	return rec
}

func countRecords(t *testing.T, app core.App, collection string) int {
	t.Helper()
	recs, err := app.FindRecordsByFilter(collection, "id != ''", "", 0, 0)
	require.NoError(t, err)
	return len(recs)
}

func TestWipe(t *testing.T) {
	app := newTestApp(t)

	admin1 := seedUser(t, app, "admin1@test.local", "admin", "Admin 1")
	admin2 := seedUser(t, app, "admin2@test.local", "admin", "Admin 2")
	p1 := seedUser(t, app, "player1@test.local", "player", "Player 1")
	p2 := seedUser(t, app, "player2@test.local", "player", "Player 2")

	pair := seedPair(t, app, "Test Pair", p1, p2)
	comp := seedCompetition(t, app, []string{pair.Id})
	seedMatch(t, app, comp.Id, pair.Id, pair.Id)
	seedVenue(t, app, "Test Venue")

	venuesBefore := countRecords(t, app, "venues")

	summary, err := Wipe(app)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.Competitions)
	assert.Equal(t, 1, summary.Pairs)
	assert.Equal(t, 5, summary.Players)
	assert.Equal(t, 1, summary.Matches)

	assert.Equal(t, 0, countRecords(t, app, "competitions"))
	assert.Equal(t, 0, countRecords(t, app, "pairs"))
	assert.Equal(t, 0, countRecords(t, app, "matches"))

	// Admins survive
	_, err = app.FindRecordById("users", admin1.Id)
	require.NoError(t, err)
	_, err = app.FindRecordById("users", admin2.Id)
	require.NoError(t, err)

	// Only admins remain
	allUsers, err := app.FindRecordsByFilter("users", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(allUsers), "only the two admins should remain")

	// Venues untouched
	assert.Equal(t, venuesBefore, countRecords(t, app, "venues"))
}

func TestSampleLeague(t *testing.T) {
	app := newTestApp(t)

	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	hooks.Register(app, svc, notifier)

	require.NoError(t, SampleLeague(app))

	players, err := app.FindRecordsByFilter("users", "email ~ '@padelleague.com'", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 8, len(players))

	pairs, err := app.FindRecordsByFilter("pairs", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 4, len(pairs))

	comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 1, len(comps))
	assert.Equal(t, "Liga de ejemplo", comps[0].GetString("name"))
	assert.Equal(t, 6, comps[0].GetInt("rounds"))

	matches, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 12, len(matches))

	finalCount := 0
	for _, m := range matches {
		if m.GetString("status") == league.StatusFinal {
			finalCount++
		}
	}
	assert.Equal(t, 8, finalCount, "rounds 1-4 should have 2 matches each = 8 final")

	// Standings should compute without error
	svc = league.New(app, nil)
	standings, err := svc.ComputeStandings(comps[0].Id)
	require.NoError(t, err)
	assert.Equal(t, 4, len(standings))

	for i, s := range standings {
		t.Logf("standing %d: pair=%s pts=%d", i, s.PairName, s.Points)
	}
}
