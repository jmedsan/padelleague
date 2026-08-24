// Package hooks registers PocketBase event hooks and cron jobs.
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
	league.StatusPending:   {league.StatusConfirmed, league.StatusFinal},
	league.StatusConfirmed: {league.StatusFinal, league.StatusDisputed},
	league.StatusDisputed:  {league.StatusFinal},
}

func validateTransition(oldStatus, newStatus string) error {
	if oldStatus == newStatus {
		return nil
	}
	allowed, ok := validTransitions[oldStatus]
	if !ok {
		return fmt.Errorf("invalid transition from %s", oldStatus)
	}
	if !slices.Contains(allowed, newStatus) {
		return fmt.Errorf("invalid transition: %s → %s", oldStatus, newStatus)
	}
	return nil
}

func handleAdvance(svc *league.Service, notifier *notify.Notifier, rec *core.Record) {
	if rec.GetString("status") != league.StatusFinal {
		return
	}
	if err := svc.AdvancePlayoff(rec); err != nil {
		slog.Error("auto-advance playoff failed", "match", rec.Id, "err", err)
		if notifier != nil {
			_ = notifier.NotifyAdmins("admin_message",
				"Error en avance de playoff",
				fmt.Sprintf("El partido %s finalizó pero el bracket no avanzó automáticamente. Error: %v", rec.Id, err),
				rec.Id)
		}
	}
}

// Register wires all PocketBase event hooks and cron jobs onto the given app.
func Register(app core.App, svc *league.Service, notifier *notify.Notifier) {
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("role") == "" {
			e.Record.Set("role", "player")
		}
		return e.Next()
	})

	app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
		old := e.Record.Original().GetString("status")
		if err := validateTransition(old, e.Record.GetString("status")); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("matches").BindFunc(func(e *core.RecordEvent) error {
		handleAdvance(svc, notifier, e.Record)
		return e.Next()
	})

	app.Cron().MustAdd("quorum-timeout", "*/5 * * * *", func() {
		svc.ConfirmStaleMatches()
	})
}
