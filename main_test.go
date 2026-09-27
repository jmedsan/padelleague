package main

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newTestPB(t *testing.T, args ...string) *pocketbase.PocketBase {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	app.RootCmd.SetArgs(args)
	return app
}

// A failing subcommand (as serve fails on "address already in use") must
// surface as execute's error, so main exits non-zero; pocketbase.Execute
// alone returns nil for it.
func TestExecute_ReturnsSubcommandError(t *testing.T) {
	app := newTestPB(t, "parent", "boom")
	want := errors.New("bind: address already in use")
	parent := &cobra.Command{Use: "parent"}
	parent.AddCommand(&cobra.Command{Use: "boom", SilenceUsage: true, RunE: func(*cobra.Command, []string) error { return want }})
	app.RootCmd.AddCommand(parent)

	require.ErrorIs(t, execute(app), want)
}

func TestExecute_NilWhenSubcommandSucceeds(t *testing.T) {
	app := newTestPB(t, "ok")
	ran := false
	app.RootCmd.AddCommand(&cobra.Command{Use: "ok", RunE: func(*cobra.Command, []string) error { ran = true; return nil }})

	require.NoError(t, execute(app))
	require.True(t, ran)
}

// start registers PocketBase's own system commands, as app.Start does.
func TestStart_RegistersSystemCommands(t *testing.T) {
	app := newTestPB(t, "help")
	require.NoError(t, start(app))
	var names []string
	for _, c := range app.RootCmd.Commands() {
		names = append(names, c.Name())
	}
	require.Contains(t, names, "serve")
	require.Contains(t, names, "superuser")
}
