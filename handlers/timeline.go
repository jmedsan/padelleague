package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

type timelineEntry struct {
	MatchID  string
	ActorID  string
	Kind     string
	Detail   string
	ParentID string
}

func addTimelineEntry(app core.App, e timelineEntry) {
	col, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		slog.Error("timeline: find collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("match", e.MatchID)
	rec.Set("author", e.ActorID)
	rec.Set("type", e.Kind)
	rec.Set("content", e.Detail)
	if e.ParentID != "" {
		rec.Set("parent", e.ParentID)
	}
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save entry", "match", e.MatchID, "err", err)
	}
}
