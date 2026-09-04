package seed

import (
	"image"
	_ "image/jpeg"
	"os"
	"strings"
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

func seedMatch(t *testing.T, app core.App, compID, p1ID, p2ID string) {
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

	allOpts := WipeOptions{Players: true, Pairs: true, Competitions: true, Matches: true}
	summary, err := WipeSelective(app, allOpts)
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
	hooks.Register(app, svc, notifier, nil)

	require.NoError(t, SampleLeaguePartial(app, SampleOptions{
		Players: true, Pairs: true, Competitions: true, Matches: true,
		StaticFS: os.DirFS(".."),
	}))

	players, err := app.FindRecordsByFilter("users", "email ~ '@padelleague.com'", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 8, len(players))

	pairs, err := app.FindRecordsByFilter("pairs", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 4, len(pairs))

	comps, err := app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 2, len(comps))
	var mainComp *core.Record
	for _, c := range comps {
		if c.GetString("name") == "Dale Fuerte a la Bola" {
			mainComp = c
		}
	}
	require.NotNil(t, mainComp, "main competition must exist")
	assert.Equal(t, 6, mainComp.GetInt("rounds"))

	matches, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 12, len(matches))

	finalCount := 0
	for _, m := range matches {
		if m.GetString("status") == league.StatusFinal {
			finalCount++
		}
	}
	assert.Equal(t, 9, finalCount, "rounds 1-4 (8 matches) + round 6 walkover (1) = 9 final")

	scheduledCount := 0
	walkoverCount := 0
	for _, m := range matches {
		if m.GetString("status") == league.StatusScheduled {
			scheduledCount++
		}
		if m.GetString("review_type") == "walkover" {
			walkoverCount++
		}
	}
	assert.Equal(t, 1, scheduledCount, "one arranged-but-unplayed match")
	assert.Equal(t, 1, walkoverCount, "one walkover match")

	// One pair should be unpaid
	comp := mainComp
	ps := comp.Get("payment_status")
	if pm, ok := ps.(map[string]any); ok {
		unpaidCount := 0
		for _, paid := range pm {
			if p, ok := paid.(bool); ok && !p {
				unpaidCount++
			}
		}
		assert.Equal(t, 1, unpaidCount, "one pair should be unpaid")
	}

	// Invitations should exist
	invites, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(invites), "two sample invitations")

	// Venues should exist
	venues, err := app.FindRecordsByFilter("venues", "id != ''", "", 0, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(venues), 3, "at least 3 venue records")

	// Standings should compute without error
	svc = league.New(app, nil)
	standings, err := svc.ComputeStandings(comp.Id)
	require.NoError(t, err)
	assert.Equal(t, 4, len(standings))

	for i, s := range standings {
		t.Logf("standing %d: pair=%s pts=%d", i, s.PairName, s.Points)
	}
}

func TestSampleLeague_AvatarsUseLiveCompressionPipeline(t *testing.T) {
	app := newTestApp(t)

	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	hooks.Register(app, svc, notifier, nil)

	require.NoError(t, SampleLeaguePartial(app, SampleOptions{
		Players:      true,
		Pairs:        true,
		Competitions: true,
		Matches:      true,
		StaticFS:     os.DirFS(".."),
	}))

	players, err := app.FindRecordsByFilter("users", "email ~ '@padelleague.com'", "display_name", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 8, len(players))

	fsys, err := app.NewFilesystem()
	require.NoError(t, err)
	defer func() { _ = fsys.Close() }()

	for i, p := range players {
		avatarFile := p.GetString("avatar")
		if i >= 5 {
			assert.Empty(t, avatarFile, "player %d should have no seeded avatar", i+1)
			continue
		}
		require.NotEmpty(t, avatarFile, "player %d should have a seeded avatar", i+1)
		assert.True(t, strings.HasSuffix(avatarFile, ".jpg"), "seeded avatar %q should be a JPEG, matching the live upload pipeline's output format", avatarFile)

		r, err := fsys.GetReader(p.BaseFilesPath() + "/" + avatarFile)
		require.NoError(t, err)
		cfg, format, err := image.DecodeConfig(r)
		_ = r.Close()
		require.NoError(t, err)
		assert.Equal(t, "jpeg", format)
		assert.Equal(t, 400, cfg.Width, "seeded avatar should be resized to the same 400x400 the live handler produces")
		assert.Equal(t, 400, cfg.Height)
	}
}

func TestSampleLeagueWithPlayoff(t *testing.T) {
	app := newTestApp(t)

	notifier := notify.NewNotifier(app, "", "")
	svc := league.New(app, notifier)
	hooks.Register(app, svc, notifier, nil)

	require.NoError(t, SampleLeaguePartial(app, SampleOptions{
		Players: true, Pairs: true, Competitions: true, Matches: true, Playoff: true,
		StaticFS: os.DirFS(".."),
	}))

	comps, err := app.FindRecordsByFilter("competitions", "id != ''", "name", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 3, len(comps), "league + mixed + playoff")

	var playoff *core.Record
	for _, c := range comps {
		if c.GetString("type") == "playoff" {
			playoff = c
		}
	}
	require.NotNil(t, playoff, "playoff competition must exist")
	assert.Equal(t, "Playoff de ejemplo", playoff.GetString("name"))

	playoffMatches, err := app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": playoff.Id})
	require.NoError(t, err)
	assert.Greater(t, len(playoffMatches), 0, "playoff bracket must have matches")
}

func TestWipeSelective_PlayersOnly(t *testing.T) {
	app := newTestApp(t)

	seedUser(t, app, "player1@test.local", "player", "Player 1")
	seedUser(t, app, "player2@test.local", "player", "Player 2")
	seedUser(t, app, "admin1@test.local", "admin", "Admin 1")

	summary, err := WipeSelective(app, WipeOptions{Players: true})
	require.NoError(t, err)

	assert.True(t, summary.Players > 0, "should delete non-admin players")
	assert.Equal(t, 0, summary.Pairs, "should not touch pairs")

	_, err = app.FindAuthRecordByEmail("users", "admin1@test.local")
	require.NoError(t, err, "admin should survive")
}

func TestWipeSelective_PlayersOnly_FailsWithPairs(t *testing.T) {
	app := newTestApp(t)

	p1 := seedUser(t, app, "player1@test.local", "player", "Player 1")
	p2 := seedUser(t, app, "player2@test.local", "player", "Player 2")
	seedPair(t, app, "Test Pair", p1, p2)

	_, err := WipeSelective(app, WipeOptions{Players: true})
	assert.Error(t, err, "should fail when pairs reference players")
}

func TestWipeSelective_AllEqualsWipe(t *testing.T) {
	app := newTestApp(t)

	p1 := seedUser(t, app, "player1@test.local", "player", "Player 1")
	p2 := seedUser(t, app, "player2@test.local", "player", "Player 2")
	pair := seedPair(t, app, "Test Pair", p1, p2)
	comp := seedCompetition(t, app, []string{pair.Id})
	seedMatch(t, app, comp.Id, pair.Id, pair.Id)

	summary, err := WipeSelective(app, WipeOptions{
		Players: true, Pairs: true,
		Competitions: true, Matches: true,
	})
	require.NoError(t, err)

	assert.True(t, summary.Players > 0)
	assert.Equal(t, 1, summary.Pairs)
	assert.Equal(t, 1, summary.Competitions)
	assert.Equal(t, 1, summary.Matches)

	assert.Equal(t, 0, countRecords(t, app, "pairs"))
	assert.Equal(t, 0, countRecords(t, app, "competitions"))
	assert.Equal(t, 0, countRecords(t, app, "matches"))
}
