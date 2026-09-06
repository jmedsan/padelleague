package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterBackup_DisabledWithoutConfig(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, BackupConfig{})

	jobs := app.Cron().Jobs()
	for _, j := range jobs {
		assert.NotEqual(t, "gdrive-backup", j.Id(), "no backup cron should be registered without config")
	}
}

func TestRegisterBackup_DisabledWithoutToken(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, BackupConfig{FolderID: "test-folder-id"})

	jobs := app.Cron().Jobs()
	for _, j := range jobs {
		assert.NotEqual(t, "gdrive-backup", j.Id(), "no backup cron should be registered without a drive token")
	}
}

func TestRegisterBackup_OAuthToken_RegistersHourlyCron(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, BackupConfig{
		DriveToken: `{"access_token":"ya29.test","refresh_token":"1//test","expiry":"2026-01-01T00:00:00Z"}`,
		FolderID:   "test-folder-id",
	})

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "gdrive-backup" {
			found = true
			assert.Equal(t, "0 * * * *", j.Expression(), "zero IntervalMin defaults to hourly")
			break
		}
	}
	assert.True(t, found, "gdrive-backup cron job must be registered when an OAuth token is configured")
}

func TestBackupCronExpr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		intervalMin int
		want        string
	}{
		{"zero defaults to hourly", 0, "0 * * * *"},
		{"negative defaults to hourly", -5, "0 * * * *"},
		{"sub-hour interval runs every N minutes", 15, "*/15 * * * *"},
		{"exactly 60 runs hourly", 60, "0 * * * *"},
		{"multi-hour interval", 180, "0 */3 * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, backupCronExpr(tc.intervalMin))
		})
	}
}

func TestRegisterBackup_WritesRcloneConfig(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	token := `{"access_token":"ya29.test","refresh_token":"1//test"}`
	registerBackup(app, BackupConfig{
		DriveToken: token,
		FolderID:   "test-folder-id",
	})

	config, err := os.ReadFile(os.TempDir() + "/rclone.conf")
	require.NoError(t, err)
	assert.Contains(t, string(config), "type = drive")
	assert.Contains(t, string(config), "scope = drive.file")
	assert.Contains(t, string(config), "root_folder_id = test-folder-id")
	assert.Contains(t, string(config), "token = "+token)
	assert.NotContains(t, string(config), "client_id", "config must not contain a custom OAuth client")
	assert.NotContains(t, string(config), "client_secret", "config must not contain a custom OAuth client")
}

func TestRunBackup_CreatesBackupViaCreateBackup(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	configPath := filepath.Join(t.TempDir(), "rclone.conf")
	require.NoError(t, os.WriteFile(configPath, []byte("[gdrive]\n"), 0o600))

	// No rclone binary configured to succeed against a fake remote, but
	// CreateBackup must run (and the zip must exist briefly) before the
	// rclone copy attempt, regardless of whether that upload succeeds.
	runBackup(app, configPath)

	backupsDir := filepath.Join(app.DataDir(), core.LocalBackupsDirName)
	entries, err := os.ReadDir(backupsDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the zip must be removed after the upload attempt")
}
