package handlers

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// CompetitionDashboardHandler renders the admin competitions overview.
type CompetitionDashboardHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewCompetitionDashboardHandler creates a CompetitionDashboardHandler.
func NewCompetitionDashboardHandler(app core.App, renderPage RenderFunc) *CompetitionDashboardHandler {
	return &CompetitionDashboardHandler{app: app, renderPage: renderPage}
}

// AdminEntry redirects /admin to the competitions list — the only nav entry
// pointing at /admin was "Panel", now removed, so /admin is reached solely
// by direct URL and should land on the dashboard, not skip it.
func (h *CompetitionDashboardHandler) AdminEntry(e *core.RequestEvent) error {
	return e.Redirect(http.StatusFound, "/admin/competitions")
}

// Dashboard renders the single admin landing page: urgent alerts, the setup
// checklist inline on each inactive competition's own card, and the
// active/inactive competition lists. This absorbed what used to be a
// separate admin section on the player home page (bootstrap prompt, playoff
// prompts, setup cards, alert rows) — the owner wanted one admin landing
// page instead of two with split responsibilities.
func (h *CompetitionDashboardHandler) Dashboard(e *core.RequestEvent) error {
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != ''", "name", 0, 0, nil)

	var active, inactive []CompetitionView
	for _, comp := range allComps {
		if comp.GetBool("active") {
			active = append(active, NewCompetitionView(h.app, comp, AdminSummary))
			continue
		}
		summary := NewCompetitionView(h.app, comp, AdminSummary)
		setup := league.CompSetupOf(h.app, comp)
		summary.Setup = &setup
		inactive = append(inactive, summary)
	}

	now := time.Now()
	report := league.HealthReport(h.app, now)
	disputeCount, issueCount := healthCounts(report)

	var activeComps []*core.Record
	for _, comp := range allComps {
		if comp.GetBool("active") {
			activeComps = append(activeComps, comp)
		}
	}

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"PageTitle":      "Competiciones",
		"Active":         active,
		"Inactive":       inactive,
		"DisputeCount":   disputeCount,
		"IssueCount":     issueCount,
		"UrgentItems":    urgentHealthItems(report),
		"AdminBootstrap": len(allComps) == 0,
		"PlayoffPrompts": league.PlayoffPrompts(h.app, activeComps, now),
	})
}

// healthCounts derives the competitions-list stat bar figures from
// HealthReport: DisputeCount is the disputes category alone; IssueCount
// sums the non-urgent categories (overdue, unscheduled, unpaid) so it
// doesn't double-count disputes/walkovers, which the Alertas section and
// the disputes stat already surface.
func healthCounts(categories []league.HealthCategory) (disputeCount, issueCount int) {
	for _, cat := range categories {
		if cat.Key == "disputes" {
			disputeCount = len(cat.Items)
		}
		if !cat.Urgent {
			issueCount += len(cat.Items)
		}
	}
	return disputeCount, issueCount
}

// urgentHealthItems flattens HealthReport's urgent categories (disputes,
// walkovers) into the compact rows the competitions page shows above the
// competition list, rendered via the shared healthItemRow partial.
func urgentHealthItems(categories []league.HealthCategory) []league.HealthItem {
	var out []league.HealthItem
	for _, cat := range categories {
		if !cat.Urgent {
			continue
		}
		out = append(out, cat.Items...)
	}
	return out
}
