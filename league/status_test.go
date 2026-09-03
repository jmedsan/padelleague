package league

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, IsPreScore(tc.status))
		})
	}
}

func TestStatusLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   string
	}{
		{StatusPending, "Pendiente"},
		{StatusScheduled, "Confirmada"},
		{StatusConfirmed, "Propuesta"},
		{StatusDisputed, "En disputa"},
		{StatusFinal, "Confirmado"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, StatusLabel(tc.status))
		})
	}
}
