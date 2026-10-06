package handlers

import (
	"github.com/pocketbase/pocketbase/core"
)

// Proposal types an admin decision can settle.
const (
	proposalScheduling = "scheduling_proposal"
	proposalResult     = "result_submission"
)

// adminRejectReason is the rejection reason recorded on every proposal an admin
// decision settles.
const adminRejectReason = "Resuelto por el administrador"

// adminChange is what an admin write decided: the timeline entry it leaves and
// the kinds of pending proposals it makes moot.
type adminChange struct {
	kind    string   // timeline type: "admin_action" or "result_event"
	detail  string   // timeline text
	decides []string // proposal types whose pending proposals are rejected
}

// adminWrite is one admin decision on a match. apply runs inside the
// transaction on a fresh copy of the match: it checks the match is still in the
// state the decision needs, sets the fields, and returns what it decided.
type adminWrite struct {
	matchID string
	adminID string
	apply   func(txApp core.App, match *core.Record) (adminChange, error)
}

// writeAsAdmin is the ONE way an admin sets or corrects a result, a date or a
// resolution. In one transaction it applies the decision to a fresh match,
// rejects (as the admin) every pending proposal of the kinds the decision
// settles, saves the match and writes the timeline entry. It returns the saved
// match and what was decided.
func writeAsAdmin(app core.App, w adminWrite) (*core.Record, adminChange, error) {
	var saved *core.Record
	var change adminChange
	err := app.RunInTransaction(func(txApp core.App) error {
		match, err := txApp.FindRecordById("matches", w.matchID)
		if err != nil {
			return err
		}
		if change, err = w.apply(txApp, match); err != nil {
			return err
		}
		if err := rejectPendingByAdmin(txApp, match, w.adminID, change.decides...); err != nil {
			return err
		}
		if err := txApp.Save(match); err != nil {
			return err
		}
		addTimelineEntry(txApp, timelineEntry{
			MatchID: match.Id, ActorID: w.adminID, Kind: change.kind, Detail: change.detail,
		})
		saved = match
		return nil
	})
	return saved, change, err
}

// rejectPendingByAdmin rejects, as adminID, every pending proposal of the given
// types on the match. Each leaves a frozen "rejected" response entry in the
// timeline, attributed to the admin, like a player's own rejection.
func rejectPendingByAdmin(txApp core.App, match *core.Record, adminID string, types ...string) error {
	for _, typ := range types {
		pending, err := txApp.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = {:t} && proposal_status = 'pending'", "created", 0, 0,
			map[string]any{"mid": match.Id, "t": typ})
		if err != nil {
			return err
		}
		for _, p := range pending {
			p.Set("proposal_status", "rejected")
			p.Set("rejection_text", adminRejectReason)
			if err := txApp.Save(p); err != nil {
				return err
			}
			addTimelineEntry(txApp, timelineEntry{
				MatchID: match.Id, ActorID: adminID, Kind: responseKind(typ),
				Detail:   "rechazó la propuesta de " + pairPlayerLabel(txApp, p.GetString("author"), match),
				ParentID: p.Id, Action: "reject", Note: adminRejectReason,
				Data: ParseProposalData(p.Get("proposal_data")),
			})
		}
	}
	return nil
}

// responseKind is the timeline type of the response to a proposal type.
func responseKind(proposalType string) string {
	if proposalType == proposalResult {
		return "result_response"
	}
	return "scheduling_response"
}
