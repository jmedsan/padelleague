package handlers

import (
	"encoding/json"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
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
	Data   *league.ProposalData
	Scores string
	// Note is a frozen note shown under this entry's dateBox/resultBox (e.g.
	// a rejection reason) — stored on this entry's OWN record, since the
	// timeline reads it back via mc.msg, not the parent proposal's record.
	Note string
}

// addTimelineEntry writes a timeline entry and only logs a failure: for a
// best-effort history line that must not undo the action it records.
func addTimelineEntry(app core.App, e timelineEntry) {
	if err := saveTimelineEntry(app, e); err != nil {
		slog.Error("timeline: save entry", "match", e.MatchID, "err", err)
	}
}

// saveTimelineEntry writes a timeline entry and returns the failure, for
// callers (inside a transaction) whose action must roll back with the entry.
func saveTimelineEntry(app core.App, e timelineEntry) error {
	col, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("match", e.MatchID)
	rec.Set("author", e.ActorID)
	rec.Set("type", e.Kind)
	rec.Set("content", e.Detail)
	if e.ParentID != "" {
		rec.Set("parent", e.ParentID)
	}
	if e.Note != "" {
		rec.Set("rejection_text", e.Note)
	}
	if e.Action != "" {
		pd := league.ProposalData{Action: e.Action, Scores: e.Scores}
		if e.Data != nil {
			pd.Date, pd.Time, pd.VenueName = e.Data.Date, e.Data.Time, e.Data.VenueName
			if pd.Scores == "" {
				pd.Scores = e.Data.Scores
			}
		}
		pdJSON, _ := json.Marshal(pd)
		rec.Set("proposal_data", string(pdJSON))
	}
	return app.Save(rec)
}
