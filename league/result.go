package league

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

var (
	// ErrNoWinner indicates the score is valid but no side has won yet.
	ErrNoWinner = errors.New("league: no winner")
	// ErrMatchNotPreScore indicates the match is already past the scoring stage.
	ErrMatchNotPreScore = errors.New("league: match not in pre-score status")
)

// Outcome is what an accepted score means for the match.
type Outcome struct {
	Won        bool
	WinnerSide int
	Carried    string
}

// EvaluateScore applies the league rule to a parsed score: a full 2-set winner
// wins; an open-set leader wins only when ahead by 3+ games AND already holding
// one completed set. Anything else is "no terminado".
func EvaluateScore(sc Score) Outcome {
	if sc.Sets1 == 2 {
		return Outcome{Won: true, WinnerSide: 1}
	}
	if sc.Sets2 == 2 {
		return Outcome{Won: true, WinnerSide: 2}
	}

	carried := serializeCompletedSets(sc)

	if sc.Open != nil {
		diff := sc.Open.G1 - sc.Open.G2
		if diff >= 3 && sc.Sets1 >= 1 {
			return Outcome{Won: true, WinnerSide: 1}
		}
		if diff <= -3 && sc.Sets2 >= 1 {
			return Outcome{Won: true, WinnerSide: 2}
		}
	}

	return Outcome{Won: false, Carried: carried}
}

// ScoreNote returns an annotation for a score: empty for complete scores,
// a descriptive note for unfinished ones.
func ScoreNote(score string) string {
	sc, err := ParseScoreMode(score, AllowOpenSet)
	if err != nil {
		return ""
	}
	out := EvaluateScore(sc)
	if out.Won {
		return ""
	}
	if sc.Sets1+sc.Sets2 > 0 || sc.Open != nil {
		return "No terminado · se reanudará otro día"
	}
	return ""
}

// TallyScore parses a stored final score and returns per-side sets/games with
// the open set (if any) awarded to its leader. ok is false for walkovers and
// unparsable scores.
func TallyScore(score string) (Score, bool) {
	sc, err := ParseScoreMode(score, AllowOpenSet)
	if err != nil || (sc.Sets1 == 0 && sc.Sets2 == 0 && sc.Open == nil) {
		return Score{}, false
	}
	if sc.Open != nil {
		if sc.Open.G1 > sc.Open.G2 {
			sc.Sets1++
		} else if sc.Open.G2 > sc.Open.G1 {
			sc.Sets2++
		}
		sc.Open = nil
	}
	return sc, true
}

func serializeCompletedSets(sc Score) string {
	if len(sc.CompletedSets) == 0 {
		return ""
	}
	parts := make([]string, len(sc.CompletedSets))
	for i, s := range sc.CompletedSets {
		parts[i] = fmt.Sprintf("%d-%d", s[0], s[1])
	}
	return strings.Join(parts, " ")
}

// AcceptedResult describes who accepted which proposal.
type AcceptedResult struct {
	Proposal *core.Record
	ActorID  string // "" = accepted by quorum timeout
}

// ApplyAcceptedResult finalizes or suspends match according to the proposal's
// score, marks the proposal accepted, supersedes sibling proposals, writes the
// timeline entries and notifies both pairs.
func (svc *Service) ApplyAcceptedResult(match *core.Record, in AcceptedResult) (Outcome, error) {
	scores := parseProposalScores(in.Proposal.GetString("proposal_data"))
	if scores == "" {
		return Outcome{}, fmt.Errorf("apply result: empty proposal scores")
	}

	sc, err := ParseScoreMode(scores, AllowOpenSet)
	if err != nil {
		return Outcome{}, fmt.Errorf("apply result: %w", err)
	}
	out := EvaluateScore(sc)

	fresh, err := svc.app.FindRecordById("matches", match.Id)
	if err != nil {
		return Outcome{}, fmt.Errorf("apply result: %w", err)
	}
	if !IsPreScore(fresh.GetString("status")) {
		return Outcome{}, ErrMatchNotPreScore
	}

	compName := CompetitionName(svc.app, fresh.GetString("competition"))

	c := resultCtx{fresh: fresh, in: in, out: out, scores: scores, compName: compName}
	if out.Won {
		if err := svc.applyWon(c); err != nil {
			return Outcome{}, err
		}
	} else {
		if err := svc.applyNotWon(c, sc); err != nil {
			return Outcome{}, err
		}
	}

	return out, nil
}

type resultCtx struct {
	fresh            *core.Record
	in               AcceptedResult
	out              Outcome
	scores, compName string
}

func (svc *Service) logResultAccepted(c resultCtx) {
	if c.in.ActorID == "" {
		AddSystemResultAccepted(svc.app, c.fresh.Id, c.in.Proposal.Id, c.scores)
	} else {
		AddResultAccepted(svc.app, ResultAcceptedEntry{
			MatchID:  c.fresh.Id,
			ParentID: c.in.Proposal.Id,
			ActorID:  c.in.ActorID,
			Scores:   c.scores,
		})
	}
}

func (svc *Service) applyWon(c resultCtx) error {
	winnerID := c.fresh.GetString("pair1")
	if c.out.WinnerSide == 2 {
		winnerID = c.fresh.GetString("pair2")
	}

	err := svc.app.RunInTransaction(func(txApp core.App) error {
		freshProp, err := txApp.FindRecordById("match_messages", c.in.Proposal.Id)
		if err != nil {
			return fmt.Errorf("reload proposal: %w", err)
		}
		if freshProp.GetString("proposal_status") != "pending" {
			return fmt.Errorf("proposal already processed")
		}

		c.fresh.Set("scores", c.scores)
		c.fresh.Set("winner", winnerID)
		c.fresh.Set("status", StatusFinal)
		c.fresh.Set("carried_sets", "")
		if c.in.ActorID == "" {
			c.fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
		}
		if err := txApp.Save(c.fresh); err != nil {
			return fmt.Errorf("save match: %w", err)
		}

		freshProp.Set("proposal_status", "accepted")
		if err := txApp.Save(freshProp); err != nil {
			return fmt.Errorf("update proposal: %w", err)
		}

		return supersedeResults(txApp, c.fresh.Id, c.in.Proposal.Id)
	})
	if err != nil {
		return fmt.Errorf("apply result: %w", err)
	}

	svc.logResultAccepted(c)
	svc.notifyAccepted(c.fresh, c.in, c.compName)
	return nil
}

func (svc *Service) applyNotWon(c resultCtx, sc Score) error {
	err := svc.app.RunInTransaction(func(txApp core.App) error {
		freshProp, err := txApp.FindRecordById("match_messages", c.in.Proposal.Id)
		if err != nil {
			return fmt.Errorf("reload proposal: %w", err)
		}
		if freshProp.GetString("proposal_status") != "pending" {
			return fmt.Errorf("proposal already processed")
		}

		c.fresh.Set("carried_sets", c.out.Carried)
		c.fresh.Set("date", "")
		c.fresh.Set("time", "")
		c.fresh.Set("status", StatusPending)
		c.fresh.Set("last_warn_level", 0)
		c.fresh.Set("submitted_by", "")
		c.fresh.Set("submitted_at", "")
		if c.in.ActorID == "" {
			c.fresh.Set("dispute_notes", "Auto-confirmado por tiempo de espera")
		}
		if err := txApp.Save(c.fresh); err != nil {
			return fmt.Errorf("save match: %w", err)
		}

		freshProp.Set("proposal_status", "accepted")
		if err := txApp.Save(freshProp); err != nil {
			return fmt.Errorf("update proposal: %w", err)
		}

		if err := supersedeResults(txApp, c.fresh.Id, c.in.Proposal.Id); err != nil {
			return err
		}

		return supersedeSchedulingProposals(txApp, c.fresh.Id)
	})
	if err != nil {
		return fmt.Errorf("apply result: %w", err)
	}

	ClearMatchReminders(svc.app, c.fresh.Id)

	svc.logResultAccepted(c)

	setNum := len(sc.CompletedSets) + 1
	systemText := fmt.Sprintf("Partido no terminado: %s · se reanuda desde %s, %d.º set desde 0-0", c.scores, c.out.Carried, setNum)
	AddSystemResultEvent(svc.app, c.fresh.Id, systemText)

	svc.notifyNotWon(c.fresh, c.out, c.compName, sc)
	return nil
}

func supersedeResults(app core.App, matchID, acceptedID string) error {
	siblings, _ := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending' && id != {:eid}",
		"", 0, 0,
		map[string]any{"mid": matchID, "eid": acceptedID})
	for _, s := range siblings {
		s.Set("proposal_status", "superseded")
		if err := app.Save(s); err != nil {
			return fmt.Errorf("supersede sibling result: %w", err)
		}
	}
	return nil
}

func supersedeSchedulingProposals(app core.App, matchID string) error {
	accepted, _ := app.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'scheduling_proposal' && proposal_status = 'accepted'",
		"", 0, 0, map[string]any{"mid": matchID})
	for _, sp := range accepted {
		sp.Set("proposal_status", "superseded")
		if err := app.Save(sp); err != nil {
			return fmt.Errorf("supersede scheduling proposal: %w", err)
		}
	}
	return nil
}

func (svc *Service) notifyAccepted(fresh *core.Record, in AcceptedResult, compName string) {
	pair1ID := fresh.GetString("pair1")
	pair2ID := fresh.GetString("pair2")

	if in.ActorID != "" {
		team, err := PlayerTeam(svc.app, in.ActorID, fresh)
		if err != nil {
			slog.Error("apply result: resolve actor team", "match", fresh.Id, "err", err)
			return
		}
		proposerPairID := pair1ID
		responderPairID := pair2ID
		if team == 1 {
			proposerPairID = pair2ID
			responderPairID = pair1ID
		}
		responderName := PairNames(svc.app, []string{responderPairID})[responderPairID]
		proposerPlayers := PlayersForPair(svc.app, proposerPairID)
		svc.notifier.NotifyPlayers(proposerPlayers, NotifResultConfirmed(fresh.Id, responderName, compName))
		return
	}

	for _, pid := range []string{pair1ID, pair2ID} {
		players := PlayersForPair(svc.app, pid)
		svc.notifier.NotifyPlayers(players, Notification{
			Type:    "general",
			Title:   "Resultado confirmado automáticamente",
			Body:    fmt.Sprintf("El resultado ha sido confirmado por tiempo de espera · %s.", compName),
			MatchID: fresh.Id,
		})
	}
}

func (svc *Service) notifyNotWon(fresh *core.Record, out Outcome, compName string, sc Score) {
	setNum := len(sc.CompletedSets) + 1
	body := fmt.Sprintf("Se reanuda desde %s (%d.º set desde 0-0). Acordad una nueva fecha · %s.", out.Carried, setNum, compName)
	for _, pid := range []string{fresh.GetString("pair1"), fresh.GetString("pair2")} {
		players := PlayersForPair(svc.app, pid)
		svc.notifier.NotifyPlayers(players, Notification{
			Type:    "scheduling",
			Title:   "Partido por reanudar",
			Body:    body,
			MatchID: fresh.Id,
		})
	}
}
