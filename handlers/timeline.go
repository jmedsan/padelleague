package handlers

import (
	"encoding/json"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

type timelineEntry struct {
	MatchID  string
	ActorID  string
	Kind     string
	Detail   string
	ParentID string
	// Action is "accept" or "reject" for scheduling_response/result_response
	// entries — stored in proposal_data so the timeline can show the frozen
	// per-event status without re-parsing Detail's prose.
	Action string
	// Scores is the result score this entry refers to (result_response only),
	// stored in proposal_data so the timeline can render it via resultBox.
	Scores string
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
	if e.Action != "" {
		pdJSON, _ := json.Marshal(ProposalData{Action: e.Action, Scores: e.Scores})
		rec.Set("proposal_data", string(pdJSON))
	}
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save entry", "match", e.MatchID, "err", err)
	}
}
