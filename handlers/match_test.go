package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"padelleague/league"
)

func TestStatusLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   string
	}{
		{league.StatusPending, "Pendiente"},
		{league.StatusConfirmed, "Enviado — esperando confirmación"},
		{league.StatusDisputed, "En disputa"},
		{league.StatusFinal, "Finalizado"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, statusLabel(tt.status))
		})
	}
}

func TestStatusClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   string
	}{
		{league.StatusPending, "badge-warning"},
		{league.StatusConfirmed, "badge-info"},
		{league.StatusDisputed, "badge-error"},
		{league.StatusFinal, "badge-success"},
		{"unknown", "badge-ghost"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, statusClass(tt.status))
		})
	}
}
