package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{StatusPending, "Pendiente"},
		{StatusConfirmed, "Enviado — esperando confirmación"},
		{StatusDisputed, "En disputa"},
		{StatusFinal, "Finalizado"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, statusLabel(tt.status))
		})
	}
}

func TestStatusClass(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{StatusPending, "badge-warning"},
		{StatusConfirmed, "badge-info"},
		{StatusDisputed, "badge-error"},
		{StatusFinal, "badge-success"},
		{"unknown", "badge-ghost"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, statusClass(tt.status))
		})
	}
}
