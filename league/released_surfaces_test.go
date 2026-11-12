package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releasedWithProposal builds a released match that still carries a pending
// result proposal, the shape a released scheduled/proposed match has.
func releasedWithProposal(t *testing.T, app core.App) (comp, m *core.Record, p1, p2 *core.Record) {
	t.Helper()
	p1 = makePair(t, app, "RelSurfA")
	p2 = makePair(t, app, "RelSurfB")
	comp = makeCompetition(t, app, []*core.Record{p1, p2})
	m = makeMatch(t, app, comp.Id, p1.Id, p2.Id, StatusReleased)
	makeProposal(t, app, m.Id, p1.GetString("player1"), "6-3 6-4")
	return comp, m, p1, p2
}

func TestReleasedMatch_NotInStandings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp, _, _, _ := releasedWithProposal(t, app)

	rows, err := New(app, nil).ComputeStandings(comp.Id)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Zero(t, r.Played)
		assert.Zero(t, r.Points)
		assert.False(t, r.HasProvisional)
	}
}

func TestReleasedMatch_NotProvisional(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, m, _, _ := releasedWithProposal(t, app)

	got, err := ProvisionalResults(app, []*core.Record{m})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestReleasedMatch_NotInStats(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, _, p1, _ := releasedWithProposal(t, app)

	s := New(app, nil).Summarize([]string{p1.Id})
	assert.Zero(t, s.TotalPlayed)
	assert.Zero(t, s.Wins)
}

func TestReleasedMatch_NotUnplayed(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	comp, _, p1, p2 := releasedWithProposal(t, app)

	counts, err := unplayedCounts(app, comp, PhasePlaying)
	require.NoError(t, err)
	assert.Zero(t, counts[p1.Id])
	assert.Zero(t, counts[p2.Id])
}

func TestReleasedMatch_NotInPlayerStatPairs(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, m, p1, p2 := releasedWithProposal(t, app)
	// A player outside both current pairs whose lineup was frozen on the match.
	outsider := makeUser(t, app, "RelOut", "relsurf-out@example.com")
	m.Set("lineup1", []string{outsider.Id})
	require.NoError(t, app.Save(m))

	ids := PlayerStatPairIDs(app, outsider.Id)
	assert.NotContains(t, ids, p1.Id)
	assert.NotContains(t, ids, p2.Id)
}
