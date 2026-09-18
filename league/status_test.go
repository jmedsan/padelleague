package league

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoundLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		round int
		want  string
	}{
		{0, ""},
		{1, "Jornada 1"},
		{5, "Jornada 5"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("round=%d", tc.round), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, RoundLabel(tc.round))
		})
	}
}

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
