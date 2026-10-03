package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Removes result proposals stored with no score (the submit form accepted an
// empty marker): the proposal and its timeline replies, the notifications that
// announced it, and the match's submitted_by/submitted_at when no other
// proposal remains. Irreversible: the removed rows carry no data.
func init() {
	m.Register(removeEmptyResultProposals, func(_ core.App) error { return nil })
}

func removeEmptyResultProposals(app core.App) error {
	empty, err := app.FindRecordsByFilter("match_messages",
		"type = 'result_submission' && content = ''", "", 0, 0, nil)
	if err != nil {
		return err
	}
	matchIDs := map[string]struct{}{}
	for _, proposal := range empty {
		matchIDs[proposal.GetString("match")] = struct{}{}
		if err := deleteEmptyProposal(app, proposal); err != nil {
			return err
		}
	}
	for matchID := range matchIDs {
		if err := removeEmptyProposalNotifications(app, matchID); err != nil {
			return err
		}
		if err := clearSubmission(app, matchID); err != nil {
			return err
		}
	}
	return nil
}

func deleteEmptyProposal(app core.App, proposal *core.Record) error {
	replies, err := app.FindRecordsByFilter("match_messages", "parent = {:id}", "", 0, 0,
		dbx.Params{"id": proposal.Id})
	if err != nil {
		return err
	}
	for _, reply := range replies {
		if err := app.Delete(reply); err != nil {
			return err
		}
	}
	return app.Delete(proposal)
}

func removeEmptyProposalNotifications(app core.App, matchID string) error {
	notes, err := app.FindRecordsByFilter("notifications",
		"related_match = {:m} && ((type = 'match_progress' && body = 'Resultado propuesto: ') || "+
			"(type = 'quorum_request' && body ~ 'ha enviado . Confirma'))",
		"", 0, 0, dbx.Params{"m": matchID})
	if err != nil {
		return err
	}
	for _, note := range notes {
		if err := app.Delete(note); err != nil {
			return err
		}
	}
	return nil
}

func clearSubmission(app core.App, matchID string) error {
	match, err := app.FindRecordById("matches", matchID)
	if err != nil {
		return nil // match deleted since
	}
	if status := match.GetString("status"); status != "pending" && status != "scheduled" {
		return nil
	}
	left, err := app.FindRecordsByFilter("match_messages",
		"match = {:m} && type = 'result_submission' && proposal_status = 'pending'",
		"", 1, 0, dbx.Params{"m": matchID})
	if err != nil || len(left) > 0 {
		return err
	}
	match.Set("submitted_by", "")
	match.SetRaw("submitted_at", types.DateTime{}) // autodate: Set is a no-op
	return app.Save(match)
}
