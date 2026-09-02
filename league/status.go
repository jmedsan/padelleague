package league

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
// Confirmada(o)/Rechazada(o)/En disputa).
func StatusLabel(status string) string {
	switch status {
	case StatusPending:
		return "Pendiente"
	case StatusScheduled:
		return "Confirmado"
	case StatusConfirmed:
		return "Propuesta"
	case StatusDisputed:
		return "En disputa"
	case StatusFinal:
		return "Confirmado"
	}
	return status
}
