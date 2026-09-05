package hooks

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
)

// rcloneConfigTemplate is the Google Drive remote definition rclone needs to
// sync with a service account (no interactive OAuth flow).
const rcloneConfigTemplate = `[gdrive]
type = drive
scope = drive
service_account_file = %s
root_folder_id = %s
`

// registerBackup wires an hourly rclone sync of the PocketBase data
// directory to a Google Drive folder, when GDRIVE_SERVICE_ACCOUNT is
// configured. Backups are best-effort: a single failed sync just logs and
// waits for the next hourly run, since a full data-dir sync is idempotent.
func registerBackup(app core.App, serviceAccountJSON, folderID string) {
	if serviceAccountJSON == "" {
		slog.Info("startup", "backup", "disabled")
		return
	}

	saPath := filepath.Join(os.TempDir(), "gdrive-sa.json")
	if err := os.WriteFile(saPath, []byte(serviceAccountJSON), 0o600); err != nil {
		slog.Error("backup: failed to write service account file", "err", err)
		slog.Info("startup", "backup", "disabled")
		return
	}

	configPath := filepath.Join(os.TempDir(), "rclone.conf")
	config := fmt.Sprintf(rcloneConfigTemplate, saPath, folderID)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		slog.Error("backup: failed to write rclone config", "err", err)
		slog.Info("startup", "backup", "disabled")
		return
	}

	dataDir := app.DataDir()
	app.Cron().MustAdd("gdrive-backup", "0 * * * *", func() {
		runBackup(configPath, dataDir)
	})

	slog.Info("startup", "backup", "gdrive", "folder", folderID)
}

// runBackup syncs dataDir to the root of the "gdrive" remote configured at
// configPath. The remote's root_folder_id already scopes it to the target
// Drive folder, so the destination is just "gdrive:" with no path suffix.
func runBackup(configPath, dataDir string) {
	cmd := exec.Command("rclone", "sync", dataDir, "gdrive:", "--config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("backup: rclone sync failed", "err", err, "output", string(output))
		return
	}
	slog.Info("backup: rclone sync succeeded")
}
