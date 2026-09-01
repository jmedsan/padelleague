package league

import "testing"

func TestValidatePairComposition(t *testing.T) {
	tests := []struct {
		name       string
		genderType string
		g1, g2     string
		wantErr    string
	}{
		{"free allows anything", "free", "male", "female", ""},
		{"free allows blank", "free", "", "", ""},

		{"male accepts MM", "male", "male", "male", ""},
		{"male rejects MF", "male", "male", "female", "solo masculina"},
		{"male rejects FF", "male", "female", "female", "solo masculina"},
		{"male rejects blank g1", "male", "", "male", "género asignado"},

		{"female accepts FF", "female", "female", "female", ""},
		{"female rejects MM", "female", "male", "male", "solo femenina"},
		{"female rejects MF", "female", "male", "female", "solo femenina"},
		{"female rejects blank g2", "female", "female", "", "género asignado"},

		{"mixed accepts MF", "mixed", "male", "female", ""},
		{"mixed accepts FM", "mixed", "female", "male", ""},
		{"mixed rejects MM", "mixed", "male", "male", "un jugador y una jugadora"},
		{"mixed rejects FF", "mixed", "female", "female", "un jugador y una jugadora"},
		{"mixed rejects blank", "mixed", "male", "", "género asignado"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePairComposition(tt.genderType, tt.g1, tt.g2)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Fatalf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
