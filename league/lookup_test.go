package league

import "testing"

func TestEntityURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind, id, want string
	}{
		{"player", "abc", "/player/abc"},
		{"competition", "xyz", "/competition/xyz"},
		{"match", "m1", "/match/m1"},
		{"pair", "p1", "/pair/p1"},
		{"unknown", "x", "#"},
		{"player", "", "#"},
		{"", "", "#"},
	}
	for _, tt := range tests {
		if got := EntityURL(tt.kind, tt.id); got != tt.want {
			t.Errorf("EntityURL(%q, %q) = %q, want %q", tt.kind, tt.id, got, tt.want)
		}
	}
}
