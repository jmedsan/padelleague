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

// AdminEntry redirects to the single active competition detail when exactly
// one exists; otherwise falls through to the full dashboard.
func (h *CompetitionDashboardHandler) AdminEntry(e *core.RequestEvent) error {
	activeComps, _ := h.app.FindRecordsByFilter("competitions",
		"active = true", "", 0, 0, nil)
	if len(activeComps) == 1 {
		return e.Redirect(http.StatusFound, "/admin/competitions/"+activeComps[0].Id)
	}
	return h.Dashboard(e)
}

// Dashboard renders the admin competitions overview with active/inactive lists and issue counts.
func (h *CompetitionDashboardHandler) Dashboard(e *core.RequestEvent) error {
	allComps, _ := h.app.FindRecordsByFilter("competitions", "id != ''", "name", 0, 0, nil)

	var active, inactive []CompetitionView
	for _, comp := range allComps {
		summary := NewCompetitionView(h.app, comp, AdminSummary)
		if comp.GetBool("active") {
			active = append(active, summary)
		} else {
			inactive = append(inactive, summary)
		}
	}

	disputeCount, issueCount := healthCounts(league.HealthReport(h.app, time.Now()))

	return h.renderPage(e, "admin/competitions.html", map[string]any{
		"Active":       active,
		"Inactive":     inactive,
		"DisputeCount": disputeCount,
		"IssueCount":   issueCount,
	})
}

// healthCounts derives the competitions-list stat bar figures from
// HealthReport: DisputeCount is the disputes category alone; IssueCount
// sums every category so the two stats stay consistent with the Salud page.
func healthCounts(categories []league.HealthCategory) (disputeCount, issueCount int) {
	for _, cat := range categories {
		issueCount += len(cat.Items)
		if cat.Key == "disputes" {
			disputeCount = len(cat.Items)
		}
	}
	return disputeCount, issueCount
}
