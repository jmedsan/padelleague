package league

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// ConfirmStaleMatches auto-finalizes matches past the quorum timeout.
func (svc *Service) ConfirmStaleMatches() {
	stale, err := svc.app.FindRecordsByFilter("matches",
		"status = 'confirmed'", "", 0, 0, nil)
	if err != nil || len(stale) == 0 {
		return
	}

	compCache := map[string]*core.Record{}
	for _, m := range stale {
		compID := m.GetString("competition")
		if _, ok := compCache[compID]; !ok {
			comp, err := svc.app.FindRecordById("competitions", compID)
			if err != nil {
				compCache[compID] = nil
				continue
			}
			compCache[compID] = comp
		}
	}

	for _, m := range stale {
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}
		svc.confirmIfExpired(m, comp)
	}
}

func (svc *Service) confirmIfExpired(m *core.Record, comp *core.Record) {
	timeoutHours := int(comp.GetFloat("quorum_timeout_hours"))
	if timeoutHours == 0 {
		return
	}
	submittedAt := m.GetString("submitted_at")
	if submittedAt == "" {
		return
	}
	dt, err := types.ParseDateTime(submittedAt)
	if err != nil {
		return
	}
	if time.Since(dt.Time()) < time.Duration(timeoutHours)*time.Hour {
		return
	}

	fresh, err := svc.app.FindRecordById("matches", m.Id)
	if err != nil || fresh.GetString("status") != "confirmed" {
		return
	}
	winnerID, err := DetermineWinner(fresh, fresh.GetString("scores"))
	if err != nil {
		return
	}

	fresh.Set("status", "final")
	fresh.Set("winner", winnerID)
	fresh.Set("confirmed_by", "")
	fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
	if err := svc.app.Save(fresh); err != nil {
		slog.Error("save stale match confirmation", "match", m.Id, "err", err)
		return
	}

	for _, pid := range []string{fresh.GetString("pair1"), fresh.GetString("pair2")} {
		players := PlayersForPair(svc.app, pid)
		svc.notifier.NotifyPlayers(players, "general",
			"Resultado confirmado automaticamente",
			"El resultado ha sido confirmado por tiempo de espera.",
			fresh.Id)
	}
}

// ConfirmReminderHours returns the per-competition reminder threshold,
// defaulting to 12 when unset or zero.
func ConfirmReminderHours(comp *core.Record) int {
	h := int(comp.GetFloat("confirm_reminder_hours"))
	if h <= 0 {
		return 12
	}
	return h
}

// RemindPendingConfirmations notifies the confirming team of matches
// awaiting confirmation past the reminder threshold, once per submission.
func (svc *Service) RemindPendingConfirmations(now time.Time) {
	pending, err := svc.app.FindRecordsByFilter("matches",
		"status = 'confirmed' && confirm_reminded = false", "", 0, 0, nil)
	if err != nil || len(pending) == 0 {
		return
	}

	compCache := map[string]*core.Record{}
	for _, m := range pending {
		compID := m.GetString("competition")
		if _, ok := compCache[compID]; !ok {
			comp, err := svc.app.FindRecordById("competitions", compID)
			if err != nil {
				compCache[compID] = nil
				continue
			}
			compCache[compID] = comp
		}
	}

	for _, m := range pending {
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}
		svc.remindIfDue(m, comp, now)
	}
}

func (svc *Service) remindIfDue(m *core.Record, comp *core.Record, now time.Time) {
	threshold := ConfirmReminderHours(comp)
	timeout := int(comp.GetFloat("quorum_timeout_hours"))
	if timeout > 0 && threshold >= timeout {
		return
	}

	submittedAt := m.GetString("submitted_at")
	if submittedAt == "" {
		return
	}
	dt, err := types.ParseDateTime(submittedAt)
	if err != nil {
		return
	}
	if now.Sub(dt.Time()) < time.Duration(threshold)*time.Hour {
		return
	}

	fresh, err := svc.app.FindRecordById("matches", m.Id)
	if err != nil || fresh.GetString("status") != "confirmed" {
		return
	}

	submitterID := fresh.GetString("submitted_by")
	rivalPairID := fresh.GetString("pair2")
	team, err := PlayerTeam(svc.app, submitterID, fresh)
	if err != nil {
		slog.Error("remind: resolve submitter team", "match", m.Id, "err", err)
		return
	}
	if team == 2 {
		rivalPairID = fresh.GetString("pair1")
	}

	players := PlayersForPair(svc.app, rivalPairID)
	if len(players) == 0 {
		return
	}

	title := "Resultado pendiente de confirmar"
	body := fmt.Sprintf("Tu rival envió un resultado hace más de %d horas. Confirma o disputa.", threshold)
	link := "/match/" + fresh.Id

	svc.notifier.NotifyPlayers(players, "quorum_request", title, body, fresh.Id)
	svc.notifier.EmailPlayers(players, title, body, link)

	fresh.Set("confirm_reminded", true)
	if err := svc.app.Save(fresh); err != nil {
		slog.Error("save confirm_reminded", "match", m.Id, "err", err)
	}
}
