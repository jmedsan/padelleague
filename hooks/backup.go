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

// registerBackup wires a periodic backup of the PocketBase data directory to
// Google Drive via an OAuth token (RCLONE_DRIVE_TOKEN, from a one-time
// `rclone config` run). An empty FolderID or DriveToken disables it. The
// cron frequency is set by cfg.IntervalMin (BACKUP_INTERVAL_MINUTES).
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

	expr := backupCronExpr(cfg.IntervalMin)
	app.Cron().MustAdd("gdrive-backup", expr, func() {
		runBackup(app, configPath)
	})

	slog.Info("startup", "backup", "gdrive", "folder", cfg.FolderID, "interval_min", cfg.IntervalMin, "cron", expr)
}

// backupCronExpr builds the cron expression for the backup job from an
// interval in minutes. Intervals under 60 run every N minutes; 60 and above
// run every N/60 hours. Zero or negative defaults to hourly.
func backupCronExpr(intervalMin int) string {
	if intervalMin <= 0 {
		intervalMin = 60
	}
	if intervalMin < 60 {
		return fmt.Sprintf("*/%d * * * *", intervalMin)
	}
	if hours := intervalMin / 60; hours > 1 {
		return fmt.Sprintf("0 */%d * * *", hours)
	}
	return "0 * * * *"
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
	defer func() {
		_ = os.Remove(zipPath)
		_ = os.Remove(zipPath + ".attrs")
	}()

	cmd := exec.Command("rclone", "copy", zipPath, "gdrive:", "--config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("backup: rclone copy failed", "err", err, "output", string(output))
		return
	}
	slog.Info("backup: uploaded", "file", name)
}
