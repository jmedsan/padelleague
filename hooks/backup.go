package hooks

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"padelleague/notify"
)

// BackupConfig configures the email backup cron. An empty Email disables it.
type BackupConfig struct {
	Email string
	// EncryptionKey is the openssl passphrase for the backup archive. Empty
	// refuses to send in every environment — the archive holds personal data
	// (names, emails, phones) and is never mailed in the clear.
	EncryptionKey string
}

var (
	backupApp core.App
	backupCfg BackupConfig
)

// BackupEnabled reports whether email backup is configured.
func BackupEnabled() bool { return backupCfg.Email != "" }

// RunBackupNow triggers an immediate backup. Returns an error if backup is not configured.
func RunBackupNow() error {
	if backupCfg.Email == "" {
		return fmt.Errorf("backup not configured")
	}
	runBackup(backupApp, backupCfg)
	return nil
}

// registerBackup wires a cron that emails a zip of pb_data/ to cfg.Email
// every 12 hours. An empty Email disables it.
func registerBackup(app core.App, cfg BackupConfig) {
	if cfg.Email == "" {
		slog.Info("startup", "backup", "disabled (no BACKUP_EMAIL)")
		return
	}

	backupApp = app
	backupCfg = cfg

	app.Cron().MustAdd("email-backup", "0 */12 * * *", func() {
		runBackup(app, cfg)
	})

	slog.Info("startup", "backup", "email", "interval", "12h", "to", cfg.Email)
}

// backupDecryptCmd is the exact command that decrypts an emailed .zip.enc
// backup, given the BACKUP_ENCRYPTION_KEY value as its passphrase.
const backupDecryptCmd = "openssl enc -d -aes-256-cbc -pbkdf2 -pass env:BACKUP_ENCRYPTION_KEY -in backup.zip.enc -out backup.zip"

// runBackup creates a zip of pb_data/ via PocketBase's backup API, encrypts
// it, and emails it to cfg.Email. A missing EncryptionKey aborts the backup
// in every environment — the archive holds personal data and is never
// mailed in the clear.
func runBackup(app core.App, cfg BackupConfig) {
	if cfg.EncryptionKey == "" {
		slog.Error("backup failed", "err", "BACKUP_ENCRYPTION_KEY is required, refusing to send unencrypted backup")
		return
	}

	name := fmt.Sprintf("pb-backup-%s.zip", time.Now().Format("20060102-150405"))
	if err := app.CreateBackup(context.Background(), name); err != nil {
		slog.Error("backup failed", "err", err)
		return
	}
	zipPath := filepath.Join(app.DataDir(), core.LocalBackupsDirName, name)
	defer func() {
		_ = os.Remove(zipPath)
		_ = os.Remove(zipPath + ".attrs")
	}()

	data, err := os.ReadFile(zipPath)
	if err != nil {
		slog.Error("backup failed", "err", err)
		return
	}

	encrypted, err := encryptBackup(data, cfg.EncryptionKey)
	if err != nil {
		slog.Error("backup failed", "err", err)
		return
	}

	if err := sendBackupEmail(app, cfg.Email, name+".enc", encrypted); err != nil {
		slog.Error("backup failed", "err", err)
		return
	}
	slog.Info("backup sent", "to", cfg.Email, "size", len(encrypted), "encrypted", true, "decrypt", backupDecryptCmd)
}

// opensslSaltedMagic is the 8-byte header openssl enc writes on every
// -pbkdf2 output, identifying the "Salted__" + 8-byte-salt format.
var opensslSaltedMagic = []byte("Salted__")

// opensslPBKDF2Iter and opensslKeyIVLen match openssl 3.x's defaults for
// `openssl enc -aes-256-cbc -pbkdf2` with no -iter override: PBKDF2-HMAC-SHA256,
// 10000 iterations, deriving 48 bytes (32-byte key + 16-byte IV) in one call.
const (
	opensslPBKDF2Iter = 10000
	opensslKeyIVLen   = aes.BlockSize + 32 // 16-byte IV + 32-byte AES-256 key
)

// encryptBackup encrypts data with AES-256-CBC + PKCS7 padding, using a
// random 8-byte salt and PBKDF2-derived key/IV from passphrase — the exact
// wire format `openssl enc -aes-256-cbc -pbkdf2` produces, so the result
// decrypts with:
//
//	openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:<passphrase> -in backup.zip.enc -out backup.zip
func encryptBackup(data []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	keyIV, err := pbkdf2.Key(sha256.New, passphrase, salt, opensslPBKDF2Iter, opensslKeyIVLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	key, iv := keyIV[:32], keyIV[32:] // openssl -pbkdf2 derives key bytes first, then IV

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	padded := pkcs7Pad(data, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	out := make([]byte, 0, len(opensslSaltedMagic)+len(salt)+len(ciphertext))
	out = append(out, opensslSaltedMagic...)
	out = append(out, salt...)
	out = append(out, ciphertext...)
	return out, nil
}

// pkcs7Pad pads data to a multiple of blockSize per RFC 5652 (openssl's default).
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

// sendBackupEmail emails the archive as an attachment via the configured SMTP mailer.
func sendBackupEmail(app core.App, to, attachmentName string, data []byte) error {
	if !notify.IsMailerConfigured(app) {
		return fmt.Errorf("SMTP not configured")
	}
	now := time.Now()
	subject := notify.SubjectPrefix() + "[Liga Dale Fuerte] Backup " + now.Format("2006-01-02")
	body := "Backup automático de pb_data/ — " + now.Format("2006-01-02 15:04:05")

	msg := &mailer.Message{
		From: mail.Address{
			Name:    app.Settings().Meta.SenderName,
			Address: app.Settings().Meta.SenderAddress,
		},
		To:      []mail.Address{{Address: to}},
		Subject: subject,
		Text:    body,
		Attachments: map[string]io.Reader{
			attachmentName: bytes.NewReader(data),
		},
	}
	return app.NewMailClient().Send(msg)
}
