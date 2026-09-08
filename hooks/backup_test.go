package hooks

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupCronExpr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		intervalHours int
		want          string
	}{
		{"zero defaults to every 12h", 0, "0 */12 * * *"},
		{"negative defaults to every 12h", -3, "0 */12 * * *"},
		{"sub-24h interval runs every N hours", 6, "0 */6 * * *"},
		{"exactly 24 runs daily", 24, "0 0 * * *"},
		{"above 24 runs daily", 48, "0 0 * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, backupCronExpr(tc.intervalHours))
		})
	}
}

func TestRegisterBackup_DisabledWithoutEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	before := app.Settings().Backups.Cron

	registerBackup(app, BackupConfig{})
	require.NoError(t, app.OnServe().Trigger(&core.ServeEvent{App: app}, func(*core.ServeEvent) error { return nil }))

	assert.Equal(t, before, app.Settings().Backups.Cron, "backup settings must be untouched without BACKUP_EMAIL")
}

func TestRegisterBackup_Enabled_SetsBackupCronSettings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, BackupConfig{Email: "backup@test.local", IntervalHours: 6})
	require.NoError(t, app.OnServe().Trigger(&core.ServeEvent{App: app}, func(*core.ServeEvent) error { return nil }))

	assert.Equal(t, "0 */6 * * *", app.Settings().Backups.Cron)
	assert.Equal(t, 2, app.Settings().Backups.CronMaxKeep)
}

func TestEncryptBackup_ProducesOpensslSaltedFormat(t *testing.T) {
	t.Parallel()
	plaintext := []byte("pb_data backup contents")
	ciphertext, err := encryptBackup(plaintext, "test-passphrase")
	require.NoError(t, err)

	require.Len(t, ciphertext, len(opensslSaltedMagic)+8+32, // header(8) + salt(8) + two padded AES blocks
		"24-byte plaintext PKCS7-pads to two 16-byte AES blocks")
	assert.Equal(t, opensslSaltedMagic, ciphertext[:8], "must start with the openssl Salted__ magic")
	assert.NotEqual(t, plaintext, ciphertext, "must not be plaintext")
}

func TestEncryptBackup_RandomSaltEachCall(t *testing.T) {
	t.Parallel()
	a, err := encryptBackup([]byte("data"), "pass")
	require.NoError(t, err)
	b, err := encryptBackup([]byte("data"), "pass")
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "each encryption must use a fresh random salt")
}

// TestEncryptBackup_DecryptsWithRealOpenssl round-trips through the actual
// openssl binary to prove the wire format matches
// `openssl enc -aes-256-cbc -pbkdf2` byte-for-byte, not just our own decoder.
func TestEncryptBackup_DecryptsWithRealOpenssl(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available in this environment")
	}
	t.Parallel()

	plaintext := []byte("pb_data backup contents for openssl round-trip")
	passphrase := "correct horse battery staple"
	ciphertext, err := encryptBackup(plaintext, passphrase)
	require.NoError(t, err)

	dir := t.TempDir()
	encPath := filepath.Join(dir, "backup.enc")
	outPath := filepath.Join(dir, "backup.out")
	require.NoError(t, os.WriteFile(encPath, ciphertext, 0o600))

	cmd := exec.Command("openssl", "enc", "-d", "-aes-256-cbc", "-pbkdf2",
		"-pass", "pass:"+passphrase, "-in", encPath, "-out", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "openssl decrypt failed: %s", output)

	decrypted, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEmailBackup_WithoutKeyRefusesToSend(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTPForBackupTest(t, app)

	// A missing EncryptionKey must refuse to send, regardless of environment.
	emailBackup(app, BackupConfig{Email: "backup@test.local"}, "does-not-matter.zip")

	assert.Equal(t, 0, app.TestMailer.TotalSend(), "no email must be sent without an encryption key")
}

// TestOnBackupCreate_EmailsEncryptedAttachment drives the real
// app.CreateBackup() flow (the same path PocketBase's own cron and the
// admin "Backup ahora" button use) and asserts the OnBackupCreate hook
// emails an encrypted attachment once the zip exists.
func TestOnBackupCreate_EmailsEncryptedAttachment(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTPForBackupTest(t, app)

	registerBackup(app, BackupConfig{Email: "backup@test.local", EncryptionKey: "test-passphrase"})

	require.NoError(t, app.CreateBackup(context.Background(), "test-backup.zip"))

	require.Equal(t, 1, app.TestMailer.TotalSend())
	msg := app.TestMailer.LastMessage()
	assert.Equal(t, "backup@test.local", msg.To[0].Address)
	assert.Contains(t, msg.Subject, "Backup")

	require.Len(t, msg.Attachments, 1)
	for name, r := range msg.Attachments {
		assert.Equal(t, "test-backup.zip.enc", name)
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, opensslSaltedMagic, data[:8], "attachment must be openssl-format encrypted")
	}
}

// enableSMTPForBackupTest flips the app into "mailer configured" mode.
// tests.TestApp routes NewMailClient() to its TestMailer, so nothing leaves
// the process.
func enableSMTPForBackupTest(t *testing.T, app *tests.TestApp) {
	t.Helper()
	app.Settings().SMTP.Enabled = true
	app.Settings().SMTP.Host = "smtp.test.local"
	app.Settings().Meta.SenderAddress = "sender@test.local"
	app.Settings().Meta.SenderName = "Test Sender"
}
