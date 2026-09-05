package hooks

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterBackup_DisabledWithoutServiceAccount(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, "", "")

	jobs := app.Cron().Jobs()
	for _, j := range jobs {
		assert.NotEqual(t, "gdrive-backup", j.Id(), "no backup cron should be registered without a service account")
	}
}

func TestRegisterBackup_RegistersHourlyCron(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, `{"type":"service_account","project_id":"test"}`, "test-folder-id")

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "gdrive-backup" {
			found = true
			assert.Equal(t, "0 * * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "gdrive-backup cron job must be registered when a service account is configured")
}

func TestRegisterBackup_WritesRcloneConfig(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, `{"type":"service_account","project_id":"test"}`, "test-folder-id")

	config, err := os.ReadFile(os.TempDir() + "/rclone.conf")
	require.NoError(t, err)
	assert.Contains(t, string(config), "type = drive")
	assert.Contains(t, string(config), "root_folder_id = test-folder-id")
	assert.Contains(t, string(config), "gdrive-sa.json")

	sa, err := os.ReadFile(os.TempDir() + "/gdrive-sa.json")
	require.NoError(t, err)
	assert.Equal(t, `{"type":"service_account","project_id":"test"}`, string(sa))
}
