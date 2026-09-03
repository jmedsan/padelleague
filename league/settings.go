package league

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// AppSettings holds the global default values for new competitions.
type AppSettings struct {
	QuorumTimeoutHours   int
	ArrangeGraceDays     int
	WalkoverScore        string
	DefaultPenalty       int
	RecoveryDays         int
	PlayTwice            bool
	GenderType           string
	InviteMaxUses        int
	InviteExpirationDays int
}

var (
	settingsCache *AppSettings
	settingsMu    sync.Mutex
)

// DefaultSettings returns hardcoded fallback values.
func DefaultSettings() AppSettings {
	return AppSettings{
		QuorumTimeoutHours:   48,
		ArrangeGraceDays:     3,
		WalkoverScore:        "6-0 6-0",
		DefaultPenalty:       3,
		RecoveryDays:         14,
		GenderType:           "free",
		InviteMaxUses:        10,
		InviteExpirationDays: 7,
	}
}

// LoadSettings reads the app_settings singleton, falling back to defaults.
func LoadSettings(app core.App) AppSettings {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if settingsCache != nil {
		return *settingsCache
	}

	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		d := DefaultSettings()
		settingsCache = &d
		return d
	}

	r := records[0]
	s := AppSettings{
		QuorumTimeoutHours:   int(r.GetFloat("quorum_timeout_hours")),
		ArrangeGraceDays:     int(r.GetFloat("arrange_grace_days")),
		WalkoverScore:        r.GetString("walkover_score"),
		DefaultPenalty:       int(r.GetFloat("default_penalty")),
		RecoveryDays:         int(r.GetFloat("recovery_days")),
		PlayTwice:            r.GetBool("play_twice"),
		GenderType:           r.GetString("gender_type"),
		InviteMaxUses:        int(r.GetFloat("invite_max_uses")),
		InviteExpirationDays: int(r.GetFloat("invite_expiration_days")),
	}
	if s.WalkoverScore == "" {
		s.WalkoverScore = "6-0 6-0"
	}
	if s.GenderType == "" {
		s.GenderType = "free"
	}
	settingsCache = &s
	return s
}

// InvalidateSettingsCache clears the cached settings so the next
// LoadSettings reads fresh from the database.
func InvalidateSettingsCache() {
	settingsMu.Lock()
	settingsCache = nil
	settingsMu.Unlock()
}
