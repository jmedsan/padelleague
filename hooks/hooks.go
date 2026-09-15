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

func handleAdvance(svc *league.Service, rec *core.Record) {
	if rec.GetString("status") != league.StatusFinal {
		return
	}
	if err := svc.AdvancePlayoff(rec); err != nil {
		slog.Error("auto-advance playoff failed", "match", rec.Id, "err", err)
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
	if comp.GetString("calendar_status") != "published" {
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

func checkMatchReminders(app core.App, notifier *notify.Notifier, now time.Time) {
	settings := league.LoadSettings(app)
	cutoff := now.Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05.000Z")
	matches, err := app.FindRecordsByFilter("matches",
		"status = {:status} && date >= {:cutoff}",
		"", 0, 0, map[string]any{"status": league.StatusScheduled, "cutoff": cutoff})
	if err != nil {
		slog.Error("match reminders: list matches", "err", err)
		return
	}

	rc := reminderCtx{app: app, notifier: notifier, settings: settings}
	compCache := map[string]*core.Record{}
	for _, m := range matches {
		rc.remindMatch(m, compCache, now)
	}
}

func resolveComp(app core.App, compID string, cache map[string]*core.Record) (*core.Record, bool) {
	comp, cached := cache[compID]
	if cached {
		return comp, comp.GetString("calendar_status") == "published"
	}
	var err error
	comp, err = app.FindRecordById("competitions", compID)
	if err != nil {
		return nil, false
	}
	cache[compID] = comp
	return comp, comp.GetString("calendar_status") == "published"
}

type reminderCtx struct {
	app      core.App
	notifier *notify.Notifier
	settings league.AppSettings
}

func (rc reminderCtx) remindMatch(m *core.Record, compCache map[string]*core.Record, now time.Time) {
	start, ok := league.MatchStart(m)
	if !ok {
		slog.Warn("match reminders: unparsable start", "match", m.Id)
		return
	}
	until := start.Sub(now)
	if until <= 0 {
		return
	}

	comp, ok := resolveComp(rc.app, m.GetString("competition"), compCache)
	if !ok {
		return
	}

	pair1ID := m.GetString("pair1")
	pair2ID := m.GetString("pair2")
	pairNames := league.PairNames(rc.app, []string{pair1ID, pair2ID})

	type side struct {
		players  []string
		opponent string
	}
	sides := []side{
		{league.PlayersForPair(rc.app, pair1ID), pairNames[pair2ID]},
		{league.PlayersForPair(rc.app, pair2ID), pairNames[pair1ID]},
	}

	venue := m.GetString("club")
	compName := comp.GetString("name")
	for _, s := range sides {
		rc.remindSide(remindSideArgs{
			match: m, comp: comp, venue: venue, compName: compName,
			start: start, until: until, players: s.players, opponent: s.opponent,
		})
	}
}

type remindSideArgs struct {
	match, comp     *core.Record
	venue, compName string
	start           time.Time
	until           time.Duration
	players         []string
	opponent        string
}

func (rc reminderCtx) remindSide(a remindSideArgs) {
	for _, uid := range a.players {
		user, err := rc.app.FindRecordById("users", uid)
		if err != nil {
			continue
		}
		hours := league.ReminderHours(user, a.comp, rc.settings)
		due := league.DueReminderHours(hours, a.until)
		if len(due) == 0 {
			continue
		}
		if !claimReminders(rc.app, a.match.Id, uid, due) {
			continue
		}
		rc.notifier.NotifyPlayers([]string{uid},
			league.NotifMatchUpcoming(league.MatchUpcomingParams{
				MatchID: a.match.Id, Start: a.start, Until: a.until,
				Venue: a.venue, CompName: a.compName, Opponent: a.opponent,
			}))
	}
}

func claimReminders(app core.App, matchID, userID string, due []int) bool {
	col, err := app.FindCollectionByNameOrId("match_reminders")
	if err != nil {
		slog.Error("match reminders: find collection", "err", err)
		return false
	}

	smallestClaimed := false
	smallest := due[len(due)-1]
	for _, h := range due {
		rec := core.NewRecord(col)
		rec.Set("match", matchID)
		rec.Set("user", userID)
		rec.Set("hours_before", h)
		if err := app.Save(rec); err != nil {
			continue
		}
		if h == smallest {
			smallestClaimed = true
		}
	}
	return smallestClaimed
}

// Deps holds the shared dependencies Register wires onto the app.
type Deps struct {
	Svc         *league.Service
	Notifier    *notify.Notifier
	SearchIndex *search.Index
	Backup      BackupConfig
	SMTP        SMTPConfig
}

// Register wires all PocketBase event hooks and cron jobs onto the given app.
func Register(app core.App, deps Deps) {
	svc, notifier, searchIndex := deps.Svc, deps.Notifier, deps.SearchIndex
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
		handleAdvance(svc, e.Record)
		return e.Next()
	})

	app.Cron().MustAdd("quorum-timeout", "*/5 * * * *", func() {
		svc.ConfirmStaleMatches()
	})

	app.Cron().MustAdd("scheduling-reminders", "0 9 * * *", func() {
		checkSchedulingReminders(app, notifier)
	})

	app.Cron().MustAdd("match-reminders", "*/5 * * * *", func() {
		checkMatchReminders(app, notifier, time.Now())
	})

	app.Cron().MustAdd("confirmation-reminders", "0 */6 * * *", func() {
		svc.RemindPendingConfirmations(time.Now())
	})

	app.Cron().MustAdd("pending-match-penalties", "0 1 * * *", func() {
		applyPendingMatchPenalties(app, notifier)
	})

	registerSearch(app, searchIndex)
	registerBackup(app, deps.Backup)
	registerSMTP(app, deps.SMTP)
	registerMailerBranding(app)
}

func registerSearch(app core.App, idx *search.Index) {
	if idx == nil {
		return
	}
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		idx.Rebuild(app)
		tz := league.Timezone(app)
		slog.Info("startup", "league_timezone", tz.String())
		return e.Next()
	})
	app.Cron().MustAdd("search-index-rebuild", "*/30 * * * *", func() {
		idx.Rebuild(app)
	})
	for _, collection := range []string{"users", "pairs", "competitions", "matches", "venues", "announcements"} {
		app.OnRecordAfterCreateSuccess(collection).BindFunc(func(e *core.RecordEvent) error {
			search.UpsertRecord(idx, app, e.Record.Collection().Name, e.Record)
			return e.Next()
		})
		app.OnRecordAfterUpdateSuccess(collection).BindFunc(func(e *core.RecordEvent) error {
			search.UpsertRecord(idx, app, e.Record.Collection().Name, e.Record)
			return e.Next()
		})
	}
	app.OnRecordAfterDeleteSuccess("announcements").BindFunc(func(e *core.RecordEvent) error {
		idx.Upsert(e.Record.Id, nil)
		return e.Next()
	})
}

func applyPendingMatchPenalties(app core.App, notifier *notify.Notifier) {
	now := time.Now()
	comps, err := app.FindRecordsByFilter("competitions",
		"active = true && finalized = false && end_date != ''",
		"", 0, 0, nil)
	if err != nil {
		slog.Error("pending-match-penalties: list competitions", "err", err)
		return
	}
	for _, comp := range comps {
		phase := league.CompetitionPhase(comp, now)
		if phase != league.PhaseRecovery && phase != league.PhaseFinished {
			continue
		}
		applied, err := league.ApplyPendingMatchPenalties(app, comp)
		if err != nil {
			slog.Error("pending-match-penalties: apply", "competition", comp.Id, "err", err)
			continue
		}
		if len(applied) == 0 {
			continue
		}
		slog.Info("pending-match-penalties: applied", "competition", comp.Id, "count", len(applied))
		compID := comp.Id
		for _, pen := range applied {
			players := league.PlayersForPair(app, pen.GetString("pair"))
			notifier.NotifyPlayers(players, league.Notification{
				Type:  "penalty",
				Title: "Penalización aplicada",
				Body:  fmt.Sprintf("%.0f puntos — %s", pen.GetFloat("amount"), pen.GetString("reason")),
				Link:  "/competition/" + compID,
			})
		}
		_ = notifier.NotifyAdmins(league.Notification{
			Type:  "penalty",
			Title: "Penalizaciones automáticas aplicadas",
			Body:  fmt.Sprintf("%d penalizaciones aplicadas en %s", len(applied), comp.GetString("name")),
			Link:  "/admin/competitions/" + compID,
		})
	}
}
