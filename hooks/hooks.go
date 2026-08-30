// Package hooks registers PocketBase event hooks and cron jobs.
package hooks

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
	"padelleague/search"
)

var validTransitions = map[string][]string{
	league.StatusPending:   {league.StatusConfirmed, league.StatusFinal, league.StatusDisputed},
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
				"El partido finalizó pero el bracket no avanzó automáticamente. Revisa el panel de administración.",
				rec.Id)
		}
	}
}

func checkSchedulingReminders(app core.App, notifier *notify.Notifier) {
	comps, err := app.FindRecordsByFilter("competitions", "active = true", "", 0, 0, nil)
	if err != nil {
		slog.Error("scheduling reminders: list competitions", "err", err)
		return
	}

	now := time.Now()
	for _, comp := range comps {
		remindCompetitionMatches(app, notifier, comp, now)
	}
}

func remindCompetitionMatches(app core.App, notifier *notify.Notifier, comp *core.Record, now time.Time) {
	if league.CompetitionPhase(comp, now) == league.PhaseFinished {
		return
	}

	graceDays := comp.GetInt("arrange_grace_days")

	matches, err := app.FindRecordsByFilter("matches",
		"competition = {:comp} && status = 'pending'",
		"", 0, 0, map[string]any{"comp": comp.Id})
	if err != nil {
		slog.Error("scheduling reminders: list matches", "comp", comp.Id, "err", err)
		return
	}

	for _, m := range matches {
		deadline, ok := league.RoundArrangeDate(comp, m.GetInt("round_number"))
		if !ok {
			continue
		}

		level := league.WarningLevel(deadline, graceDays, now)
		if int(level) <= m.GetInt("last_warn_level") {
			continue
		}

		playerIDs := append(
			league.PlayersForPair(app, m.GetString("pair1")),
			league.PlayersForPair(app, m.GetString("pair2"))...,
		)

		notifier.NotifyPlayers(playerIDs, league.NotifSchedulingReminder(m.Id, level.Label()))

		m.Set("last_warn_level", int(level))
		if err := app.Save(m); err != nil {
			slog.Error("scheduling reminder save last_warn_level", "match", m.Id, "err", err)
		}
	}
}

// Register wires all PocketBase event hooks and cron jobs onto the given app.
func Register(app core.App, svc *league.Service, notifier *notify.Notifier, searchIndex *search.Index) {
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if len(e.Record.GetStringSlice("roles")) == 0 {
			e.Record.Set("roles", []string{"player"})
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

	app.Cron().MustAdd("scheduling-reminders", "0 9 * * *", func() {
		checkSchedulingReminders(app, notifier)
	})

	app.Cron().MustAdd("confirmation-reminders", "0 */6 * * *", func() {
		svc.RemindPendingConfirmations(time.Now())
	})

	if searchIndex != nil {
		app.OnServe().BindFunc(func(e *core.ServeEvent) error {
			searchIndex.Rebuild(app)
			return e.Next()
		})
		app.Cron().MustAdd("search-index-rebuild", "*/10 * * * *", func() {
			searchIndex.Rebuild(app)
		})
	}
}
