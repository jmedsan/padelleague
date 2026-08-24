package league

import (
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

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
