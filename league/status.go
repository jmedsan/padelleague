package league

import "fmt"

// RoundLabel returns the display label for a league round number.
// Round 0 (leveled-league assignments) returns "" — it has no round identity.
func RoundLabel(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("Jornada %d", n)
}

// Match status constants.
const (
	StatusPending   = "pending"
	StatusScheduled = "scheduled"
	StatusConfirmed = "confirmed"
	StatusDisputed  = "disputed"
	StatusFinal     = "final"
)

// IsPreScore returns true for statuses before any score has been submitted.
func IsPreScore(status string) bool {
	return status == StatusPending || status == StatusScheduled
}

// StatusLabel returns the Spanish display label for a match status,
// following the standard status vocabulary (Pendiente/Propuesta/
// Confirmada(o)/Rechazada(o)/En disputa/Arbitraje solicitado).
func StatusLabel(status string) string {
	switch status {
	case StatusPending:
		return "Pendiente"
	case StatusScheduled:
		return "Confirmada"
	case StatusConfirmed:
		return "Propuesta"
	case StatusDisputed:
		return "En disputa"
	case StatusFinal:
		return "Confirmado"
	}
	return status
}

// Arbitration category constants — a participant's open request for admin
// review (matches.arbitration). Empty means no request is open.
const (
	ArbitrationResult      = "result"
	ArbitrationScheduling  = "scheduling"
	ArbitrationAbandonment = "abandonment"
	ArbitrationNoShow      = "no_show"
	ArbitrationOther       = "other"
)

// ArbitrationLabel returns the Spanish display label for an arbitration
// category, "" when category is "" (no request open).
func ArbitrationLabel(category string) string {
	switch category {
	case ArbitrationResult:
		return "Resultado"
	case ArbitrationScheduling:
		return "Fecha y hora"
	case ArbitrationAbandonment:
		return "Abandono del rival"
	case ArbitrationNoShow:
		return "Incomparecencia"
	case ArbitrationOther:
		return "Otro"
	}
	return ""
}
