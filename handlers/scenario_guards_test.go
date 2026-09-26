package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFuncRe    = regexp.MustCompile(`(?ms)^func (Test\w+)\(t \*testing\.T\) \{\n.*?^\}\n`)
	noContentRe   = regexp.MustCompile(`ExpectedStatus:\s*204`)
	redirectCheck = []string{"HX-Redirect", "expectRedirect(", "ExpectRedirect("}
)

// TestNoContentScenariosPinTheRedirect fails when a scenario expecting 204
// does not assert the HX-Redirect target: every mutation answers 204 plus a
// redirect, and a test that only checks the status let an admin land on the
// list instead of the detail page (9236fe2) without failing.
func TestNoContentScenariosPinTheRedirect(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	require.NoError(t, err)
	var offenders []string
	for _, f := range files {
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range testFuncRe.FindAllStringSubmatch(string(src), -1) {
			body := m[0]
			if !noContentRe.MatchString(body) {
				continue
			}
			pinned := false
			for _, marker := range redirectCheck {
				if strings.Contains(body, marker) {
					pinned = true
				}
			}
			if !pinned {
				offenders = append(offenders, f+": "+m[1])
			}
		}
	}
	assert.Empty(t, offenders, "add expectRedirect(s, …) or handlers.ExpectRedirect(s, …) before s.Test(t)")
}
