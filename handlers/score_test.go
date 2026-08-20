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
