package league

import (
	"sort"

	"github.com/pocketbase/pocketbase/core"
)

// AttachedDocuments returns a competition's attached reference items, sorted by
// title so a rendered list has a stable order.
func AttachedDocuments(app core.App, comp *core.Record) []*core.Record {
	ids := comp.GetStringSlice("documents")
	docs := make([]*core.Record, 0, len(ids))
	for _, id := range ids {
		if d, err := app.FindRecordById("documents", id); err == nil {
			docs = append(docs, d)
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].GetString("title") < docs[j].GetString("title")
	})
	return docs
}

// MandatoryDocIDs returns the IDs of a competition's attached mandatory items.
func MandatoryDocIDs(app core.App, comp *core.Record) []string {
	var ids []string
	for _, d := range AttachedDocuments(app, comp) {
		if d.GetBool("is_mandatory") {
			ids = append(ids, d.Id)
		}
	}
	return ids
}

// UnacknowledgedMandatory returns the competition's mandatory items the user has
// not yet acknowledged. An empty result means the gate is satisfied.
func UnacknowledgedMandatory(app core.App, comp *core.Record, userID string) []*core.Record {
	acked := make(map[string]struct{})
	for _, id := range ackedDocIDs(app, comp.Id, userID) {
		acked[id] = struct{}{}
	}
	var pending []*core.Record
	for _, d := range AttachedDocuments(app, comp) {
		if !d.GetBool("is_mandatory") {
			continue
		}
		if _, ok := acked[d.Id]; !ok {
			pending = append(pending, d)
		}
	}
	return pending
}

// FindOrNewAck returns the (user, competition) acknowledgment record, creating
// an unsaved one when none exists yet.
func FindOrNewAck(app core.App, compID, userID string) (*core.Record, error) {
	if acks := findAcks(app, compID, userID); len(acks) > 0 {
		return acks[0], nil
	}
	col, err := app.FindCollectionByNameOrId("document_acks")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("competition", compID)
	return rec, nil
}

// IsParticipant reports whether any of the player's pairs is enrolled in comp.
func IsParticipant(comp *core.Record, playerPairIDs map[string]struct{}) bool {
	for _, pid := range comp.GetStringSlice("pairs") {
		if _, ok := playerPairIDs[pid]; ok {
			return true
		}
	}
	return false
}

// AppendUnique returns s with v appended when not already present.
func AppendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// RemoveString returns s with every occurrence of v removed.
func RemoveString(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func ackedDocIDs(app core.App, compID, userID string) []string {
	acks := findAcks(app, compID, userID)
	if len(acks) == 0 {
		return nil
	}
	return acks[0].GetStringSlice("documents")
}

func findAcks(app core.App, compID, userID string) []*core.Record {
	acks, _ := app.FindRecordsByFilter("document_acks",
		"user = {:u} && competition = {:c}", "", 1, 0,
		map[string]any{"u": userID, "c": compID})
	return acks
}
