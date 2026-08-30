package league

import "testing"

func TestIsPreScore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{StatusPending, true},
		{StatusScheduled, true},
		{StatusConfirmed, false},
		{StatusDisputed, false},
		{StatusFinal, false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsPreScore(tc.status); got != tc.want {
			t.Errorf("IsPreScore(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
