package seed

import (
	"testing"

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
		{Email: "seed1@test.local", Password: "pass123456", Collection: "users", Role: "admin", DisplayName: "Seed Admin"},
		{Email: "seed2@test.local", Password: "pass123456", Collection: "users", Role: "player", DisplayName: "Seed Player"},
	})

	u1, err := app.FindAuthRecordByEmail("users", "seed1@test.local")
	require.NoError(t, err)
	assert.Equal(t, "admin", u1.GetString("role"))
	assert.Equal(t, "Seed Admin", u1.GetString("display_name"))
	assert.True(t, u1.Verified())

	u2, err := app.FindAuthRecordByEmail("users", "seed2@test.local")
	require.NoError(t, err)
	assert.Equal(t, "player", u2.GetString("role"))
}

func TestRun_SkipsEmptyEmailOrPassword(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "", Password: "pass123456", Collection: "users", Role: "player"},
		{Email: "nopass@test.local", Password: "", Collection: "users", Role: "player"},
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
	existing.Set("role", "player")
	existing.SetPassword("original123456")
	existing.SetVerified(true)
	require.NoError(t, app.Save(existing))

	Run(app, []User{
		{Email: "existing@test.local", Password: "newpass123456", Collection: "users", Role: "admin", DisplayName: "Changed"},
	})

	u, err := app.FindAuthRecordByEmail("users", "existing@test.local")
	require.NoError(t, err)
	assert.Equal(t, "player", u.GetString("role"), "existing user should not be overwritten")
	assert.Equal(t, "Original", u.GetString("display_name"), "existing display_name should not change")
}

func TestRun_InvalidCollection(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "bad@test.local", Password: "pass123456", Collection: "nonexistent", Role: "player"},
	})

	// Should not panic; user should not exist anywhere.
	_, err := app.FindAuthRecordByEmail("users", "bad@test.local")
	assert.Error(t, err)
}

func TestRun_MissingRequiredFields_LogsAndContinues(t *testing.T) {
	app := newTestApp(t)

	Run(app, []User{
		{Email: "norole@test.local", Password: "pass123456", Collection: "users"},
		{Email: "good@test.local", Password: "pass123456", Collection: "users", Role: "player", DisplayName: "Good"},
	})

	_, err := app.FindAuthRecordByEmail("users", "norole@test.local")
	assert.Error(t, err, "user with missing required fields should not be created")

	u, err := app.FindAuthRecordByEmail("users", "good@test.local")
	require.NoError(t, err)
	assert.Equal(t, "player", u.GetString("role"), "subsequent valid user should still be created")
}
