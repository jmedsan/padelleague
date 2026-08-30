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
