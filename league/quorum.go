package league

import (
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func (svc *Service) ConfirmStaleMatches() {
	stale, err := svc.App.FindRecordsByFilter("matches",
		"status = 'confirmed'", "", 0, 0, nil)
	if err != nil || len(stale) == 0 {
		return
	}

	compCache := map[string]*core.Record{}
	for _, m := range stale {
		compID := m.GetString("competition")
		if _, ok := compCache[compID]; !ok {
			comp, err := svc.App.FindRecordById("competitions", compID)
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

		timeoutHours := int(comp.GetFloat("quorum_timeout_hours"))
		if timeoutHours == 0 {
			continue
		}

		submittedAt := m.GetString("submitted_at")
		if submittedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, submittedAt)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05.000Z", submittedAt)
			if err != nil {
				continue
			}
		}
		if time.Since(t) < time.Duration(timeoutHours)*time.Hour {
			continue
		}

		fresh, err := svc.App.FindRecordById("matches", m.Id)
		if err != nil || fresh.GetString("status") != "confirmed" {
			continue
		}

		score := fresh.GetString("scores")
		winnerID, err := DetermineWinner(fresh, score)
		if err != nil {
			continue
		}

		fresh.Set("status", "final")
		fresh.Set("winner", winnerID)
		fresh.Set("confirmed_by", "")
		fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
		if err := svc.App.Save(fresh); err != nil {
			slog.Error("save stale match confirmation", "match", m.Id, "err", err)
			continue
		}

		pairIDs := []string{fresh.GetString("pair1"), fresh.GetString("pair2")}
		for _, pid := range pairIDs {
			players := PlayersForPair(svc.App, pid)
			svc.Notifier.NotifyPlayers(players, "general",
				"Resultado confirmado automaticamente",
				"El resultado ha sido confirmado por tiempo de espera.",
				fresh.Id)
		}
	}
}
