package handlers

import (
	"log/slog"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/search"
)

// SearchHandler serves the global search endpoint.
type SearchHandler struct {
	app        core.App
	leagueSvc  *league.Service
	index      *search.Index
	renderPage RenderFunc
}

// NewSearchHandler creates a SearchHandler with the given dependencies.
func NewSearchHandler(app core.App, leagueSvc *league.Service, index *search.Index, renderPage RenderFunc) *SearchHandler {
	return &SearchHandler{app: app, leagueSvc: leagueSvc, index: index, renderPage: renderPage}
}

// Search handles GET /search. Empty q returns recent+suggestions; non-empty q
// returns ranked, scope-filtered results grouped by type.
func (h *SearchHandler) Search(e *core.RequestEvent) error {
	q := e.Request.URL.Query().Get("q")
	viewer := h.buildViewer(e)

	if q == "" {
		return h.renderSuggestions(e, viewer)
	}

	results := h.index.Search(q, viewer, 20)

	h.recordQuery(e.Auth.Id, q)

	grouped := groupByType(results)

	return h.renderPage(e, "search-results.html", map[string]any{
		"Query":   q,
		"Results": results,
		"Grouped": grouped,
		"Empty":   len(results) == 0,
	})
}

func (h *SearchHandler) buildViewer(e *core.RequestEvent) search.Viewer {
	v := search.Viewer{
		CompIDs: make(map[string]struct{}),
	}
	if e.Auth == nil {
		return v
	}

	v.IsAdmin = slices.Contains(e.Auth.GetStringSlice("roles"), "admin")

	pairs, _ := league.PairsForPlayer(h.app, e.Auth.Id)
	for _, p := range pairs {
		comps, _ := h.app.FindRecordsByFilter("competitions",
			"pairs ~ {:pid}", "", 0, 0,
			map[string]any{"pid": p.Id})
		for _, c := range comps {
			v.CompIDs[c.Id] = struct{}{}
		}
	}

	return v
}

func (h *SearchHandler) recordQuery(userID, query string) {
	col, err := h.app.FindCollectionByNameOrId("search_history")
	if err != nil {
		slog.Error("search: find search_history collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("query", query)
	if err := h.app.Save(rec); err != nil {
		slog.Error("search: record query", "err", err)
	}
}

func (h *SearchHandler) renderSuggestions(e *core.RequestEvent, viewer search.Viewer) error {
	recent := h.recentSearches(e.Auth.Id)

	data := map[string]any{
		"Query":   "",
		"Results": []search.Result{},
		"Grouped": map[string][]search.Result{},
		"Empty":   true,
		"Recent":  recent,
	}

	if viewer.IsAdmin {
		setups, alerts, _ := league.AdminDashboard(h.app, time.Now())
		data["AdminSetups"] = setups
		data["AdminAlerts"] = alerts
	} else {
		tasks, _ := league.PlayerTasks(h.app, e.Auth.Id, time.Now())
		data["PlayerTasks"] = tasks
	}

	return h.renderPage(e, "search-results.html", data)
}

func (h *SearchHandler) recentSearches(userID string) []string {
	records, _ := h.app.FindRecordsByFilter("search_history",
		"user = {:uid}", "-created", 20, 0,
		map[string]any{"uid": userID})

	seen := make(map[string]struct{})
	var recent []string
	for _, r := range records {
		q := r.GetString("query")
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		recent = append(recent, q)
		if len(recent) >= 8 {
			break
		}
	}
	return recent
}

type groupedResults struct {
	Type    string
	Results []search.Result
}

func groupByType(results []search.Result) []groupedResults {
	order := make([]string, 0)
	m := make(map[string][]search.Result)
	for _, r := range results {
		if _, ok := m[r.Type]; !ok {
			order = append(order, r.Type)
		}
		m[r.Type] = append(m[r.Type], r)
	}
	groups := make([]groupedResults, 0, len(order))
	for _, t := range order {
		groups = append(groups, groupedResults{Type: t, Results: m[t]})
	}
	return groups
}

// RebuildIndex rebuilds the search index from the current database state.
func RebuildIndex(app core.App, index *search.Index) {
	entries := search.Build(app)
	if len(entries) == 0 {
		slog.Error("search: rebuild produced zero entries, keeping previous index")
		return
	}
	index.Replace(entries)
	slog.Info("search: index rebuilt", "entries", len(entries))
}
