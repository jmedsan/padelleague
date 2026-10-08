package league

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
)

// ProposalData holds parsed scheduling proposal details from a thread message.
type ProposalData struct {
	Date      string `json:"date"`
	Time      string `json:"time"`
	VenueID   string `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueText string `json:"venue_text"`
	Scores    string `json:"scores,omitempty"`
	// Action is set on scheduling_response/result_response entries only:
	// "accept" or "reject", the decision that produced this entry.
	Action string `json:"action,omitempty"`
}

// ParseProposalData decodes a proposal from a raw JSON field value.
func ParseProposalData(raw any) *ProposalData {
	if raw == nil {
		return nil
	}
	var pd ProposalData
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(v), &pd); err != nil {
			return nil
		}
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(b, &pd); err != nil {
			return nil
		}
	}
	return &pd
}

// HasDateAndPlace reports whether the match has a confirmed date and place.
func HasDateAndPlace(match *core.Record) bool {
	return match.GetString("date") != "" && match.GetString("club") != ""
}

// PendingSchedulingProposal returns the newest scheduling proposal of matchID
// that awaits the rival pair's response, or nil.
func PendingSchedulingProposal(app core.App, matchID string) *core.Record {
	msgs, _ := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'pending'",
		"-created", 1, 0, map[string]any{"mid": matchID})
	if len(msgs) == 0 {
		return nil
	}
	return msgs[0]
}
