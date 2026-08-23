package league

import "github.com/pocketbase/pocketbase/core"

type Round struct {
	Number  int
	Matches []RoundMatch
}

type RoundMatch struct {
	Home string
	Away string
}

func RoundRobin(pairIDs []string, double bool) []Round {
	n := len(pairIDs)
	if n < 2 {
		return nil
	}

	pairs := make([]string, len(pairIDs))
	copy(pairs, pairIDs)

	if n%2 == 1 {
		pairs = append(pairs, "")
		n++
	}

	rounds := make([]Round, 0, n-1)
	for r := 0; r < n-1; r++ {
		var matches []RoundMatch
		for i := 0; i < n/2; i++ {
			home := pairs[i]
			away := pairs[n-1-i]
			if home != "" && away != "" {
				matches = append(matches, RoundMatch{Home: home, Away: away})
			}
		}
		rounds = append(rounds, Round{Number: r + 1, Matches: matches})
		last := pairs[n-1]
		copy(pairs[2:], pairs[1:n-1])
		pairs[1] = last
	}

	if double {
		half := len(rounds)
		for i := 0; i < half; i++ {
			var swapped []RoundMatch
			for _, m := range rounds[i].Matches {
				swapped = append(swapped, RoundMatch{Home: m.Away, Away: m.Home})
			}
			rounds = append(rounds, Round{Number: half + i + 1, Matches: swapped})
		}
	}

	return rounds
}

func (svc *Service) AdvancePlayoff(matchRecord *core.Record) error {
	compID := matchRecord.GetString("competition")
	comp, err := svc.App.FindRecordById("competitions", compID)
	if err != nil || comp.GetString("type") != "playoff" {
		return nil
	}

	currentRound := int(matchRecord.GetFloat("round_number"))

	roundMatches, _ := svc.App.FindRecordsByFilter("matches",
		"competition = {:cid} && round_number = {:rn}", "created", 0, 0,
		map[string]any{"cid": compID, "rn": currentRound})

	for _, m := range roundMatches {
		if m.GetString("status") != "final" {
			return nil
		}
	}

	nextRound := currentRound + 1
	nextMatches, _ := svc.App.FindRecordsByFilter("matches",
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
		p1Idx := i * 2
		p2Idx := i*2 + 1
		if p1Idx < len(roundWinners) && roundWinners[p1Idx] != "" {
			nm.Set("pair1", roundWinners[p1Idx])
		}
		if p2Idx < len(roundWinners) && roundWinners[p2Idx] != "" {
			nm.Set("pair2", roundWinners[p2Idx])
		}
		if err := svc.App.Save(nm); err != nil {
			return err
		}
	}

	return nil
}
