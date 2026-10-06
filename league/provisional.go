package league

import (
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ProvisionalResults returns one synthetic, unsaved match per candidate that
// has a live pending result_submission proposal with a determined winner. A
// final or disputed candidate is excluded, as is an open, undecided set
// (EvaluateScore.Won == false). Each synthetic record keeps the real match's
// Id, so callers can map it back, plus the fields result readers need:
// pair1, pair2, competition, round_number, date, scores and winner.
func ProvisionalResults(app core.App, candidates []*core.Record) ([]*core.Record, error) {
	var open []*core.Record
	for _, m := range candidates {
		if s := m.GetString("status"); s != StatusFinal && s != StatusDisputed {
			open = append(open, m)
		}
	}
	if len(open) == 0 {
		return nil, nil
	}
	pending, err := pendingProposals(app, open)
	if err != nil {
		return nil, err
	}
	col, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return nil, err
	}

	var out []*core.Record
	for _, m := range open {
		p := provisionalProposal(app, m, pending[m.Id])
		if p == nil {
			continue
		}
		scores := parseProposalScores(p.GetString("proposal_data"))
		sc, err := ParseScoreMode(scores, AllowOpenSet)
		if err != nil || !EvaluateScore(sc).Won {
			continue
		}
		winner, err := DetermineWinner(m, scores)
		if err != nil {
			continue
		}
		synth := core.NewRecord(col)
		synth.Id = m.Id
		synth.Set("pair1", m.GetString("pair1"))
		synth.Set("pair2", m.GetString("pair2"))
		synth.Set("competition", m.GetString("competition"))
		synth.Set("round_number", m.GetFloat("round_number"))
		synth.Set("scores", scores)
		synth.Set("winner", winner)
		synth.Set("date", m.GetString("date"))
		out = append(out, synth)
	}
	return out, nil
}

// proposalChunkSize bounds the match ids OR-ed into one filter: PocketBase caps a
// filter at 200 expressions (search.DefaultFilterExprLimit), the type and status
// clauses included.
const proposalChunkSize = 100

// pendingProposals maps match id to its pending result proposals, newest
// first, looking only at the given matches.
func pendingProposals(app core.App, matches []*core.Record) (map[string][]*core.Record, error) {
	byMatch := make(map[string][]*core.Record, len(matches))
	for start := 0; start < len(matches); start += proposalChunkSize {
		chunk := matches[start:min(start+proposalChunkSize, len(matches))]
		clauses := make([]string, len(chunk))
		params := make(map[string]any, len(chunk))
		for i, m := range chunk {
			key := "m" + strconv.Itoa(i)
			clauses[i] = "match = {:" + key + "}"
			params[key] = m.Id
		}
		proposals, err := app.FindRecordsByFilter("match_messages",
			"type = 'result_submission' && proposal_status = 'pending' && ("+strings.Join(clauses, " || ")+")",
			"-created", 0, 0, params)
		if err != nil {
			return nil, err
		}
		for _, p := range proposals {
			byMatch[p.GetString("match")] = append(byMatch[p.GetString("match")], p)
		}
	}
	return byMatch, nil
}

// provisionalProposal picks the proposal that stands for the match's unconfirmed
// result: the newest pending one. It returns nil when there is none, or when
// the two pairs each hold a pending proposal and those disagree (a deadlock):
// a conflicting result is not a result, so it counts nowhere.
func provisionalProposal(app core.App, m *core.Record, props []*core.Record) *core.Record {
	if len(props) == 0 {
		return nil
	}
	if len(props) == 1 {
		return props[0]
	}
	var side1, side2 *core.Record
	for _, p := range props { // newest first: keep the newest per side
		switch AuthorSide(app, m, p.GetString("author")) {
		case Side1:
			if side1 == nil {
				side1 = p
			}
		case Side2:
			if side2 == nil {
				side2 = p
			}
		default:
			// An admin proposal is on neither side: it conflicts with any
			// other author's differing proposal, so treat it as its own side.
			return conflictingOrNewest(props)
		}
	}
	if side1 != nil && side2 != nil &&
		parseProposalScores(side1.GetString("proposal_data")) != parseProposalScores(side2.GetString("proposal_data")) {
		return nil
	}
	return props[0]
}

// Sides of a match an author can stand on.
const (
	SideNeither = 0 // not in either pair (an admin submitting for the match)
	Side1       = 1
	Side2       = 2
)

// AuthorSide says which pair authorID belongs to: Side1, Side2, or SideNeither.
// It never infers a side from "not in the other pair".
func AuthorSide(app core.App, m *core.Record, authorID string) int {
	switch {
	case slices.Contains(PlayersForPair(app, m.GetString("pair1")), authorID):
		return Side1
	case slices.Contains(PlayersForPair(app, m.GetString("pair2")), authorID):
		return Side2
	}
	return SideNeither
}

// conflictingOrNewest handles proposals from a non-participant: the match counts
// only when every pending proposal carries the same scores.
func conflictingOrNewest(props []*core.Record) *core.Record {
	first := parseProposalScores(props[0].GetString("proposal_data"))
	for _, p := range props[1:] {
		if parseProposalScores(p.GetString("proposal_data")) != first {
			return nil
		}
	}
	return props[0]
}
