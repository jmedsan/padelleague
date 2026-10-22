package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithdrawnSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		withdrawn []string
		want      map[string]bool
	}{
		{name: "no withdrawn pairs", withdrawn: nil, want: map[string]bool{}},
		{name: "one withdrawn pair", withdrawn: []string{"pair-a"}, want: map[string]bool{"pair-a": true}},
		{
			name:      "several withdrawn pairs",
			withdrawn: []string{"pair-a", "pair-c"},
			want:      map[string]bool{"pair-a": true, "pair-c": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			comp := makeCompetition(t, app, nil)
			comp.Set("withdrawn_pairs", tc.withdrawn)
			require.NoError(t, app.Save(comp))

			assert.Equal(t, tc.want, WithdrawnSet(comp))
		})
	}
}

func TestLoadWithdrawnIndex_ScopedPerCompetition(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	pairA := makePair(t, app, "Pareja A")
	pairB := makePair(t, app, "Pareja B")
	withdrawnComp := makeCompetition(t, app, []*core.Record{pairA, pairB})
	withdrawnComp.Set("withdrawn_pairs", []string{pairA.Id})
	require.NoError(t, app.Save(withdrawnComp))
	cleanComp := makeCompetition(t, app, []*core.Record{pairA, pairB})

	idx, err := LoadWithdrawnIndex(app)
	require.NoError(t, err)

	assert.True(t, idx.Has(withdrawnComp.Id, pairA.Id), "withdrawn pair in its competition")
	assert.False(t, idx.Has(withdrawnComp.Id, pairB.Id), "pair still active in its competition")
	assert.False(t, idx.Has(cleanComp.Id, pairA.Id), "withdrawal does not leak into another competition")
	assert.False(t, idx.Has("unknown-competition", pairA.Id), "unknown competition has no withdrawals")
}
