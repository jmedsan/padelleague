package league

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// ConfirmStaleMatches auto-finalizes matches past the quorum timeout.
// Handles both legacy status=confirmed matches and proposal-based pending result_submissions.
func (svc *Service) ConfirmStaleMatches() {
	svc.confirmStaleConfirmed()
	svc.confirmStaleProposals()
}

func (svc *Service) confirmStaleConfirmed() {
	stale, err := svc.app.FindRecordsByFilter("matches",
		"status = 'confirmed'", "", 0, 0, nil)
	if err != nil || len(stale) == 0 {
		return
	}

	compCache := svc.loadCompCache(stale)
	for _, m := range stale {
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}
		svc.confirmIfExpired(m, comp)
	}
}

func (svc *Service) confirmStaleProposals() {
	proposals, err := svc.app.FindRecordsByFilter("match_messages",
		"type = 'result_submission' && proposal_status = 'pending'",
		"", 0, 0, nil)
	if err != nil || len(proposals) == 0 {
		return
	}

	matchCache, compCache := svc.loadProposalCaches(proposals)
	for _, p := range proposals {
		m := matchCache[p.GetString("match")]
		if m == nil || !IsPreScore(m.GetString("status")) {
			continue
		}
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}
		svc.acceptProposalIfExpired(p, m, comp)
	}
}

func (svc *Service) loadProposalCaches(proposals []*core.Record) (map[string]*core.Record, map[string]*core.Record) {
	matchCache := map[string]*core.Record{}
	compCache := map[string]*core.Record{}
	for _, p := range proposals {
		matchID := p.GetString("match")
		if _, ok := matchCache[matchID]; ok {
			continue
		}
		m, err := svc.app.FindRecordById("matches", matchID)
		if err != nil {
			matchCache[matchID] = nil
			continue
		}
		matchCache[matchID] = m
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
	return matchCache, compCache
}

func (svc *Service) acceptProposalIfExpired(proposal, m, comp *core.Record) {
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

	pd := parseProposalScores(proposal.GetString("proposal_data"))
	if pd == "" {
		return
	}

	fresh, err := svc.app.FindRecordById("matches", m.Id)
	if err != nil || !IsPreScore(fresh.GetString("status")) {
		return
	}

	winnerID, err := DetermineWinner(fresh, pd)
	if err != nil {
		slog.Error("auto-accept proposal: determine winner", "match", m.Id, "err", err)
		return
	}

	fresh.Set("status", "final")
	fresh.Set("scores", pd)
	fresh.Set("winner", winnerID)
	fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
	if err := svc.app.Save(fresh); err != nil {
		slog.Error("auto-accept proposal: save match", "match", m.Id, "err", err)
		return
	}

	proposal.Set("proposal_status", "accepted")
	if err := svc.app.Save(proposal); err != nil {
		slog.Error("auto-accept proposal: update proposal", "match", m.Id, "err", err)
	}

	for _, pid := range []string{fresh.GetString("pair1"), fresh.GetString("pair2")} {
		players := PlayersForPair(svc.app, pid)
		svc.notifier.NotifyPlayers(players, Notification{
			Type:    "general",
			Title:   "Resultado confirmado automáticamente",
			Body:    "El resultado ha sido confirmado por tiempo de espera.",
			MatchID: fresh.Id,
		})
	}
}

func parseProposalScores(pdJSON string) string {
	if pdJSON == "" {
		return ""
	}
	type pd struct {
		Scores string `json:"scores"`
	}
	var p pd
	if err := json.Unmarshal([]byte(pdJSON), &p); err != nil {
		return ""
	}
	return p.Scores
}

func (svc *Service) loadCompCache(matches []*core.Record) map[string]*core.Record {
	compCache := map[string]*core.Record{}
	for _, m := range matches {
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
	return compCache
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
		svc.notifier.NotifyPlayers(players, Notification{
			Type:    "general",
			Title:   "Resultado confirmado automáticamente",
			Body:    "El resultado ha sido confirmado por tiempo de espera.",
			MatchID: fresh.Id,
		})
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

// RemindPendingConfirmations notifies the rival team of matches
// awaiting confirmation past the reminder threshold, once per submission.
// Handles both legacy status=confirmed and proposal-based matches.
func (svc *Service) RemindPendingConfirmations(now time.Time) {
	svc.remindLegacyPending(now)
	svc.remindProposalPending(now)
}

func (svc *Service) remindLegacyPending(now time.Time) {
	pending, err := svc.app.FindRecordsByFilter("matches",
		"status = 'confirmed' && confirm_reminded = false", "", 0, 0, nil)
	if err != nil || len(pending) == 0 {
		return
	}

	compCache := svc.loadCompCache(pending)
	for _, m := range pending {
		comp := compCache[m.GetString("competition")]
		if comp == nil {
			continue
		}
		svc.remindIfDue(m, comp, now)
	}
}

func (svc *Service) remindProposalPending(now time.Time) {
	proposals, err := svc.app.FindRecordsByFilter("match_messages",
		"type = 'result_submission' && proposal_status = 'pending'",
		"", 0, 0, nil)
	if err != nil || len(proposals) == 0 {
		return
	}

	seen := map[string]bool{}
	for _, p := range proposals {
		matchID := p.GetString("match")
		if seen[matchID] {
			continue
		}
		seen[matchID] = true

		m, err := svc.app.FindRecordById("matches", matchID)
		if err != nil || !IsPreScore(m.GetString("status")) {
			continue
		}
		if m.GetBool("confirm_reminded") {
			continue
		}

		comp, err := svc.app.FindRecordById("competitions", m.GetString("competition"))
		if err != nil {
			continue
		}
		svc.remindProposalIfDue(m, comp, now)
	}
}

func (svc *Service) remindProposalIfDue(m, comp *core.Record, now time.Time) {
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

	submitterID := m.GetString("submitted_by")
	rivalPairID := m.GetString("pair2")
	team, err := PlayerTeam(svc.app, submitterID, m)
	if err != nil {
		slog.Error("remind proposal: resolve submitter team", "match", m.Id, "err", err)
		return
	}
	if team == 2 {
		rivalPairID = m.GetString("pair1")
	}

	players := PlayersForPair(svc.app, rivalPairID)
	if len(players) == 0 {
		return
	}

	title := "Resultado pendiente de respuesta"
	body := fmt.Sprintf("Tu rival propuso un resultado hace más de %d horas. Acepta o contrapropón.", threshold)
	link := "/match/" + m.Id

	svc.notifier.NotifyPlayers(players, Notification{
		Type: "quorum_request", Title: title, Body: body, MatchID: m.Id,
	})
	svc.notifier.EmailPlayers(players, title, body, link)

	m.Set("confirm_reminded", true)
	if err := svc.app.Save(m); err != nil {
		slog.Error("save confirm_reminded for proposal", "match", m.Id, "err", err)
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

	svc.notifier.NotifyPlayers(players, Notification{
		Type: "quorum_request", Title: title, Body: body, MatchID: fresh.Id,
	})
	svc.notifier.EmailPlayers(players, title, body, link)

	fresh.Set("confirm_reminded", true)
	if err := svc.app.Save(fresh); err != nil {
		slog.Error("save confirm_reminded", "match", m.Id, "err", err)
	}
}
