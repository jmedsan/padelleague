package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterBackup_DisabledWithoutEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	registerBackup(app, BackupConfig{})

	jobs := app.Cron().Jobs()
	for _, j := range jobs {
		assert.NotEqual(t, "email-backup", j.Id(), "no backup cron should be registered without BACKUP_EMAIL")
	}
}

func TestRegisterBackup_Enabled_RegistersEvery12hCronAndExposesState(t *testing.T) {
	app := newTestApp(t)

	registerBackup(app, BackupConfig{Email: "backup@test.local"})

	jobs := app.Cron().Jobs()
	var found bool
	for _, j := range jobs {
		if j.Id() == "email-backup" {
			found = true
			assert.Equal(t, "0 */12 * * *", j.Expression())
			break
		}
	}
	assert.True(t, found, "email-backup cron job must be registered when BACKUP_EMAIL is set")
	assert.True(t, BackupEnabled())
	assert.NoError(t, RunBackupNow(), "RunBackupNow only errors when backup is unconfigured; internal send failures are logged, not returned")
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

func TestRunBackup_WithoutKeyRefusesToSendInAnyEnv(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// A missing EncryptionKey must abort before even creating a backup —
	// CreateBackup is never called, so the backups dir is never created.
	runBackup(app, BackupConfig{Email: "backup@test.local"})

	backupsDir := filepath.Join(app.DataDir(), core.LocalBackupsDirName)
	_, err := os.Stat(backupsDir)
	assert.True(t, os.IsNotExist(err), "no backup should even be created without an encryption key")
}

func TestRunBackup_WithKeyCreatesThenRemovesTempFiles(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// SMTP is not configured in the test app, so sendBackupEmail fails after
	// CreateBackup — this still exercises the create/read/encrypt/cleanup
	// path without a real mailer.
	runBackup(app, BackupConfig{Email: "backup@test.local", EncryptionKey: "test-passphrase"})

	backupsDir := filepath.Join(app.DataDir(), core.LocalBackupsDirName)
	entries, err := os.ReadDir(backupsDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the zip and its .attrs sidecar must both be removed after the send attempt")
}
