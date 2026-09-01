package handlers

import (
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

type healthItem struct{ Label, URL string }

type healthGroup struct {
	Title   string
	Items   []healthItem
	ListURL string
}

// AdminHealthHandler serves the consolidated league health dashboard.
type AdminHealthHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewAdminHealthHandler creates an AdminHealthHandler.
func NewAdminHealthHandler(app core.App, renderPage RenderFunc) *AdminHealthHandler {
	return &AdminHealthHandler{app: app, renderPage: renderPage}
}

// Health renders the admin health dashboard.
func (h *AdminHealthHandler) Health(e *core.RequestEvent) error {
	groups := h.build()
	return h.renderPage(e, "admin/health.html", map[string]any{
		"Groups": groups,
	})
}

func (h *AdminHealthHandler) build() []healthGroup {
	disputes := healthGroup{Title: "Disputas abiertas", ListURL: "/admin/disputes"}
	walkovers := healthGroup{Title: "Walkovers pendientes"}
	overdue := healthGroup{Title: "Partidos vencidos", ListURL: "/admin/outstanding"}
	unpaid := healthGroup{Title: "Parejas sin pagar"}
	unscheduled := healthGroup{Title: "Partidos sin fecha", ListURL: "/admin/outstanding"}

	_, alerts, err := league.AdminDashboard(h.app, time.Now())
	if err != nil {
		slog.Warn("health: AdminDashboard failed", "err", err)
	}
	for _, a := range alerts {
		item := healthItem{Label: a.Description, URL: "/match/" + a.MatchID}
		switch a.Kind {
		case "dispute":
			disputes.Items = append(disputes.Items, item)
		case "walkover":
			walkovers.Items = append(walkovers.Items, item)
		case "overdue":
			overdue.Items = append(overdue.Items, item)
		}
	}

	h.buildUnpaid(&unpaid)
	h.buildUnscheduled(&unscheduled)

	return []healthGroup{disputes, walkovers, overdue, unpaid, unscheduled}
}

func (h *AdminHealthHandler) buildUnpaid(g *healthGroup) {
	comps, err := h.app.FindRecordsByFilter("competitions", "active = true", "name", 0, 0, nil)
	if err != nil {
		return
	}
	for _, comp := range comps {
		ps := make(map[string]bool)
		_ = comp.UnmarshalJSONField("payment_status", &ps)
		pairIDs := comp.GetStringSlice("pairs")
		pairNames := league.PairNames(h.app, pairIDs)
		compName := comp.GetString("name")
		for _, pid := range pairIDs {
			if !ps[pid] {
				label := pairNames[pid] + " — " + compName
				g.Items = append(g.Items, healthItem{
					Label: label,
					URL:   "/admin/competitions/" + comp.Id,
				})
			}
		}
	}
}

func (h *AdminHealthHandler) buildUnscheduled(g *healthGroup) {
	matches, err := h.app.FindRecordsByFilter("matches",
		"status != 'final' && date = ''", "round_number", 0, 0, nil)
	if err != nil {
		slog.Warn("health: unscheduled query failed", "err", err)
		return
	}
	for _, m := range matches {
		pairNames := league.PairNames(h.app, []string{m.GetString("pair1"), m.GetString("pair2")})
		p1 := pairNames[m.GetString("pair1")]
		p2 := pairNames[m.GetString("pair2")]
		g.Items = append(g.Items, healthItem{
			Label: p1 + " vs " + p2,
			URL:   "/match/" + m.Id,
		})
	}
}
