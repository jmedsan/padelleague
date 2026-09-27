package testapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// TestTemplateKey_ChangesWithMigrationContent proves the cache key is
// content-derived: hashDir feeds every migrations/*.go file into the same
// hash templateKey uses, so changing one file's content changes the key —
// the actual guarantee that makes the disk cache safe to reuse across
// processes without a version number to remember to bump.
func TestTemplateKey_ChangesWithMigrationContent(t *testing.T) {
	dirA := t.TempDir()
	writeGoFile(t, dirA, "0001_init.go", "package migrations\n// v1\n")

	dirB := t.TempDir()
	writeGoFile(t, dirB, "0001_init.go", "package migrations\n// v2 (different)\n")

	require.NotEqual(t, digestOf(t, dirA), digestOf(t, dirB),
		"hashDir must produce a different digest when migration content differs")
}

// TestTemplateKey_StableForIdenticalContent proves the key is deterministic
// (same inputs, same key) and independent of directory-entry iteration
// order — both required for cross-process cache hits to actually happen.
func TestTemplateKey_StableForIdenticalContent(t *testing.T) {
	dirA := t.TempDir()
	writeGoFile(t, dirA, "0002_second.go", "package migrations\n// b\n")
	writeGoFile(t, dirA, "0001_first.go", "package migrations\n// a\n")

	dirB := t.TempDir()
	writeGoFile(t, dirB, "0001_first.go", "package migrations\n// a\n")
	writeGoFile(t, dirB, "0002_second.go", "package migrations\n// b\n")

	require.Equal(t, digestOf(t, dirA), digestOf(t, dirB),
		"hashDir must be order-independent: same file set and content, different creation order")
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func digestOf(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	require.NoError(t, hashDir(h, dir))
	return hex.EncodeToString(h.Sum(nil))
}

// TestIsValidTemplate_MissingMarker proves a directory without readyMarker
// (never built, or a process died mid-build before publishing) is treated
// as a miss, never as a usable template.
func TestIsValidTemplate_MissingMarker(t *testing.T) {
	dir := t.TempDir()
	require.False(t, isValidTemplate(dir), "a directory with no .ready file must not be considered valid")
}

// TestIsValidTemplate_WithMarker proves a directory carrying the marker
// (the state buildTemplate leaves after a successful atomic rename) is
// treated as a hit.
func TestIsValidTemplate_WithMarker(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, readyMarker), nil, 0o644))
	require.True(t, isValidTemplate(dir))
}

// TestBuildTemplate_PublishesAtomically proves the hit/miss path end to
// end against the real PocketBase seed app: a fresh dir is a miss, gets
// built, and afterwards isValidTemplate reports a hit — the exact sequence
// resolveTemplate drives, and the one a second process concurrently
// resolving the same key relies on to find a usable template. It also
// proves the published template is actually bootable, and that no
// build-in-progress sibling directory is left behind after a clean publish.
func TestBuildTemplate_PublishesAtomically(t *testing.T) {
	resetTemplateState(t)
	dir := filepath.Join(t.TempDir(), "template")
	require.False(t, isValidTemplate(dir), "fresh dir must start as a miss")

	require.NoError(t, buildTemplate(dir))

	require.True(t, isValidTemplate(dir), "buildTemplate must publish a dir isValidTemplate accepts")
	require.Equal(t, dir, templateDir, "a successful publish must point templateDir at the published dir")

	tmp := dir + ".build-" + strconv.Itoa(os.Getpid())
	_, err := os.Stat(tmp)
	require.True(t, os.IsNotExist(err), "the private build-in-progress sibling dir must not survive a successful publish")

	// The template must actually be usable: tests.NewTestApp copies it and
	// boots against the copy without re-running migrations.
	app, err := tests.NewTestApp(dir)
	require.NoError(t, err)
	defer app.Cleanup()
}

// TestBuildTemplate_LostRaceReusesWinner proves the lost-race branch: a
// second build for a dir that is already published cannot rename over it,
// so it must discard its own build and adopt the existing valid dir.
func TestBuildTemplate_LostRaceReusesWinner(t *testing.T) {
	resetTemplateState(t)
	dir := filepath.Join(t.TempDir(), "template")
	require.NoError(t, buildTemplate(dir))
	templateDir = ""

	require.NoError(t, buildTemplate(dir), "losing the race to a valid dir is not an error")

	require.Equal(t, dir, templateDir, "the loser must adopt the winner's dir")
	_, err := os.Stat(dir + ".build-" + strconv.Itoa(os.Getpid()))
	require.True(t, os.IsNotExist(err), "the loser must remove its own build-in-progress dir")
}

// TestBuildTemplate_RenameBlockedByInvalidDir proves the lost-race branch
// only adopts a VALID dir: a non-empty dir without readyMarker (a crashed
// build) blocks the rename and must surface as an error.
func TestBuildTemplate_RenameBlockedByInvalidDir(t *testing.T) {
	resetTemplateState(t)
	dir := filepath.Join(t.TempDir(), "template")
	writeGoFile(t, mkdir(t, dir), "leftover.go", "package x\n")

	require.ErrorContains(t, buildTemplate(dir), "rename template into place")
	require.Empty(t, templateDir)
}

// TestResolveTemplate_ReusesExistingCache proves resolveTemplate takes the
// hit path for a cache dir another process already published. The log line
// is the discriminator: a rebuild on a valid dir also ends with the same
// templateDir (through buildTemplate's lost-race branch), so only the hit
// log tells the two apart.
func TestResolveTemplate_ReusesExistingCache(t *testing.T) {
	resetTemplateState(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")

	key, err := templateKey()
	require.NoError(t, err)
	prebuilt, err := cacheDir(key)
	require.NoError(t, err)
	require.NoError(t, buildTemplate(prebuilt))
	templateDir = ""

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	templateOnce.Do(resolveTemplate)

	require.NoError(t, templateErr)
	require.Equal(t, prebuilt, templateDir)
	require.Contains(t, logs.String(), "template cache hit")
	require.NotContains(t, logs.String(), "template cache miss")
}

// TestKeyInputs_CoversEveryTemplateInput pins the exact input list, so
// dropping go.mod, go.sum, this file, or migrations/ from the key fails.
func TestKeyInputs_CoversEveryTemplateInput(t *testing.T) {
	files, migrationsDir := keyInputs()
	require.Equal(t, []string{repoPath("go.mod"), repoPath("go.sum"), selfPath()}, files)
	require.Equal(t, repoPath("migrations"), migrationsDir)
	require.Equal(t, "testapp.go", filepath.Base(selfPath()))
	for _, f := range append(files, migrationsDir) {
		_, err := os.Stat(f)
		require.NoError(t, err, "key input %s must exist", f)
	}
}

// TestHashInputs_ChangesWithEachFile proves every listed file feeds the key:
// changing any one of them changes it.
func TestHashInputs_ChangesWithEachFile(t *testing.T) {
	root := t.TempDir()
	migrations := mkdir(t, filepath.Join(root, "migrations"))
	writeGoFile(t, migrations, "0001_init.go", "package migrations\n")
	names := []string{"go.mod", "go.sum", "testapp.go"}
	var files []string
	for _, n := range names {
		writeGoFile(t, root, n, n+" v1\n")
		files = append(files, filepath.Join(root, n))
	}
	base, err := hashInputs(files, migrations)
	require.NoError(t, err)

	for _, n := range names {
		writeGoFile(t, root, n, n+" v2\n")
		changed, err := hashInputs(files, migrations)
		require.NoError(t, err)
		require.NotEqual(t, base, changed, "changing %s must change the key", n)
		writeGoFile(t, root, n, n+" v1\n")
	}
}

func mkdir(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

// resetTemplateState clears the package-level template state before a test
// and again after, so tests that drive buildTemplate/resolveTemplate
// directly don't leak into each other. sync.Once can't be copied, so the
// cleanup leaves a fresh Once: the next New re-resolves (a cache hit).
func resetTemplateState(t *testing.T) {
	t.Helper()
	templateOnce, templateDir, templateErr = sync.Once{}, "", nil
	t.Cleanup(func() { templateOnce, templateDir, templateErr = sync.Once{}, "", nil })
}
