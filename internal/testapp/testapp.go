// Package testapp hands tests a PocketBase app built from a pre-migrated
// template instead of re-running every migration per test.
//
// Callers must import padelleague/migrations for side effects so the
// template includes the app schema; this package does not, because the
// migrations package's own tests would otherwise form an import cycle.
package testapp

import (
	"fmt"
	"os"
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

// Run wraps m.Run so the template directory is removed when the package's
// tests finish. Use it from TestMain: os.Exit(testapp.Run(m)).
func Run(m *testing.M) int {
	code := m.Run()
	if templateDir != "" {
		if err := os.RemoveAll(templateDir); err != nil {
			fmt.Fprintln(os.Stderr, "testapp: remove template:", err)
		}
	}
	return code
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
	templateOnce.Do(buildTemplate)
	if templateErr != nil {
		tb.Fatalf("testapp: build template: %v", templateErr)
	}
	app, err := tests.NewTestApp(templateDir)
	if err != nil {
		tb.Fatalf("testapp: copy template: %v", err)
	}
	return app
}

func buildTemplate() {
	templateErr = func() error {
		seed, err := tests.NewTestApp()
		if err != nil {
			return err
		}
		defer seed.Cleanup()
		if err := lowerPasswordCost(seed); err != nil {
			return err
		}
		dir, err := os.MkdirTemp("", "pbtmpl-*")
		if err != nil {
			return err
		}
		// CopyFS needs the destination absent; MkdirTemp already created it.
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.CopyFS(dir, os.DirFS(seed.DataDir())); err != nil {
			return err
		}
		templateDir = dir
		return nil
	}()
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
