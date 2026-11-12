package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"padelleague/league"
)

func TestEffectiveStatusBadge_Released(t *testing.T) {
	t.Parallel()
	// Released wins over void and arbitration.
	label, class := effectiveStatusBadge(league.StatusReleased, "open", true)
	assert.Equal(t, "Liberado", label)
	assert.Equal(t, "badge-ghost", class)
}

func TestReleaseConfirmText_ByStatus(t *testing.T) {
	t.Parallel()
	pre := ReleaseConfirmText("pending")
	scored := ReleaseConfirmText(league.StatusFinal)
	assert.Equal(t, "¿Liberar este partido? Las parejas podrán ser emparejadas de nuevo.", pre)
	assert.Equal(t, "¿Liberar este partido? El resultado dejará de contar en la clasificación y las estadísticas. Las parejas podrán ser emparejadas de nuevo.", scored)
}

func TestCanRequestArbitration_ReleasedRefused(t *testing.T) {
	t.Parallel()
	assert.False(t, canRequestArbitration(league.StatusReleased, 1, ""))
}
