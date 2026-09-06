package hooks

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// rcloneConfigTemplate is the Google Drive remote config for rclone.
// When GDRIVE_SERVICE_ACCOUNT is set, it uses a service account (requires
// Google Workspace). When RCLONE_DRIVE_TOKEN is set, it uses OAuth2 (works
// with free Gmail — run `rclone config` once to generate the token).
const rcloneConfigTemplate = `[gdrive]
type = drive
scope = drive.file
root_folder_id = %s
%s
`

// registerBackup wires an hourly backup of the PocketBase data directory
// to Google Drive, when configured. Supports two auth modes:
// - Service account (GDRIVE_SERVICE_ACCOUNT): for Google Workspace
// - OAuth token (RCLONE_DRIVE_TOKEN): for free Gmail (one-time setup)
func registerBackup(app core.App, cfg BackupConfig) {
	if cfg.FolderID == "" {
		slog.Info("startup", "backup", "disabled")
		return
	}

	authLine := ""
	if cfg.ServiceAccountJSON != "" {
		saPath := filepath.Join(os.TempDir(), "gdrive-sa.json")
		if err := os.WriteFile(saPath, []byte(cfg.ServiceAccountJSON), 0o600); err != nil {
			slog.Error("backup: failed to write service account file", "err", err)
			slog.Info("startup", "backup", "disabled")
			return
		}
		authLine = "service_account_file = " + saPath
	} else if cfg.DriveToken != "" {
		authLine = "token = " + cfg.DriveToken
	} else {
		slog.Info("startup", "backup", "disabled (no auth: set GDRIVE_SERVICE_ACCOUNT or RCLONE_DRIVE_TOKEN)")
		return
	}

	configPath := filepath.Join(os.TempDir(), "rclone.conf")
	config := fmt.Sprintf(rcloneConfigTemplate, cfg.FolderID, authLine)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		slog.Error("backup: failed to write rclone config", "err", err)
		slog.Info("startup", "backup", "disabled")
		return
	}

	dataDir := app.DataDir()
	app.Cron().MustAdd("gdrive-backup", "0 * * * *", func() {
		runBackup(configPath, dataDir)
	})

	backupConfigPath = configPath
	backupDataDir = dataDir

	slog.Info("startup", "backup", "gdrive", "folder", cfg.FolderID)
}

var backupConfigPath, backupDataDir string

// BackupEnabled reports whether Google Drive backup is configured.
func BackupEnabled() bool { return backupConfigPath != "" }

// RunBackupNow triggers an immediate backup. Returns nil on success.
func RunBackupNow() error {
	if backupConfigPath == "" {
		return fmt.Errorf("backup not configured")
	}
	runBackup(backupConfigPath, backupDataDir)
	return nil
}

func runBackup(configPath, dataDir string) {
	// Create a timestamped zip of the data directory to avoid syncing
	// the live SQLite (which may be mid-write). PocketBase's data dir
	// contains data.db, auxiliary.db, and the storage/ folder.
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("pb-backup-%s.zip", time.Now().Format("20060102-150405")))
	if err := zipDir(dataDir, zipPath); err != nil {
		slog.Error("backup: zip failed", "err", err)
		return
	}
	defer os.Remove(zipPath)

	cmd := exec.Command("rclone", "copy", zipPath, "gdrive:", "--config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("backup: rclone copy failed", "err", err, "output", string(output))
		return
	}
	slog.Info("backup: uploaded", "file", filepath.Base(zipPath))
}

func zipDir(srcDir, dstPath string) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		zf, err := w.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(zf, src)
		return err
	})
}
