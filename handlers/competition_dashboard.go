package handlers

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"padelleague/league"
)

// AdminIssue represents a problem detected in competition state for the admin dashboard.
type AdminIssue struct {
	Type            string
	TypeLabel       string
	BadgeClass      string
	CompetitionName string
	Pair1Name       string
	Pair2Name       string
	MatchID         string
	Detail          string
}

// CompetitionSummary holds aggregate stats for a competition on the dashboard.
type CompetitionSummary struct {
	Competition   *core.Record
	PairsCount    int
	TotalMatches  int
	PlayedMatches int
	DisputeCount  int
}

// Dashboard renders the admin competitions overview with active/inactive lists and issues.
func (h *CompetitionHandler) Dashboard(e *core.RequestEvent) error {
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != ''", "", 0, 0, nil)

	var active, inactive []CompetitionSummary
	for _, comp := range allComps {
		summary := h.buildCompetitionSummary(comp)
		if comp.GetBool("active") {
			active = append(active, summary)
		} else {
			inactive = append(inactive, summary)
		}
	}

	totalDisputes, _ := h.app.FindRecordsByFilter("matches",
		"status = 'disputed'", "", 0, 0, nil)

	issues := h.buildAdminIssues(active)

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Active":       active,
		"Inactive":     inactive,
		"DisputeCount": len(totalDisputes),
		"Issues":       issues,
		"IssueCount":   len(issues),
	})
}

func (h *CompetitionHandler) buildCompetitionSummary(comp *core.Record) CompetitionSummary {
	allMatches, _ := h.app.FindRecordsByFilter("matches",
		"competition = {:cid}", "", 0, 0,
		map[string]any{"cid": comp.Id})
	playedMatches := 0
	disputeCount := 0
	for _, m := range allMatches {
		if m.GetString("status") == league.StatusFinal {
			playedMatches++
		}
		if m.GetString("status") == league.StatusDisputed {
			disputeCount++
		}
	}
	return CompetitionSummary{
		Competition:   comp,
		PairsCount:    len(comp.GetStringSlice("pairs")),
		TotalMatches:  len(allMatches),
		PlayedMatches: playedMatches,
		DisputeCount:  disputeCount,
	}
}

func (h *CompetitionHandler) buildAdminIssues(active []CompetitionSummary) []AdminIssue {
	var issues []AdminIssue
	now := time.Now().UTC()
	for _, cs := range active {
		comp := cs.Competition
		compName := comp.GetString("name")
		quorumHours := comp.GetFloat("quorum_timeout_hours")

		matches, _ := h.app.FindRecordsByFilter("matches",
			"competition = {:cid}", "", 0, 0,
			map[string]any{"cid": comp.Id})
		pairIDs := make([]string, 0)
		for _, m := range matches {
			pairIDs = append(pairIDs, m.GetString("pair1"), m.GetString("pair2"))
		}
		pairNames := league.PairNames(h.app, pairIDs)

		ictx := issueContext{compName: compName, quorumHours: quorumHours, pairNames: pairNames, now: now}
		for _, m := range matches {
			issues = append(issues, h.classifyMatchIssues(m, ictx)...)
		}
	}
	return issues
}

type issueContext struct {
	compName    string
	quorumHours float64
	pairNames   map[string]string
	now         time.Time
}

func (h *CompetitionHandler) classifyMatchIssues(m *core.Record, ctx issueContext) []AdminIssue {
	status := m.GetString("status")
	base := AdminIssue{
		CompetitionName: ctx.compName,
		Pair1Name:       ctx.pairNames[m.GetString("pair1")],
		Pair2Name:       ctx.pairNames[m.GetString("pair2")],
		MatchID:         m.Id,
	}

	switch status {
	case league.StatusDisputed:
		issue := base
		issue.Type = "dispute"
		issue.TypeLabel = "Disputa"
		issue.BadgeClass = "badge-error"
		issue.Detail = "pendiente de resolución"
		return []AdminIssue{issue}
	case league.StatusConfirmed:
		return h.checkQuorumIssue(m, base, ctx.quorumHours, ctx.now)
	case league.StatusPending:
		return h.checkPendingIssues(m, base, ctx.now)
	}
	return nil
}

func (h *CompetitionHandler) checkQuorumIssue(m *core.Record, base AdminIssue, quorumHours float64, now time.Time) []AdminIssue {
	if quorumHours <= 0 {
		return nil
	}
	sa := m.GetString("submitted_at")
	if sa == "" {
		return nil
	}
	dt, err := types.ParseDateTime(sa)
	if err != nil {
		return nil
	}
	elapsed := now.Sub(dt.Time())
	if elapsed <= time.Duration(quorumHours)*time.Hour {
		return nil
	}
	days := int(elapsed.Hours() / 24)
	detail := fmt.Sprintf("enviado hace %d días", days)
	if days == 0 {
		detail = fmt.Sprintf("enviado hace %d horas", int(elapsed.Hours()))
	}
	issue := base
	issue.Type = "quorum"
	issue.TypeLabel = "Quorum"
	issue.BadgeClass = "badge-warning"
	issue.Detail = detail
	return []AdminIssue{issue}
}

func (h *CompetitionHandler) checkPendingIssues(m *core.Record, base AdminIssue, now time.Time) []AdminIssue {
	var issues []AdminIssue
	if d := m.GetString("date"); d != "" {
		if dt, err := types.ParseDateTime(d); err == nil && dt.Time().Before(now) {
			issue := base
			issue.Type = "overdue"
			issue.TypeLabel = "Vencido"
			issue.BadgeClass = "badge-ghost"
			issue.Detail = "fecha: " + d
			issues = append(issues, issue)
		}
	}
	lastMsg, _ := h.app.FindRecordsByFilter("match_messages",
		"match = {:mid}", "-created", 1, 0,
		map[string]any{"mid": m.Id})
	if len(lastMsg) == 0 {
		return issues
	}
	created := lastMsg[0].GetString("created")
	t, err := time.Parse("2006-01-02 15:04:05.000Z", created)
	if err != nil {
		return issues
	}
	if now.Sub(t) > 14*24*time.Hour {
		days := int(now.Sub(t).Hours() / 24)
		issue := base
		issue.Type = "stale"
		issue.TypeLabel = "Inactivo"
		issue.BadgeClass = "badge-info"
		issue.Detail = fmt.Sprintf("sin actividad en %d días", days)
		issues = append(issues, issue)
	}
	return issues
}
