package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// rcloneConfigTemplate is the Google Drive remote config for rclone. It uses
// OAuth2 with rclone's built-in published client (no client_id/client_secret),
// which avoids the 7-day token expiry of a custom OAuth app. scope = drive.file
// means the remote can only see files it created itself — root_folder_id must
// be a folder created via `rclone mkdir`, not one created in the Drive UI.
const rcloneConfigTemplate = `[gdrive]
type = drive
scope = drive.file
root_folder_id = %s
token = %s
`

// registerBackup wires an hourly backup of the PocketBase data directory to
// Google Drive via an OAuth token (RCLONE_DRIVE_TOKEN, from a one-time
// `rclone config` run). An empty FolderID or DriveToken disables it.
func registerBackup(app core.App, cfg BackupConfig) {
	if cfg.FolderID == "" || cfg.DriveToken == "" {
		slog.Info("startup", "backup", "disabled")
		return
	}

	configPath := filepath.Join(os.TempDir(), "rclone.conf")
	config := fmt.Sprintf(rcloneConfigTemplate, cfg.FolderID, cfg.DriveToken)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		slog.Error("backup: failed to write rclone config", "err", err)
		slog.Info("startup", "backup", "disabled")
		return
	}

	backupApp = app
	backupConfigPath = configPath

	app.Cron().MustAdd("gdrive-backup", "0 * * * *", func() {
		runBackup(app, configPath)
	})

	slog.Info("startup", "backup", "gdrive", "folder", cfg.FolderID)
}

var (
	backupApp        core.App
	backupConfigPath string
)

// BackupEnabled reports whether Google Drive backup is configured.
func BackupEnabled() bool { return backupConfigPath != "" }

// RunBackupNow triggers an immediate backup. Returns nil on success.
func RunBackupNow() error {
	if backupConfigPath == "" {
		return fmt.Errorf("backup not configured")
	}
	runBackup(backupApp, backupConfigPath)
	return nil
}

func runBackup(app core.App, configPath string) {
	name := fmt.Sprintf("pb-backup-%s.zip", time.Now().Format("20060102-150405"))
	if err := app.CreateBackup(context.Background(), name); err != nil {
		slog.Error("backup: create failed", "err", err)
		return
	}
	zipPath := filepath.Join(app.DataDir(), core.LocalBackupsDirName, name)
	defer func() { _ = os.Remove(zipPath) }()

	cmd := exec.Command("rclone", "copy", zipPath, "gdrive:", "--config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("backup: rclone copy failed", "err", err, "output", string(output))
		return
	}
	slog.Info("backup: uploaded", "file", name)
}
