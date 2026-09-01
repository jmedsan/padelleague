package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cardActions struct {
	submit   bool
	edit     bool
	walkover bool
	correct  bool
}

func TestNewMatchCardActions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePairTB(t, app, "Card A")
	p2 := makePairTB(t, app, "Card B")
	outsider := makeUserTB(t, app, "Card Outsider", "")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	cases := []struct {
		name      string
		mode      Mode
		status    string
		viewerID  string
		submitted bool
		recent    bool
		want      cardActions
	}{
		{
			name:     "player pending participant can submit edit and report walkover",
			mode:     PlayerFull,
			status:   "pending",
			viewerID: p1.GetString("player1"),
			want:     cardActions{submit: true, edit: true, walkover: true},
		},
		{
			name:      "player confirmed submitter can correct and report walkover",
			mode:      PlayerFull,
			status:    "confirmed",
			viewerID:  p1.GetString("player1"),
			submitted: true,
			recent:    true,
			want:      cardActions{walkover: true, correct: true},
		},
		{
			name:      "player confirmed opponent can report walkover",
			mode:      PlayerFull,
			status:    "confirmed",
			viewerID:  p2.GetString("player1"),
			submitted: true,
			want:      cardActions{walkover: true},
		},
		{
			name:     "admin summary has no player actions",
			mode:     AdminSummary,
			status:   "pending",
			viewerID: p1.GetString("player1"),
			want:     cardActions{},
		},
		{
			name:      "admin full has no player actions",
			mode:      AdminFull,
			status:    "confirmed",
			viewerID:  p2.GetString("player1"),
			submitted: true,
			want:      cardActions{},
		},
		{
			name:     "player outsider has no actions",
			mode:     PlayerFull,
			status:   "pending",
			viewerID: outsider.Id,
			want:     cardActions{},
		},
		{
			// A non-participant (team == 0) on a confirmed match must get no
			// confirm/dispute/correct — guards the `team > 0` boundary on the
			// confirmed path that a pending-only outsider case cannot reach.
			name:      "player outsider on confirmed match has no actions",
			mode:      PlayerFull,
			status:    "confirmed",
			viewerID:  outsider.Id,
			submitted: true,
			recent:    true,
			want:      cardActions{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match.Set("status", tc.status)
			match.Set("submitted_by", "")
			match.Set("submitted_at", "")
			if tc.submitted {
				match.Set("submitted_by", p1.GetString("player1"))
				if tc.recent {
					match.SetRaw("submitted_at", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
				}
			}
			require.NoError(t, app.Save(match))

			card := NewMatchCard(app, match, tc.mode, tc.viewerID)
			assert.Equal(t, tc.want, cardActions{
				submit:   card.CanSubmit,
				edit:     card.CanEdit,
				walkover: card.CanWalkover,
				correct:  card.CanCorrect,
			})
		})
	}
}

func TestMatchCardPlayerModeHidesAdminControls(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "player match card has no admin controls",
		Method:             http.MethodGet,
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"Player Card A", "Player Card B"},
		NotExpectedContent: []string{"Resolver", "Aprobar incomparecencia", "Corrección de administrador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Player Card A")
		p2 := makePairTB(tb, app, "Player Card B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", p1.GetString("player1"))
		match.Set("disputed_by", p2.GetString("player1"))
		match.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(match))
		s.URL = "/match/" + match.Id
		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestMatchCardAdminSummaryIsReadOnly(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     testAppFactory,
		Name:               "admin summary match card is read-only",
		Method:             http.MethodGet,
		URL:                "/",
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"Summary League", "Summary A", "Summary B", "Ver partido completo"},
		NotExpectedContent: []string{"Resolver", "Aprobar incomparecencia", "Corrección de administrador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupPublicRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Summary A")
		p2 := makePairTB(tb, app, "Summary B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		comp.Set("name", "Summary League")
		require.NoError(tb, app.Save(comp))
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", p1.GetString("player1"))
		match.Set("disputed_by", p2.GetString("player1"))
		match.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(match))
		s.Headers = authHeaders(tb, makeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestMatchCardAdminFullShowsScoresAndResolveEndpoint(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "admin thread panel shows both scores and resolve endpoint",
		Method:          http.MethodGet,
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"Marcador final", "Full A", "Full B"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Full A")
		p2 := makePairTB(tb, app, "Full B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
		match.Set("scores", "6-3 6-4")
		match.Set("submitted_by", p1.GetString("player1"))
		match.Set("disputed_by", p2.GetString("player1"))
		match.Set("disputed_scores", "6-4 6-3")
		require.NoError(tb, app.Save(match))
		// The single result panel (with the resolve endpoint) is in the thread fragment.
		s.URL = "/match/" + match.Id + "/thread"
		s.ExpectedContent = append(s.ExpectedContent, `hx-post="/admin/disputes/`+match.Id+`/resolve"`)
		s.Headers = authHeaders(tb, makeAdminUserTB(tb, app))
	}
	s.Test(t)
}

func TestMatchCardCrossRoleLeakGuard(t *testing.T) {
	t.Parallel()

	t.Run("PlayerFull disputed match has no admin forms", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory:  testAppFactory,
			Name:            "player full has no admin resolve or walkover-approve or override forms",
			Method:          http.MethodGet,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{"En disputa", "Leak Guard A"},
			NotExpectedContent: []string{
				"Resolver",
				"Aprobar incomparecencia",
				"Corrección de administrador",
				`hx-post="/admin/disputes/`,
				`admin-override`,
			},
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAllRoutes(tb, app, e)
			p1 := makePairTB(tb, app, "Leak Guard A")
			p2 := makePairTB(tb, app, "Leak Guard B")
			comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
			match.Set("scores", "6-3 6-4")
			match.Set("submitted_by", p1.GetString("player1"))
			match.Set("disputed_by", p2.GetString("player1"))
			match.Set("disputed_scores", "6-4 6-3")
			require.NoError(tb, app.Save(match))
			s.URL = "/match/" + match.Id
			player, err := app.FindRecordById("users", p2.GetString("player1"))
			require.NoError(tb, err)
			s.Headers = authHeaders(tb, player)
		}
		s.Test(t)
	})

	t.Run("AdminFull disputed match has no player submit or confirm forms", func(t *testing.T) {
		t.Parallel()
		s := &tests.ApiScenario{
			TestAppFactory:  testAppFactory,
			Name:            "admin full has no player submit or confirm forms",
			Method:          http.MethodGet,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{"Resolver", "Marcador final"},
			NotExpectedContent: []string{
				"Registrar resultado",
				"Confirmar",
				"Corregir marcador",
				"Reportar partido no jugado",
			},
		}
		s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAllRoutes(tb, app, e)
			p1 := makePairTB(tb, app, "Leak Admin A")
			p2 := makePairTB(tb, app, "Leak Admin B")
			comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
			match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "disputed")
			match.Set("scores", "6-3 6-4")
			match.Set("submitted_by", p1.GetString("player1"))
			match.Set("disputed_by", p2.GetString("player1"))
			match.Set("disputed_scores", "6-4 6-3")
			require.NoError(tb, app.Save(match))
			// The single result panel (resolve for admin) is in the thread fragment.
			s.URL = "/match/" + match.Id + "/thread"
			s.Headers = authHeaders(tb, makeAdminUserTB(tb, app))
		}
		s.Test(t)
	})
}

func TestMatchRowRendersOuterAnchorNoInnerLinks(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "competition fixture row is an outer anchor with no inner links",
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			"Row Pair A",
			"Row Pair B",
			"Pendiente",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAllRoutes(tb, app, e)
		p1 := makePairTB(tb, app, "Row Pair A")
		p2 := makePairTB(tb, app, "Row Pair B")
		comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
		makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, "pending")
		s.URL = "/competition/" + comp.Id
		player, err := app.FindRecordById("users", p1.GetString("player1"))
		require.NoError(tb, err)
		s.Headers = authHeaders(tb, player)
	}
	s.Test(t)
}

func TestNewMatchRowFields(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePairTB(t, app, "MR A")
	p2 := makePairTB(t, app, "MR B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	pairNames := map[string]string{p1.Id: "MR A", p2.Id: "MR B"}
	playerPairIDs := map[string]struct{}{p1.Id: {}}

	mc := NewMatchRow(match, pairNames, playerPairIDs)
	assert.Equal(t, PlayerRow, mc.Mode)
	assert.Equal(t, "MR A", mc.Pair1Name)
	assert.Equal(t, "MR B", mc.Pair2Name)
	assert.True(t, mc.IsMyMatch)
	assert.Equal(t, "Pendiente", mc.StatusLabel)
	assert.False(t, mc.CanSubmit)
}

func TestPopulateFeederOnMatchCard(t *testing.T) {
	t.Parallel()
	mc := MatchCard{}
	mc.PopulateFeeder(1, 0)
	assert.Equal(t, "Ganador de J1-1", mc.Feeder1)
	assert.Equal(t, "Ganador de J1-2", mc.Feeder2)

	mc2 := MatchCard{Pair1Name: "A"}
	mc2.PopulateFeeder(1, 0)
	assert.Empty(t, mc2.Feeder1)
	assert.Equal(t, "Ganador de J1-2", mc2.Feeder2)

	// matchIdx > 0 so the *2 in matchIdx*2+n is distinguishable from /2
	// (at matchIdx 0 both yield 0): idx 3 → feeders 7 and 8.
	mc3 := MatchCard{}
	mc3.PopulateFeeder(1, 3)
	assert.Equal(t, "Ganador de J1-7", mc3.Feeder1)
	assert.Equal(t, "Ganador de J1-8", mc3.Feeder2)
}

func TestPairPlayerLabel(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "Label A")
	p2 := makePairTB(t, app, "Label B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	t.Run("team 1 player", func(t *testing.T) {
		got := pairPlayerLabel(app, p1.GetString("player1"), match)
		assert.Equal(t, "Label A P1 (Label A)", got)
	})

	t.Run("team 2 player", func(t *testing.T) {
		got := pairPlayerLabel(app, p2.GetString("player1"), match)
		assert.Equal(t, "Label B P1 (Label B)", got)
	})

	t.Run("empty user ID", func(t *testing.T) {
		assert.Equal(t, "", pairPlayerLabel(app, "", match))
	})

	t.Run("admin non-participant returns bare name", func(t *testing.T) {
		admin := makeAdminUserTB(t, app)
		got := pairPlayerLabel(app, admin.Id, match)
		assert.Equal(t, "Admin", got)
	})
}
