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
	league.StatusPending:   {league.StatusScheduled, league.StatusConfirmed, league.StatusFinal, league.StatusDisputed},
	league.StatusScheduled: {league.StatusPending, league.StatusConfirmed, league.StatusFinal, league.StatusDisputed},
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
			n := league.NotifAdminPlayoffAdvanceFailed(rec.Id)
			_ = notifier.NotifyAdmins(n)
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

		pair1ID, pair2ID := m.GetString("pair1"), m.GetString("pair2")
		pairNames := league.PairNames(app, []string{pair1ID, pair2ID})
		compName := comp.GetString("name")

		notifier.NotifyPlayers(league.PlayersForPair(app, pair1ID),
			league.NotifSchedulingReminder(league.SchedulingReminderParams{
				MatchID: m.Id, Opponent: pairNames[pair2ID], CompName: compName, Level: level, Deadline: deadline,
			}))
		notifier.NotifyPlayers(league.PlayersForPair(app, pair2ID),
			league.NotifSchedulingReminder(league.SchedulingReminderParams{
				MatchID: m.Id, Opponent: pairNames[pair1ID], CompName: compName, Level: level, Deadline: deadline,
			}))

		m.Set("last_warn_level", int(level))
		if err := app.Save(m); err != nil {
			slog.Error("scheduling reminder save last_warn_level", "match", m.Id, "err", err)
		}
	}
}

// checkMatchDayReminders sends a "your match is tomorrow" notification to
// both pairs of every match with a confirmed date (status = scheduled)
// falling on the next calendar day, at most once per match.
func checkMatchDayReminders(app core.App, notifier *notify.Notifier, now time.Time) {
	tomorrow := now.AddDate(0, 0, 1)
	matches, err := app.FindRecordsByFilter("matches",
		"status = {:status} && reminder_sent != true",
		"", 0, 0, map[string]any{"status": league.StatusScheduled})
	if err != nil {
		slog.Error("match day reminders: list matches", "err", err)
		return
	}

	for _, m := range matches {
		d := m.GetDateTime("date").Time()
		if d.IsZero() || !sameDay(d, tomorrow) {
			continue
		}

		notif := league.NotifMatchReminder(m.Id, m.GetString("time"), m.GetString("club"))
		notifier.NotifyPlayers(league.PlayersForPair(app, m.GetString("pair1")), notif)
		notifier.NotifyPlayers(league.PlayersForPair(app, m.GetString("pair2")), notif)

		m.Set("reminder_sent", true)
		if err := app.Save(m); err != nil {
			slog.Error("match day reminder save reminder_sent", "match", m.Id, "err", err)
		}
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
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
		checkMatchDayReminders(app, notifier, time.Now())
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
