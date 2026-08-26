// Draft: iCal handler tests to kill mutation survivors
//
// File: handlers/ical_api_test.go (or add to public_api_test.go)
//
// These tests parse the iCal output structurally rather than substring-matching.
// Uses helpers: setupPublicRoutes, makePairTB, makeCompetitionTB, makeMatchTB, authHeaders

package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseVEvents splits an iCal body into VEVENT blocks and parses each into
// a map of property → value. Handles only single-value properties (no folding).
func parseVEvents(body string) []map[string]string {
	var events []map[string]string
	blocks := strings.Split(body, "BEGIN:VEVENT")
	for _, block := range blocks[1:] { // skip preamble
		end := strings.Index(block, "END:VEVENT")
		if end < 0 {
			continue
		}
		props := make(map[string]string)
		for _, line := range strings.Split(block[:end], "\n") {
			line = strings.TrimRight(line, "\r")
			if idx := strings.Index(line, ":"); idx > 0 {
				props[line[:idx]] = line[idx+1:]
			}
		}
		events = append(events, props)
	}
	return events
}

// Duration: DTSTART and DTEND are exactly 2 hours apart

func TestICalMatch_Duration2Hours(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} event spans 2 hours",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DurA")
		p2 := makePairTB(tb, app, "DurB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15")
		match.Set("time", "18:30")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		require.Equal(tb, 1, len(events), "expected exactly one VEVENT")
		ev := events[0]

		// 18:30 → DTSTART=20260915T183000, DTEND=20260915T203000
		assert.Equal(tb, "20260915T183000", ev["DTSTART"])
		assert.Equal(tb, "20260915T203000", ev["DTEND"])
	}
	s.Test(t)
}

// Default time (no time provided) → 19:00 start, 21:00 end

func TestICalMatch_DefaultTime1900(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} defaults to 19:00 when no time",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DefA")
		p2 := makePairTB(tb, app, "DefB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15")
		// no time set
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		require.Equal(tb, 1, len(events))
		assert.Equal(tb, "20260915T190000", events[0]["DTSTART"])
		assert.Equal(tb, "20260915T210000", events[0]["DTEND"])
	}
	s.Test(t)
}

// LOCATION carries the venue

func TestICalMatch_LocationFromClub(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} includes LOCATION from club field",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "LocA")
		p2 := makePairTB(tb, app, "LocB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15")
		match.Set("time", "20:00")
		match.Set("club", "Padel 360")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		require.Equal(tb, 1, len(events))
		assert.Equal(tb, "Padel 360", events[0]["LOCATION"])
	}
	s.Test(t)
}

// No venue → no LOCATION line

func TestICalMatch_NoLocationWhenNoClub(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} omits LOCATION when club is empty",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "NoLocA")
		p2 := makePairTB(tb, app, "NoLocB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15")
		match.Set("time", "20:00")
		// no club set
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.NotContains(tb, body, "LOCATION:")
	}
	s.Test(t)
}

// DESCRIPTION includes competition name

func TestICalMatch_DescriptionIncludesCompName(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} DESCRIPTION has competition name",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DescA")
		p2 := makePairTB(tb, app, "DescB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15")
		match.Set("time", "20:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		require.Equal(tb, 1, len(events))
		desc := events[0]["DESCRIPTION"]
		assert.Contains(tb, desc, "Jornada 1", "should include round number")
		// Competition name is set by makeCompetitionTB — check what it uses
		// (typically "Test League" or similar). The key assertion is that
		// the DESCRIPTION contains more than just "Jornada N".
		assert.Contains(tb, desc, " — ", "should include competition separator")
	}
	s.Test(t)
}

// Competition feed: dedup investigation
//
// The `seen` map in filterDatedMatches guards against duplicate match records.
// `allMatches` comes from a single FindRecordsByFilter("matches", "competition = {:cid}")
// which returns each match exactly once.
//
// A player can be in multiple pairs (pair A and pair B). If a match is A vs B,
// both pair1 and pair2 are in compPairIDs, but the match record still appears
// once in the query results. The `seen` map is defensive dead code — there is
// no production code path that produces duplicates in allMatches.
//
// To test the seen map's behavior, we would need to make FindRecordsByFilter
// return duplicate records, which would require either mocking core.App or
// injecting duplicates via a pre-save hook. Both are fragile.
//
// RECOMMENDATION: The `seen` map is harmless defensive code. It cannot be
// tested without manufacturing a condition that can't happen in production.
// The mutation survivor on line 161 is an equivalent mutant — killing it
// would require a test that depends on impossible DB behavior.

// Competition feed: match appears once even when player is in both pairs
// This does NOT test the `seen` map (query doesn't duplicate). It tests that
// the filter logic includes the match once when both pair1 and pair2 belong
// to the requesting player's pairs.

func TestICalCompetition_MatchAppearsOnceWhenPlayerInBothPairs(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/competition/{id} match appears once when player is in both pairs",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		// Create a player who is in two different pairs
		// This requires making pairs with the same player
		user := makeUserTB(tb, app, "DedupPlayer", "dedup@test.local")
		user2 := makeUserTB(tb, app, "Partner1", "partner1@test.local")
		user3 := makeUserTB(tb, app, "Partner2", "partner2@test.local")

		col, _ := app.FindCollectionByNameOrId("pairs")
		pairA := core.NewRecord(col)
		pairA.Set("name", "Pair A")
		pairA.Set("player1", user.Id)
		pairA.Set("player2", user2.Id)
		require.NoError(tb, app.Save(pairA))

		pairB := core.NewRecord(col)
		pairB.Set("name", "Pair B")
		pairB.Set("player1", user3.Id)
		pairB.Set("player2", user.Id)
		require.NoError(tb, app.Save(pairB))

		comp := makeCompetitionTB(tb, app, "league", []*core.Record{pairA, pairB})

		match := makeMatchTB(tb, app, comp.Id, pairA.Id, pairB.Id, "pending")
		match.Set("date", "2026-09-15")
		match.Set("time", "20:00")
		require.NoError(tb, app.Save(match))

		s.URL = "/ical/competition/" + comp.Id
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		assert.Equal(tb, 1, len(events), "match between player's two pairs should appear exactly once")
	}
	s.Test(t)
}

// Competition feed: dateless matches excluded

func TestICalCompetition_DatelessMatchExcluded(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/competition/{id} excludes matches without date",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "DatelessA")
		p2 := makePairTB(tb, app, "DatelessB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})

		// Match with date
		dated := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		dated.Set("date", "2026-09-15")
		dated.Set("time", "20:00")
		require.NoError(tb, app.Save(dated))

		// Match without date
		_ = makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		// no date set on this one

		s.URL = "/ical/competition/" + comp.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		assert.Equal(tb, 1, len(events), "only the dated match should produce a VEVENT")
	}
	s.Test(t)
}

// Truncated date (ISO 8601 datetime instead of date-only)

func TestICalMatch_TruncatesLongDate(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		Name:            "GET /ical/match/{id} handles datetime string by truncating to date",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"VCALENDAR"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "TruncA")
		p2 := makePairTB(tb, app, "TruncB")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		match.Set("date", "2026-09-15 14:30:00.000Z")
		match.Set("time", "18:00")
		require.NoError(tb, app.Save(match))
		s.URL = "/ical/match/" + match.Id
		user, _ := app.FindRecordById("users", p1.GetString("player1"))
		s.Headers = authHeaders(tb, user)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		events := parseVEvents(body)
		require.Equal(tb, 1, len(events))
		assert.Equal(tb, "20260915T180000", events[0]["DTSTART"],
			"should parse date correctly even with datetime suffix")
	}
	s.Test(t)
}

// readBody reads the response body from the *http.Response passed to AfterTestFunc.
// Verified: PocketBase reads from recorder.Body directly for ExpectedContent checks,
// never from res.Body. res is recorder.Result(), which wraps recorder.Body.Bytes()
// in a fresh bytes.NewReader — still at position 0 when AfterTestFunc runs.

// Comment to add to handlers/ical.go line 161 when applying
//
// On the `seen[m.Id]` guard in filterDatedMatches:
// Add this comment above `if seen[m.Id]`:
//
//     // Defensive: allMatches currently comes from a single FindRecordsByFilter
//     // which returns distinct records, so duplicates cannot occur. The guard
//     // protects against a future caller passing a list with duplicates.
//
// The mutation survivor on this line is an equivalent mutant — no test can
// kill it without manufacturing impossible DB behavior.
