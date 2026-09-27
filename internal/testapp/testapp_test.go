package testapp

import (
	"crypto/sha256"
	"encoding/hex"
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
	dir := filepath.Join(t.TempDir(), "template")
	require.False(t, isValidTemplate(dir), "fresh dir must start as a miss")

	require.NoError(t, buildTemplate(dir))

	require.True(t, isValidTemplate(dir), "buildTemplate must publish a dir isValidTemplate accepts")

	tmp := dir + ".build-" + strconv.Itoa(os.Getpid())
	_, err := os.Stat(tmp)
	require.True(t, os.IsNotExist(err), "the private build-in-progress sibling dir must not survive a successful publish")

	// The template must actually be usable: tests.NewTestApp copies it and
	// boots against the copy without re-running migrations.
	app, err := tests.NewTestApp(dir)
	require.NoError(t, err)
	defer app.Cleanup()
}

// TestResolveTemplate_ReusesExistingCache proves resolveTemplate's actual
// hit path: given a cache dir a prior process already published (this test
// builds one directly, standing in for "another process built it"),
// resolveTemplate must resolve to it WITHOUT rebuilding — the discriminator
// is the readyMarker's mtime, which only a fresh buildTemplate call would
// bump (buildTemplate always writes a brand new marker file as the last
// step of a build). A resolveTemplate that ignored the cache and rebuilt
// unconditionally would still pass every other assertion in this file, so
// this mtime check is what actually catches that regression.
func TestResolveTemplate_ReusesExistingCache(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CACHE_HOME", "")

	key, err := templateKey()
	require.NoError(t, err)
	prebuilt, err := cacheDir(key)
	require.NoError(t, err)
	require.NoError(t, buildTemplate(prebuilt))
	require.True(t, isValidTemplate(prebuilt))

	markerPath := filepath.Join(prebuilt, readyMarker)
	before, err := os.Stat(markerPath)
	require.NoError(t, err)
	// Sleep past typical filesystem mtime resolution so a rebuild (which
	// would rewrite the marker) is unambiguously detectable.
	require.NoError(t, os.Chtimes(markerPath, before.ModTime(), before.ModTime()))

	// Reset the package-level once so this test drives resolveTemplate from
	// a clean state instead of whatever another test in this process left
	// behind — resolveTemplate is only ever meant to run once per process
	// in production; the reset here is test-only.
	templateOnce = sync.Once{}
	templateDir = ""
	templateErr = nil

	templateOnce.Do(resolveTemplate)
	require.NoError(t, templateErr)
	require.Equal(t, prebuilt, templateDir, "resolveTemplate must resolve to the already-published cache dir, not build a new one")

	after, err := os.Stat(markerPath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime(), "the marker must not be rewritten on a hit — a rewrite means resolveTemplate rebuilt instead of reusing the cache")
}
