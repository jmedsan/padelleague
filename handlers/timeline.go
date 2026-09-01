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
	// Data carries the parent proposal's date/time/venue or score, so a
	// response entry renders the identical dateBox/resultBox as its
	// proposal, just with a different status badge. Scores set here wins
	// over Data.Scores for a result_response (Data may be nil).
	Data   *ProposalData
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
		pd := ProposalData{Action: e.Action, Scores: e.Scores}
		if e.Data != nil {
			pd.Date, pd.Time, pd.VenueName = e.Data.Date, e.Data.Time, e.Data.VenueName
			if pd.Scores == "" {
				pd.Scores = e.Data.Scores
			}
		}
		pdJSON, _ := json.Marshal(pd)
		rec.Set("proposal_data", string(pdJSON))
	}
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save entry", "match", e.MatchID, "err", err)
	}
}
