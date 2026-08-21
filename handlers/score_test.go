package handlers

import (
	"testing"
)

func TestParseScore(t *testing.T) {
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

		// Padel-specific validation: set scores must be valid
		// A set winner must have 6 or 7 games. 7 only valid as 7-5 or 7-6.
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

		// Loser can't have more than 6 games
		{"invalid set: 6-8", "6-8 6-3", 0, 0, 0, 0, true},
		{"invalid set: 8-6", "8-6 6-3", 0, 0, 0, 0, true},

		// Match must be best-of-3: exactly 2 or 3 sets
		{"invalid: 1 set", "6-3", 0, 0, 0, 0, true},
		{"invalid: 4 sets", "6-3 3-6 6-4 6-2", 0, 0, 0, 0, true},

		// Winner must have exactly 2 sets
		{"invalid: winner has 3 sets", "6-3 6-4 6-2", 0, 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1, s2, g1, g2, err := parseScore(tt.score)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScore(%q) error = %v, wantErr %v", tt.score, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if s1 != tt.sets1 || s2 != tt.sets2 || g1 != tt.games1 || g2 != tt.games2 {
				t.Errorf("parseScore(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.score, s1, s2, g1, g2, tt.sets1, tt.sets2, tt.games1, tt.games2)
			}
		})
	}
}
