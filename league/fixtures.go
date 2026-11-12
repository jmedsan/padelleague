package league

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Round groups matches for a single round of play.
type Round struct {
	Number  int
	Matches []RoundMatch
}

// RoundMatch pairs home and away pair IDs for a single fixture.
type RoundMatch struct {
	Home string
	Away string
}

// RoundRobin generates a round-robin schedule for the given pair IDs using
// the canonical Berger table: within a leg, each team's home/away streak is
// at most 2 (1 with an odd pair count needing a bye) and each team's
// home−away imbalance is at most 1. A double round robin's second leg
// mirrors the first leg's sides with the round order rotated by one, so no
// pair meets again for at least n−2 rounds and the streak/imbalance bounds
// still hold across the leg boundary.
func RoundRobin(pairIDs []string, double bool) []Round {
	if len(pairIDs) < 2 {
		return nil
	}

	firstLeg := bergerLeg(pairIDs)
	rounds := numberRounds(firstLeg, 0)
	if !double {
		return rounds
	}

	secondLeg := mirrorLegSides(firstLeg)
	secondLeg = append(secondLeg[1:], secondLeg[0])
	return append(rounds, numberRounds(secondLeg, len(rounds))...)
}

// bergerLeg builds one round-robin leg via the canonical Berger table,
// grouped by round (byes still included as empty-string matches). A fixed
// pair rotates through the others while every remaining pair swaps
// positions each round; round r (0-based) has the fixed pair at index m
// play p[r], home when r is odd. Each other matchup p[(r+k)%m] vs
// p[(r-k+m)%m] puts the (r+k) side home when k is odd — this is what bounds
// the streak/imbalance to the theoretical minimum.
func bergerLeg(pairIDs []string) [][]RoundMatch {
	pairs := append([]string{}, pairIDs...)
	if len(pairs)%2 == 1 {
		pairs = append(pairs, "")
	}
	m := len(pairs) - 1

	rounds := make([][]RoundMatch, m)
	for r := range m {
		var round []RoundMatch
		fixed, opponent := pairs[m], pairs[r]
		if r%2 == 1 {
			round = append(round, RoundMatch{Home: fixed, Away: opponent})
		} else {
			round = append(round, RoundMatch{Home: opponent, Away: fixed})
		}

		for k := 1; k <= (m-1)/2; k++ {
			a, b := pairs[(r+k)%m], pairs[(r-k+m)%m]
			if k%2 == 1 {
				round = append(round, RoundMatch{Home: a, Away: b})
			} else {
				round = append(round, RoundMatch{Home: b, Away: a})
			}
		}
		rounds[r] = round
	}
	return rounds
}

// numberRounds drops bye matches and assigns sequential Round numbers
// starting at numberOffset+1.
func numberRounds(leg [][]RoundMatch, numberOffset int) []Round {
	rounds := make([]Round, len(leg))
	for i, roundMatches := range leg {
		var matches []RoundMatch
		for _, rm := range roundMatches {
			if rm.Home != "" && rm.Away != "" {
				matches = append(matches, rm)
			}
		}
		rounds[i] = Round{Number: numberOffset + i + 1, Matches: matches}
	}
	return rounds
}

// mirrorLegSides swaps Home/Away on every match, preserving round grouping.
func mirrorLegSides(leg [][]RoundMatch) [][]RoundMatch {
	mirrored := make([][]RoundMatch, len(leg))
	for i, roundMatches := range leg {
		swapped := make([]RoundMatch, len(roundMatches))
		for j, rm := range roundMatches {
			swapped[j] = RoundMatch{Home: rm.Away, Away: rm.Home}
		}
		mirrored[i] = swapped
	}
	return mirrored
}

// AdvancePlayoff seeds winners of the current round into the next playoff round.
// It reads the round and writes the next-round matches in one transaction, so a
// next-round match an admin or the players changed meanwhile is never
// overwritten with a stale copy.
func (svc *Service) AdvancePlayoff(matchRecord *core.Record) error {
	compID := matchRecord.GetString("competition")
	comp, err := svc.app.FindRecordById("competitions", compID)
	if err != nil || comp.GetString("type") != "playoff" {
		return nil
	}
	currentRound := int(matchRecord.GetFloat("round_number"))

	return svc.app.RunInTransaction(func(txApp core.App) error {
		return advanceRound(txApp, compID, currentRound)
	})
}

// advanceRound seeds the winners of a finished round into the next round's
// still-unplayed matches, on txApp's fresh reads.
func advanceRound(txApp core.App, compID string, currentRound int) error {
	// NOTE: bye handling assumes power-of-2 brackets; non-power-of-2 pair counts with byes may misalign roundWinners.
	roundMatches, _ := txApp.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "created", 0, 0,
		map[string]any{"cid": compID, "rn": currentRound})
	for _, m := range roundMatches {
		if m.GetString("status") != "final" {
			return nil
		}
	}

	nextMatches, _ := txApp.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "created", 0, 0,
		map[string]any{"cid": compID, "rn": currentRound + 1})

	var roundWinners []string
	for _, m := range roundMatches {
		roundWinners = append(roundWinners, m.GetString("winner"))
	}
	for i, nm := range nextMatches {
		if !IsPreScore(nm.GetString("status")) {
			continue
		}
		if seedNextMatch(nm, roundWinners, i) {
			if err := settleSeeds(txApp, nm); err != nil {
				return err
			}
		}
		if err := txApp.Save(nm); err != nil {
			return err
		}
	}
	return nil
}

// PlayoffMaxRound returns a playoff competition's final round number. ok is
// false for non-playoff competitions or ones with no matches yet.
func PlayoffMaxRound(app core.App, comp *core.Record) (maxRound int, ok bool) {
	if comp.GetString("type") != "playoff" {
		return 0, false
	}
	allMatches, _ := app.FindRecordsByFilter("matches",
		"competition = {:cid}", "-round_number", 1, 0,
		map[string]any{"cid": comp.Id})
	if len(allMatches) == 0 {
		return 0, false
	}
	return int(allMatches[0].GetFloat("round_number")), true
}

// seedNextMatch seeds nm's slots from the previous round's winners. A slot with no winner
// feeding it is left as it is; it reports false when that slot is also empty, so the match
// waits for its pair.
func seedNextMatch(nm *core.Record, winners []string, matchIdx int) bool {
	ready := true
	for side, field := range []string{"pair1", "pair2"} {
		idx := matchIdx*2 + side
		if idx < len(winners) {
			nm.Set(field, winners[idx])
		} else if nm.GetString(field) == "" {
			ready = false
		}
	}
	return ready
}

// settleSeeds decides nm once the previous round is final. A withdrawn seed is flag-only: it
// advances like any pair. A match with no pair at all is void, and a single seeded pair wins by
// walkover over the empty side.
func settleSeeds(txApp core.App, nm *core.Record) error {
	playable := func(id string) bool { return id != "" }
	p1, p2 := nm.GetString("pair1"), nm.GetString("pair2")
	p1Ok, p2Ok := playable(p1), playable(p2)
	switch {
	case p1Ok && p2Ok:
		return nil
	case !p1Ok && !p2Ok:
		ApplyVoidResult(nm)
		AddSystemResultEvent(txApp, nm.Id, NoResultEventText)
		return nil
	case p1Ok:
		return walkoverOver(txApp, nm, WalkoverScore, p1)
	default:
		return walkoverOver(txApp, nm, WalkoverScore, p2)
	}
}

// walkoverOver gives nm to winner by walkover over an empty side: no pair is left to play it.
func walkoverOver(txApp core.App, nm *core.Record, score, winner string) error {
	ApplyWalkoverResult(nm, winner, score)
	winnerName, err := pairName(txApp, winner)
	if err != nil {
		return err
	}
	AddSystemResultEvent(txApp, nm.Id, walkoverNoRivalText(winnerName))
	return nil
}

// walkoverNoRivalText is the timeline line for a walkover over an empty side (a void previous match).
func walkoverNoRivalText(winner string) string {
	return fmt.Sprintf("%s gana por incomparecencia: no hay pareja rival que pueda jugar", winner)
}

// pairName is the display name of pair id.
func pairName(txApp core.App, id string) (string, error) {
	pair, err := txApp.FindRecordById("pairs", id)
	if err != nil {
		return "", err
	}
	return pair.GetString("name"), nil
}
