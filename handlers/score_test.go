package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"padelleague/league"
)

func TestParseScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		score   string
		sets1   int
		sets2   int
		games1  int
		games2  int
		wantErr bool
	}{
		{"two-one", "6-3 4-6 7-5", 2, 1, 17, 14, false},
		{"straight sets", "6-3 6-4", 2, 0, 12, 7, false},
		{"opponent wins", "3-6 6-4 5-7", 1, 2, 14, 17, false},
		{"tiebreaks", "7-6(5) 6-7(3) 6-4", 2, 1, 19, 17, false},
		{"walkover", "WO", 0, 0, 0, 0, false},
		{"empty", "", 0, 0, 0, 0, true},
		{"invalid", "abc", 0, 0, 0, 0, true},
		{"tied set", "6-6 6-4", 0, 0, 0, 0, true},

		{"invalid set: 7-0", "7-0 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-1", "7-1 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-2", "7-2 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-3", "7-3 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-4", "7-4 6-3", 0, 0, 0, 0, true},
		{"invalid set: 0-7", "6-3 0-7", 0, 0, 0, 0, true},
		{"valid set: 7-5", "7-5 6-3", 2, 0, 13, 8, false},
		{"valid set: 7-6", "7-6 6-4", 2, 0, 13, 10, false},
		{"valid set: 5-7", "6-3 5-7 6-4", 2, 1, 17, 14, false},
		{"valid set: 6-7", "6-3 6-7 6-4", 2, 1, 18, 14, false},

		{"invalid set: 6-8", "6-8 6-3", 0, 0, 0, 0, true},
		{"invalid set: 8-6", "8-6 6-3", 0, 0, 0, 0, true},

		{"invalid: 1 set", "6-3", 0, 0, 0, 0, true},
		{"invalid: 4 sets", "6-3 3-6 6-4 6-2", 0, 0, 0, 0, true},

		{"invalid: winner has 3 sets", "6-3 6-4 6-2", 0, 0, 0, 0, true},

		{"invalid set: 6-5", "6-5 6-3", 0, 0, 0, 0, true},
		{"tied 7-7", "7-7 6-3", 0, 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := league.ParseScore(tt.score)
			if tt.wantErr {
				assert.Error(t, err, "ParseScore(%q) should error", tt.score)
				return
			}
			assert.NoError(t, err, "ParseScore(%q) should not error", tt.score)
			assert.Equal(t, tt.sets1, sc.Sets1, "sets1")
			assert.Equal(t, tt.sets2, sc.Sets2, "sets2")
			assert.Equal(t, tt.games1, sc.Games1, "games1")
			assert.Equal(t, tt.games2, sc.Games2, "games2")
		})
	}
}
