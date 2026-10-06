package league

import (
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
	newest, err := newestPendingProposals(app, open)
	if err != nil {
		return nil, err
	}
	col, err := app.FindCollectionByNameOrId("matches")
	if err != nil {
		return nil, err
	}

	var out []*core.Record
	for _, m := range open {
		p, ok := newest[m.Id]
		if !ok {
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

// newestPendingProposals maps match id to its newest pending result proposal,
// looking only at the given matches.
func newestPendingProposals(app core.App, matches []*core.Record) (map[string]*core.Record, error) {
	newest := make(map[string]*core.Record, len(matches))
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
		// Each match id is in exactly one chunk, so the first record seen per
		// match in this newest-first list is its newest proposal.
		for _, p := range proposals {
			if _, ok := newest[p.GetString("match")]; !ok {
				newest[p.GetString("match")] = p
			}
		}
	}
	return newest, nil
}
