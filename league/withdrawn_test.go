package league

import (
	"testing"

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
