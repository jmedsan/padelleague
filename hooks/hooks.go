package hooks

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

var validTransitions = map[string][]string{
	"pending":   {"confirmed", "final"},
	"confirmed": {"final", "disputed"},
	"disputed":  {"final"},
}

func Register(app core.App, notifier *notify.Notifier) {
	svc := league.New(app, notifier)

	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("role") == "" {
			e.Record.Set("role", "player")
		}
		return e.Next()
	})

	app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
		oldStatus := e.Record.Original().GetString("status")
		newStatus := e.Record.GetString("status")
		if oldStatus != newStatus {
			allowed, ok := validTransitions[oldStatus]
			if !ok {
				return fmt.Errorf("invalid transition from %s", oldStatus)
			}
			if !slices.Contains(allowed, newStatus) {
				return fmt.Errorf("invalid transition: %s → %s", oldStatus, newStatus)
			}
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("matches").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "final" {
			if err := svc.AdvancePlayoff(e.Record); err != nil {
				slog.Error("auto-advance playoff failed", "match", e.Record.Id, "err", err)
			}
		}
		return e.Next()
	})

	app.Cron().MustAdd("quorum-timeout", "*/5 * * * *", func() {
		svc.ConfirmStaleMatches()
	})
}
