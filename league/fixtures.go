package league

import "github.com/pocketbase/pocketbase/core"

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
func (svc *Service) AdvancePlayoff(matchRecord *core.Record) error {
	compID := matchRecord.GetString("competition")
	comp, err := svc.app.FindRecordById("competitions", compID)
	if err != nil || comp.GetString("type") != "playoff" {
		return nil
	}

	currentRound := int(matchRecord.GetFloat("round_number"))

	// NOTE: bye handling assumes power-of-2 brackets; non-power-of-2 pair counts with byes may misalign roundWinners.
	roundMatches, _ := svc.app.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "created", 0, 0,
		map[string]any{"cid": compID, "rn": currentRound})

	for _, m := range roundMatches {
		if m.GetString("status") != "final" {
			return nil
		}
	}

	nextRound := currentRound + 1
	nextMatches, _ := svc.app.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "created", 0, 0,
		map[string]any{"cid": compID, "rn": nextRound})

	if len(nextMatches) == 0 {
		return nil
	}

	var roundWinners []string
	for _, m := range roundMatches {
		roundWinners = append(roundWinners, m.GetString("winner"))
	}

	for i, nm := range nextMatches {
		if !IsPreScore(nm.GetString("status")) {
			continue
		}
		seedNextMatch(nm, roundWinners, i)
		if err := svc.app.Save(nm); err != nil {
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

func seedNextMatch(nm *core.Record, winners []string, matchIdx int) {
	p1Idx := matchIdx * 2
	p2Idx := matchIdx*2 + 1
	if p1Idx < len(winners) && winners[p1Idx] != "" {
		nm.Set("pair1", winners[p1Idx])
	}
	if p2Idx < len(winners) && winners[p2Idx] != "" {
		nm.Set("pair2", winners[p2Idx])
	}
}
