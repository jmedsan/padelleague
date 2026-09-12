package league

import (
	"encoding/json"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// systemProposalData mirrors the JSON shape handlers.ProposalData expects
// for a result_response entry's proposal_data — only action/scores set.
type systemProposalData struct {
	Scores string `json:"scores,omitempty"`
	Action string `json:"action,omitempty"`
}

// AddSystemResultAccepted writes a system-authored (empty author) timeline
// entry recording that the quorum cron auto-accepted a result. parentID is
// the accepted proposal's match_messages ID, or "" for the legacy
// status=confirmed path which has no proposal record to point at.
func AddSystemResultAccepted(app core.App, matchID, parentID, scores string) {
	col, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		slog.Error("timeline: find match_messages collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("type", "result_response")
	rec.Set("content", "Resultado aceptado: "+scores)
	if parentID != "" {
		rec.Set("parent", parentID)
	}
	pdJSON, _ := json.Marshal(systemProposalData{Action: "accept", Scores: scores})
	rec.Set("proposal_data", string(pdJSON))
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save system result-accepted entry", "match", matchID, "err", err)
	}
}

// AddResultAccepted writes an actor-attributed timeline entry recording that
// a player accepted a result proposal.
func AddResultAccepted(app core.App, matchID, parentID, actorID, scores string) {
	col, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		slog.Error("timeline: find match_messages collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("author", actorID)
	rec.Set("type", "result_response")
	rec.Set("content", "Resultado aceptado: "+scores)
	if parentID != "" {
		rec.Set("parent", parentID)
	}
	pdJSON, _ := json.Marshal(systemProposalData{Action: "accept", Scores: scores})
	rec.Set("proposal_data", string(pdJSON))
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save result-accepted entry", "match", matchID, "err", err)
	}
}

// AddSystemResultEvent writes a system-authored result_event entry (no author).
func AddSystemResultEvent(app core.App, matchID, text string) {
	col, err := app.FindCollectionByNameOrId("match_messages")
	if err != nil {
		slog.Error("timeline: find match_messages collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("match", matchID)
	rec.Set("type", "result_event")
	rec.Set("content", text)
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save result-event entry", "match", matchID, "err", err)
	}
}

// CompetitionEvent is one entry recorded on a competition's admin activity timeline.
type CompetitionEvent struct {
	CompetitionID string
	ActorID       string // empty means a system-originated event
	Kind          string // "activated" | "deactivated" | "finalized" | "settings_changed"
	Detail        string
}

// LogCompetitionEvent writes a competition_events record for the admin
// activity timeline.
func LogCompetitionEvent(app core.App, ev CompetitionEvent) {
	col, err := app.FindCollectionByNameOrId("competition_events")
	if err != nil {
		slog.Error("timeline: find competition_events collection", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("competition", ev.CompetitionID)
	if ev.ActorID != "" {
		rec.Set("actor", ev.ActorID)
	}
	rec.Set("kind", ev.Kind)
	rec.Set("detail", ev.Detail)
	if err := app.Save(rec); err != nil {
		slog.Error("timeline: save competition event", "competition", ev.CompetitionID, "err", err)
	}
}
