// Package testapp hands tests a PocketBase app built from a pre-migrated
// template instead of re-running every migration per test.
//
// Callers must import padelleague/migrations for side effects so the
// template includes the app schema; this package does not, because the
// migrations package's own tests would otherwise form an import cycle.
package testapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"golang.org/x/crypto/bcrypt"
)

var (
	templateOnce sync.Once
	templateDir  string
	templateErr  error
)

// Run is a thin wrapper around m.Run, kept so every TestMain can call
// os.Exit(testapp.Run(m)) without caring how the template is managed.
//
// The template lives in a hash-keyed cache directory shared across
// processes (see cacheDir), not a per-process temp dir, so Run does NOT
// remove it on exit: many go test binaries (one per package, and under
// mutation testing one per mutant) each build their own template
// independently, and deleting it per process would force every one of them
// to replay all migrations from scratch. The cache is pruned by OS
// temp/cache cleanup, not by this package.
func Run(m *testing.M) int {
	return m.Run()
}

// New returns a fresh app copied from the template, cleaned up with tb.
func New(tb testing.TB) *tests.TestApp {
	tb.Helper()
	app := Factory(tb)
	tb.Cleanup(app.Cleanup)
	return app
}

// Factory returns a fresh app copied from the template without registering
// cleanup; tests.ApiScenario.TestAppFactory cleans up on its own.
func Factory(tb testing.TB) *tests.TestApp {
	tb.Helper()
	templateOnce.Do(resolveTemplate)
	if templateErr != nil {
		tb.Fatalf("testapp: resolve template: %v", templateErr)
	}
	app, err := tests.NewTestApp(templateDir)
	if err != nil {
		tb.Fatalf("testapp: copy template: %v", err)
	}
	return app
}

// resolveTemplate finds or builds the on-disk template for the current
// migrations/dependencies, so a fresh go test process (gremlins spawns one
// per mutant) can reuse a template another process already built instead of
// replaying every migration.
func resolveTemplate() {
	templateErr = func() error {
		key, err := templateKey()
		if err != nil {
			return fmt.Errorf("compute template key: %w", err)
		}
		dir, err := cacheDir(key)
		if err != nil {
			return fmt.Errorf("resolve cache dir: %w", err)
		}
		if isValidTemplate(dir) {
			slog.Info("testapp: template cache hit", "key", key, "dir", dir)
			templateDir = dir
			return nil
		}
		slog.Info("testapp: template cache miss, building", "key", key, "dir", dir)
		return buildTemplate(dir)
	}()
}

// cacheDir returns the stable, hash-keyed directory a template for this key
// would live in, creating its parent if needed. It does not create the
// directory itself — that's the marker of "not built yet" isValidTemplate
// checks for.
func cacheDir(key string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "padelleague-testapp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(root, key), nil
}

// readyMarker names the file written last, after the template directory is
// fully populated. Its presence is what makes a cache dir valid: a directory
// without it is either not yet built or was left behind by a process that
// died mid-build.
const readyMarker = ".ready"

func isValidTemplate(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, readyMarker))
	return err == nil
}

// buildTemplate builds a fresh template and atomically publishes it at dir.
// Building happens in a private sibling temp directory, then os.Rename
// moves it into place — so concurrent go test processes (go test ./...
// runs package binaries in parallel, and gremlins runs multiple workers)
// that all miss the cache at once never observe a partially-written
// directory at dir. Go's os.Rename is atomic within the same filesystem,
// which cacheDir's shared root (one os.UserCacheDir() tree) guarantees.
//
// If this process loses the race, it discards its own build and reuses the
// winner's directory — the winner is exactly as valid, since the key
// guarantees byte-identical inputs.
func buildTemplate(dir string) error {
	seed, err := tests.NewTestApp()
	if err != nil {
		return err
	}
	defer seed.Cleanup()
	if err := lowerPasswordCost(seed); err != nil {
		return err
	}

	tmp := dir + ".build-" + fmt.Sprint(os.Getpid())
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.CopyFS(tmp, os.DirFS(seed.DataDir())); err != nil {
		removeBuildDir(tmp)
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, readyMarker), nil, 0o644); err != nil {
		removeBuildDir(tmp)
		return err
	}

	if err := os.Rename(tmp, dir); err != nil {
		// Lost the race to a concurrent builder (or dir already exists from
		// a prior run): discard our build and use whatever is there now,
		// as long as it's actually valid.
		removeBuildDir(tmp)
		if isValidTemplate(dir) {
			templateDir = dir
			return nil
		}
		return fmt.Errorf("rename template into place: %w", err)
	}
	templateDir = dir
	return nil
}

// removeBuildDir cleans up a failed or superseded build-in-progress
// directory. It's best-effort cleanup on an already-failing path: a
// leftover .build-<pid> dir is harmless (it's never treated as valid by
// isValidTemplate, since it never gets a readyMarker written under the
// final dir name), so a removal failure here is logged, not propagated —
// propagating it would mask the original, more relevant error.
func removeBuildDir(tmp string) {
	if err := os.RemoveAll(tmp); err != nil {
		slog.Warn("testapp: remove build-in-progress dir", "dir", tmp, "err", err)
	}
}

// templateKey hashes every input that shapes the built template: our own
// migrations (schema), go.mod/go.sum (the PocketBase version — its bundled
// system migrations and test fixture DB shape the template too — plus every
// other dependency version), and this file (the build logic itself, e.g.
// lowerPasswordCost). Any change to one of these invalidates the cache
// automatically; nothing here is a magic version number to remember to bump.
func templateKey() (string, error) {
	files, migrationsDir := keyInputs()
	return hashInputs(files, migrationsDir)
}

// keyInputs lists the files and the migrations directory templateKey hashes.
func keyInputs() (files []string, migrationsDir string) {
	return []string{repoPath("go.mod"), repoPath("go.sum"), selfPath()}, repoPath("migrations")
}

// hashInputs hashes files in order, then every *.go file under migrationsDir.
func hashInputs(files []string, migrationsDir string) (string, error) {
	h := sha256.New()
	for _, path := range files {
		if err := hashFile(h, path); err != nil {
			return "", err
		}
	}
	if err := hashDir(h, migrationsDir); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// hashDir hashes every *.go file directly under dir (migrations/ has no
// subpackages), in sorted order so the result doesn't depend on directory
// iteration order.
func hashDir(h io.Writer, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".go" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := hashFile(h, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func hashFile(h io.Writer, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = h.Write(b)
	return err
}

// selfPath returns the absolute path to this source file, so templateKey
// covers changes to buildTemplate's own logic.
func selfPath() string {
	_, file, _, _ := runtime.Caller(0)
	return file
}

// repoPath resolves a path relative to the module root. This package is at
// internal/testapp, so the module root is two directories up.
func repoPath(rel string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", rel)
}

// lowerPasswordCost sets every auth collection's bcrypt cost to the minimum:
// the default cost 10 spends ~90 ms per SetPassword, cost 4 under 2 ms.
func lowerPasswordCost(app core.App) error {
	cols, err := app.FindAllCollections(core.CollectionTypeAuth)
	if err != nil {
		return err
	}
	for _, col := range cols {
		field, ok := col.Fields.GetByName(core.FieldNamePassword).(*core.PasswordField)
		if !ok {
			return fmt.Errorf("collection %s has no password field", col.Name)
		}
		field.Cost = bcrypt.MinCost
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", col.Name, err)
		}
	}
	return nil
}
