package league

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	plannerPairCount = 6 // leveled needs target_matches below pairs-1
)

// plannerFixture is a competition of six pairs; withdrawn is always pair1 of the
// match under test unless the case asks otherwise.
type plannerFixture struct {
	app       core.App
	comp      *core.Record
	withdrawn *core.Record
	opponent  *core.Record
}

func newPlannerFixture(t *testing.T, kind string) plannerFixture {
	t.Helper()
	app := newTestApp(t)
	var pairs []*core.Record
	for i := range plannerPairCount {
		pairs = append(pairs, makePair(t, app, fmt.Sprintf("Planner %d", i)))
	}
	comp := makeCompetition(t, app, pairs)
	switch kind {
	case "leveled":
		comp.Set("target_matches", 3)
	case "playoff":
		comp.Set("type", "playoff")
	}
	require.NoError(t, app.Save(comp))
	return plannerFixture{app: app, comp: comp, withdrawn: pairs[0], opponent: pairs[1]}
}

func (f plannerFixture) match(t *testing.T, status, reviewType, scores string, admin bool, winner *core.Record) *core.Record {
	t.Helper()
	m := makeMatch(t, f.app, f.comp.Id, f.withdrawn.Id, f.opponent.Id, status)
	m.Set("review_type", reviewType)
	m.Set("scores", scores)
	m.Set("admin_result", admin)
	if winner != nil {
		m.Set("winner", winner.Id)
	}
	require.NoError(t, f.app.Save(m))
	return m
}

func TestIsVoid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     string
		withWinner bool
		want       bool
	}{
		{name: "final with no winner", status: StatusFinal, want: true},
		{name: "final with a winner", status: StatusFinal, withWinner: true, want: false},
		{name: "pending with no winner", status: StatusPending, want: false},
		{name: "scheduled with no winner", status: StatusScheduled, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPlannerFixture(t, "standard")
			var winner *core.Record
			if tc.withWinner {
				winner = f.withdrawn
			}
			m := f.match(t, tc.status, "", "", false, winner)
			assert.Equal(t, tc.want, IsVoid(m))
		})
	}
}

func TestOpponentOf(t *testing.T) {
	t.Parallel()
	f := newPlannerFixture(t, "standard")
	m := f.match(t, StatusPending, "", "", false, nil)

	assert.Equal(t, f.opponent.Id, OpponentOf(m, f.withdrawn.Id), "withdrawn pair is pair1")
	assert.Equal(t, f.withdrawn.Id, OpponentOf(m, f.opponent.Id), "withdrawn pair is pair2")
}
