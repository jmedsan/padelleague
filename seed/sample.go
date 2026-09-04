package seed

import (
	"bytes"
	"fmt"
	"io/fs"
	"time"

	"padelleague/league"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// SampleOptions selects which cumulative example stages SampleLeaguePartial
// loads. Each stage requires the previous one: pairs need players, a
// competition needs pairs, played matches need a competition.
type SampleOptions struct {
	Players      bool
	Pairs        bool
	Competitions bool
	Matches      bool
	Playoff      bool
	StaticFS     fs.FS
}

// SampleLeaguePartial loads sample data up to the highest selected stage:
// players → pairs → competition with rounds → early rounds played. With
// Players false it loads nothing. Stages below the highest selected are always
// included (the caller's UI enforces a contiguous selection).
func SampleLeaguePartial(app core.App, opts SampleOptions) error {
	if !opts.Players {
		return nil
	}
	return app.RunInTransaction(func(txApp core.App) error {
		playerIDs, err := createSamplePlayers(txApp, opts)
		if err != nil || !opts.Pairs {
			return err
		}
		pairIDs, err := createSamplePairs(txApp, playerIDs)
		if err != nil || !opts.Competitions {
			return err
		}
		comp, err := createSampleCompetition(txApp, pairIDs, opts.StaticFS)
		if err != nil {
			return err
		}
		if err := populateSampleLeague(txApp, comp, pairIDs, opts); err != nil {
			return err
		}
		if err := createMixedCompetition(txApp, pairIDs[:1]); err != nil {
			return err
		}
		return nil
	})
}

func populateSampleLeague(txApp core.App, comp *core.Record, pairIDs []string, opts SampleOptions) error {
	if err := createSampleFixtures(txApp, comp, pairIDs, opts.Matches); err != nil {
		return err
	}
	if err := saveSampleSchedule(txApp, comp); err != nil {
		return err
	}
	if err := createSampleDocuments(txApp, comp, opts.StaticFS); err != nil {
		return err
	}
	if err := ackSampleDocuments(txApp, comp, pairIDs); err != nil {
		return err
	}
	if err := createSampleVenues(txApp); err != nil {
		return err
	}
	if err := createSampleSponsor(txApp, comp, opts.StaticFS); err != nil {
		return err
	}
	if err := createSampleInvitations(txApp, comp.Id); err != nil {
		return err
	}
	if opts.Playoff {
		if err := createSamplePlayoff(txApp, pairIDs); err != nil {
			return err
		}
	}
	return nil
}

func createSamplePlayers(txApp core.App, opts SampleOptions) ([]string, error) {
	admins, err := txApp.FindRecordsByFilter("users",
		"roles ~ 'admin' && roles ~ 'player'", "created", 0, 0)
	if err != nil {
		admins = nil
	}
	sampleCount := 8 - len(admins)
	if sampleCount < 0 {
		sampleCount = 0
	}

	col, err := txApp.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	genders := []string{"male", "female", "male", "male", "female", "female", "male", "female"}
	ids := make([]string, 0, 8)
	for i := range sampleCount {
		rec := core.NewRecord(col)
		rec.Set("email", fmt.Sprintf("sample-p%d@padelleague.com", i+1))
		rec.SetPassword(SamplePlayerPassword)
		rec.Set("roles", []string{"player"})
		rec.Set("display_name", fmt.Sprintf("Jugador %d", i+1))
		rec.Set("gender", genders[i])
		rec.SetVerified(true)
		if i < 5 && opts.StaticFS != nil {
			f, err := loadSampleAvatar(opts.StaticFS, i+1)
			if err != nil {
				return nil, fmt.Errorf("load sample avatar %d: %w", i+1, err)
			}
			rec.Set("avatar", f)
		}
		if err := txApp.Save(rec); err != nil {
			return nil, fmt.Errorf("create player %d: %w", i+1, err)
		}
		ids = append(ids, rec.Id)
	}
	for _, a := range admins {
		ids = append(ids, a.Id)
	}
	return ids, nil
}

func createSamplePairs(txApp core.App, playerIDs []string) ([]string, error) {
	col, err := txApp.FindCollectionByNameOrId("pairs")
	if err != nil {
		return nil, err
	}
	names := []string{"Pareja A", "Pareja B", "Pareja C", "Pareja D"}
	ids := make([]string, 4)
	for i, name := range names {
		rec := core.NewRecord(col)
		rec.Set("name", name)
		rec.Set("player1", playerIDs[i*2])
		rec.Set("player2", playerIDs[i*2+1])
		if err := txApp.Save(rec); err != nil {
			return nil, fmt.Errorf("create pair %s: %w", name, err)
		}
		ids[i] = rec.Id
	}
	return ids, nil
}

func createMixedCompetition(txApp core.App, pairIDs []string) error {
	col, err := txApp.FindCollectionByNameOrId("competitions")
	if err != nil {
		return err
	}
	comp := core.NewRecord(col)
	comp.Set("name", "Liga mixta de ejemplo")
	comp.Set("type", "league")
	comp.Set("gender_type", "mixed")
	comp.Set("active", true)
	comp.Set("pairs", pairIDs)
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("create mixed competition: %w", err)
	}
	return nil
}

func createSampleCompetition(txApp core.App, pairIDs []string, staticFS fs.FS) (*core.Record, error) {
	col, err := txApp.FindCollectionByNameOrId("competitions")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	payment := make(map[string]bool, len(pairIDs))
	for i, pid := range pairIDs {
		payment[pid] = i != len(pairIDs)-1
	}
	comp := core.NewRecord(col)
	comp.Set("name", "Dale Fuerte a la Bola")
	comp.Set("type", "league")
	comp.Set("gender_type", "free")
	comp.Set("active", true)
	comp.Set("play_twice", true)
	comp.Set("pairs", pairIDs)
	// Mid-season: started 20 days ago with 10 days left, so the pending rounds'
	// arrange deadlines are near and sample players see live tasks on their home.
	comp.Set("start_date", now.Add(-20*24*time.Hour))
	comp.Set("end_date", now.Add(10*24*time.Hour))
	comp.Set("payment_status", payment)
	if staticFS != nil {
		f, err := loadSampleLogo(staticFS)
		if err != nil {
			return nil, fmt.Errorf("load sample logo: %w", err)
		}
		comp.Set("logo", f)
	}
	if err := txApp.Save(comp); err != nil {
		return nil, fmt.Errorf("create competition: %w", err)
	}
	return comp, nil
}

func loadSampleLogo(staticFS fs.FS) (*filesystem.File, error) {
	data, err := fs.ReadFile(staticFS, "static/img/logo.jpg")
	if err != nil {
		return nil, err
	}
	return league.CompressLogoBytes(bytes.NewReader(data), "sample-logo.jpg")
}

// sampleCtx bundles collections and timing needed while building sample matches.
type sampleCtx struct {
	app      core.App
	matchCol *core.Collection
	msgCol   *core.Collection
	compID   string
	start    time.Time
}

// createSampleFixtures builds the round-robin matches. When played is true,
// rounds 1–4 are finalized and round 5 carries a live dispute (match 0) and a
// score awaiting confirmation (match 1), so sample players get real home tasks;
// the rest stay pending. All states are set at CREATE time so the match
// status-transition hook (which only guards updates) is never violated.
// Each non-pending match also gets chronologically realistic timeline entries
// (scheduling proposal → acceptance → result events).
func createSampleFixtures(txApp core.App, comp *core.Record, pairIDs []string, played bool) error {
	rounds := league.RoundRobin(pairIDs, true)
	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	msgCol, err := txApp.FindCollectionByNameOrId("match_messages")
	if err != nil {
		return err
	}
	sc := sampleCtx{
		app: txApp, matchCol: matchCol, msgCol: msgCol,
		compID: comp.Id, start: comp.GetDateTime("start_date").Time(),
	}
	for _, round := range rounds {
		for i, m := range round.Matches {
			f := sampleFixture{round: round.Number, idx: i, home: m.Home, away: m.Away, played: played}
			if err := sc.createMatch(f); err != nil {
				return err
			}
		}
	}
	comp.Set("rounds", len(rounds))
	if played {
		return createSampleNotifications(txApp, comp)
	}
	return nil
}

// sampleFixture describes one match to seed and how "played" it should be.
type sampleFixture struct {
	round, idx int
	home, away string
	played     bool
}

func (sc *sampleCtx) createMatch(f sampleFixture) error {
	match := core.NewRecord(sc.matchCol)
	match.Set("competition", sc.compID)
	match.Set("round_number", f.round)
	match.Set("matches_to_win", 1)
	match.Set("pair1", f.home)
	match.Set("pair2", f.away)
	if err := setSampleMatchState(sc.app, match, f); err != nil {
		return err
	}
	if err := sc.app.Save(match); err != nil {
		return fmt.Errorf("create match round %d: %w", f.round, err)
	}
	if f.played && f.round <= 6 {
		if err := sc.createTimeline(match, f); err != nil {
			return fmt.Errorf("create timeline round %d: %w", f.round, err)
		}
	}
	return nil
}

// setSampleMatchState sets a match's status at create time: rounds 1–4
// finalized when played; round 5 match 0 disputed and match 1 awaiting
// confirmation; everything else pending.
func setSampleMatchState(txApp core.App, match *core.Record, f sampleFixture) error {
	switch {
	case f.played && f.round <= 4:
		return finalizeSampleMatch(txApp, match)
	case f.played && f.round == 5 && f.idx == 0:
		return disputeSampleMatch(txApp, match)
	case f.played && f.round == 5 && f.idx == 1:
		sub, err := firstPlayerOfPair(txApp, match.GetString("pair1"))
		if err != nil {
			return err
		}
		match.Set("scores", "6-2 6-2")
		match.Set("submitted_by", sub)
		match.Set("submitted_at", time.Now().UTC().Add(-24*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
		match.Set("status", league.StatusPending)
		return nil
	case f.played && f.round == 6 && f.idx == 0:
		match.Set("status", league.StatusScheduled)
		return nil
	case f.played && f.round == 6 && f.idx == 1:
		return walkoverSampleMatch(txApp, match)
	default:
		match.Set("status", league.StatusPending)
		return nil
	}
}

func finalizeSampleMatch(txApp core.App, match *core.Record) error {
	winner, err := league.DetermineWinner(match, "6-3 6-3")
	if err != nil {
		return fmt.Errorf("determine winner: %w", err)
	}
	sub, err := firstPlayerOfPair(txApp, match.GetString("pair1"))
	if err != nil {
		return err
	}
	conf, err := firstPlayerOfPair(txApp, match.GetString("pair2"))
	if err != nil {
		return err
	}
	match.Set("scores", "6-3 6-3")
	match.Set("winner", winner)
	match.Set("submitted_by", sub)
	match.Set("confirmed_by", conf)
	match.Set("status", league.StatusFinal)
	return nil
}

func submitSampleScore(txApp core.App, match *core.Record, status, scores string) error {
	sub, err := firstPlayerOfPair(txApp, match.GetString("pair1"))
	if err != nil {
		return err
	}
	match.Set("scores", scores)
	match.Set("submitted_by", sub)
	match.Set("status", status)
	return nil
}

// disputeSampleMatch models a full dispute: pair1 submitted a score and pair2
// disputed it with a note, so the admin sees both the submitted result and the
// opponent's objection to resolve.
func disputeSampleMatch(txApp core.App, match *core.Record) error {
	if err := submitSampleScore(txApp, match, league.StatusDisputed, "6-4 4-6 7-5"); err != nil {
		return err
	}
	disputer, err := firstPlayerOfPair(txApp, match.GetString("pair2"))
	if err != nil {
		return err
	}
	match.Set("disputed_by", disputer)
	match.Set("disputed_scores", "6-4 4-6 5-7")
	match.Set("dispute_notes", "No estoy de acuerdo: el tercer set fue 5-7, no 7-5.")
	return nil
}

func walkoverSampleMatch(txApp core.App, match *core.Record) error {
	winner := match.GetString("pair1")
	loser := match.GetString("pair2")
	requester, err := firstPlayerOfPair(txApp, loser)
	if err != nil {
		return err
	}
	match.Set("scores", "6-0 6-0")
	match.Set("winner", winner)
	match.Set("status", league.StatusFinal)
	match.Set("review_type", "walkover")
	match.Set("walkover_requested_by", requester)
	match.Set("dispute_notes", "[No jugado] El rival no se presentó.")

	compID := match.GetString("competition")
	if err := league.ApplyPenalty(txApp, league.PenaltyInput{
		CompetitionID: compID,
		PairID:        loser,
		Reason:        "Incomparecencia aprobada",
		AdminID:       "",
		Amount:        3,
	}); err != nil {
		return fmt.Errorf("apply walkover penalty: %w", err)
	}
	return nil
}

// createSampleTimeline populates a match thread with realistic entries:
// a scheduling proposal (accepted), then the result events matching the
// match state. Timestamps are derived from the competition start so the
// thread reads chronologically.
func (sc *sampleCtx) createTimeline(match *core.Record, f sampleFixture) error {
	roundBase := sc.start.Add(time.Duration(f.round-1) * 4 * 24 * time.Hour)
	proposer, _ := firstPlayerOfPair(sc.app, match.GetString("pair1"))
	responder, _ := firstPlayerOfPair(sc.app, match.GetString("pair2"))

	venues := []string{"Padel 360", "Wurko", "Tecnisur"}
	venue := venues[(f.round+f.idx)%len(venues)]
	playDate := roundBase.Add(3 * 24 * time.Hour)

	sched := schedulingContext{match, roundBase, playDate, venue, proposer, responder}
	if err := sc.createSchedulingEntries(sched); err != nil {
		return err
	}

	return sc.createResultEntries(resultContext{match, f, proposer, responder, playDate})
}

type schedulingContext struct {
	match               *core.Record
	roundBase, playDate time.Time
	venue               string
	proposer, responder string
}

// createSchedulingEntries writes the proposal + acceptance timeline entries
// for a match, and mirrors the acceptance onto the match record itself (date
// header card reads match.date directly rather than the thread history).
func (sc *sampleCtx) createSchedulingEntries(c schedulingContext) error {
	proposalTime := c.roundBase.Add(1 * 24 * time.Hour)
	pdJSON := fmt.Sprintf(`{"date":"%s","time":"20:00","venue_name":"%s"}`,
		c.playDate.Format("02/01/2006"), c.venue)
	proposal := core.NewRecord(sc.msgCol)
	proposal.Set("match", c.match.Id)
	proposal.Set("author", c.proposer)
	proposal.Set("type", "scheduling_proposal")
	proposal.Set("proposal_data", pdJSON)
	proposal.Set("proposal_status", "accepted")
	proposal.SetRaw("created", proposalTime.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := sc.app.Save(proposal); err != nil {
		return fmt.Errorf("save proposal: %w", err)
	}

	responderLabel := sampleLabel(sc.app, c.responder, c.match)
	proposerName := league.PlayerName(sc.app, c.proposer)
	acceptTime := proposalTime.Add(4 * time.Hour)
	acceptDetail := fmt.Sprintf("%s aceptó la propuesta de %s (%s, %s, %s)",
		responderLabel, proposerName, c.playDate.Format("02/01/2006"), "20:00", c.venue)
	respPDJSON := fmt.Sprintf(`{"action":"accept","date":"%s","time":"20:00","venue_name":"%s"}`,
		c.playDate.Format("02/01/2006"), c.venue)
	resp := core.NewRecord(sc.msgCol)
	resp.Set("match", c.match.Id)
	resp.Set("author", c.responder)
	resp.Set("type", "scheduling_response")
	resp.Set("content", acceptDetail)
	resp.Set("parent", proposal.Id)
	resp.Set("proposal_data", respPDJSON)
	resp.SetRaw("created", acceptTime.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := sc.app.Save(resp); err != nil {
		return fmt.Errorf("save scheduling response: %w", err)
	}

	// Reload from the DB first: match was created via NewRecord() and never
	// PostScan()-ed, so its Original() snapshot is blank and the
	// status-transition hook would reject this update.
	stored, err := sc.app.FindRecordById(sc.matchCol, c.match.Id)
	if err != nil {
		return fmt.Errorf("reload match for schedule: %w", err)
	}
	stored.Set("date", c.playDate.Format("2006-01-02"))
	stored.Set("time", "20:00")
	stored.Set("club", c.venue)
	if err := sc.app.Save(stored); err != nil {
		return fmt.Errorf("save match schedule: %w", err)
	}
	return nil
}

type resultContext struct {
	match                *core.Record
	f                    sampleFixture
	submitter, responder string
	playDate             time.Time
}

func (sc *sampleCtx) createResultEntries(rc resultContext) error {
	scores := rc.match.GetString("scores")

	switch {
	case rc.f.round <= 4:
		submitTime := rc.playDate.Add(22 * time.Hour)
		proposal, err := sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.submitter, scores, "accepted", submitTime})
		if err != nil {
			return err
		}
		confirmerLabel := sampleLabel(sc.app, rc.responder, rc.match)
		confirmTime := submitTime.Add(2 * time.Hour)
		return sc.saveResultResponse(resultResponseArgs{
			matchID: rc.match.Id, authorID: rc.responder, parentID: proposal.Id,
			action: "accept", content: confirmerLabel + " aceptó el resultado", scores: scores, ts: confirmTime,
		})

	case rc.f.round == 5 && rc.f.idx == 0:
		submitTime := rc.playDate.Add(22 * time.Hour)
		proposal, err := sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.submitter, scores, "superseded", submitTime})
		if err != nil {
			return err
		}
		disputeTime := submitTime.Add(1 * time.Hour)
		disputerLabel := sampleLabel(sc.app, rc.responder, rc.match)
		if err := sc.saveResultResponse(resultResponseArgs{
			matchID: rc.match.Id, authorID: rc.responder, parentID: proposal.Id,
			action: "reject", content: disputerLabel + " rechazó el resultado", scores: scores, ts: disputeTime,
		}); err != nil {
			return err
		}
		_, err = sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.responder, "6-4 4-6 5-7", "pending", disputeTime})
		return err

	case rc.f.round == 5 && rc.f.idx == 1:
		submitTime := rc.playDate.Add(22 * time.Hour)
		_, err := sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.submitter, scores, "pending", submitTime})
		return err

	case rc.f.round == 6 && rc.f.idx == 1:
		return sc.saveWalkoverTimeline(rc)
	case rc.f.round == 6:
		return nil
	}
	return nil
}

// saveWalkoverTimeline records the admin's walkover-approval line for a
// sample match, attributed to a real admin user when one exists in the
// seeded data (falling back to the submitter otherwise).
func (sc *sampleCtx) saveWalkoverTimeline(rc resultContext) error {
	reportTime := rc.playDate.Add(20 * time.Hour)
	adminID := rc.submitter
	if admins, err := sc.app.FindRecordsByFilter("users", "roles ~ 'admin'", "", 1, 0, nil); err == nil && len(admins) > 0 {
		adminID = admins[0].Id
	}
	rec := core.NewRecord(sc.msgCol)
	rec.Set("match", rc.match.Id)
	rec.Set("author", adminID)
	rec.Set("type", "result_event")
	winnerName := league.PairNames(sc.app, []string{rc.match.GetString("pair1")})[rc.match.GetString("pair1")]
	rec.Set("content", "aprobó incomparecencia a favor de "+winnerName)
	rec.SetRaw("created", reportTime.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := sc.app.Save(rec); err != nil {
		return fmt.Errorf("save walkover timeline: %w", err)
	}
	return nil
}

type resultProposalArgs struct {
	matchID, authorID, scores, status string
	ts                                time.Time
}

func (sc *sampleCtx) saveResultProposal(a resultProposalArgs) (*core.Record, error) {
	pdJSON := fmt.Sprintf(`{"scores":"%s"}`, a.scores)
	rec := core.NewRecord(sc.msgCol)
	rec.Set("match", a.matchID)
	rec.Set("author", a.authorID)
	rec.Set("type", "result_submission")
	rec.Set("content", a.scores)
	rec.Set("proposal_data", pdJSON)
	rec.Set("proposal_status", a.status)
	rec.SetRaw("created", a.ts.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := sc.app.Save(rec); err != nil {
		return nil, fmt.Errorf("save result proposal: %w", err)
	}
	return rec, nil
}

type resultResponseArgs struct {
	matchID, authorID, parentID, action, content, scores string
	ts                                                   time.Time
}

func (sc *sampleCtx) saveResultResponse(a resultResponseArgs) error {
	rec := core.NewRecord(sc.msgCol)
	rec.Set("match", a.matchID)
	rec.Set("author", a.authorID)
	rec.Set("type", "result_response")
	rec.Set("content", a.content)
	rec.Set("parent", a.parentID)
	rec.Set("proposal_data", fmt.Sprintf(`{"action":"%s","scores":"%s"}`, a.action, a.scores))
	rec.SetRaw("created", a.ts.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := sc.app.Save(rec); err != nil {
		return fmt.Errorf("save result response: %w", err)
	}
	return nil
}

func sampleLabel(txApp core.App, userID string, match *core.Record) string {
	name := league.PlayerName(txApp, userID)
	team, err := league.PlayerTeam(txApp, userID, match)
	if err != nil || team == 0 {
		return name
	}
	pairID := match.GetString("pair1")
	if team == 2 {
		pairID = match.GetString("pair2")
	}
	pairName := league.PairNames(txApp, []string{pairID})[pairID]
	return fmt.Sprintf("%s (%s)", name, pairName)
}

type notifyFn func(userIDs []string, ntype, title, body, matchID string, read bool) error

func newNotifyFn(txApp core.App) (notifyFn, error) {
	col, err := txApp.FindCollectionByNameOrId("notifications")
	if err != nil {
		return nil, err
	}
	return func(userIDs []string, ntype, title, body, matchID string, read bool) error {
		for _, uid := range userIDs {
			n := core.NewRecord(col)
			n.Set("user", uid)
			n.Set("type", ntype)
			n.Set("title", title)
			n.Set("body", body)
			n.Set("read", read)
			if matchID != "" {
				n.Set("related_match", matchID)
				n.Set("link", "/match/"+matchID)
			}
			if err := txApp.Save(n); err != nil {
				return fmt.Errorf("create sample notification: %w", err)
			}
		}
		return nil
	}, nil
}

func createSampleNotifications(txApp core.App, comp *core.Record) error {
	notify, err := newNotifyFn(txApp)
	if err != nil {
		return err
	}
	if err := notifyFinalizedMatches(txApp, comp, notify); err != nil {
		return err
	}
	return notifyLiveMatches(txApp, comp, notify)
}

func notifyFinalizedMatches(txApp core.App, comp *core.Record, notify notifyFn) error {
	compName := comp.GetString("name")
	finals, _ := txApp.FindRecordsByFilter("matches",
		"competition = {:cid} && status = 'final'", "round_number",
		0, 0, map[string]any{"cid": comp.Id})
	for _, m := range finals {
		p1 := m.GetString("pair1")
		p2 := m.GetString("pair2")
		p1Name := league.PairNames(txApp, []string{p1})[p1]
		p2Name := league.PairNames(txApp, []string{p2})[p2]
		nSubmit := league.NotifResultSubmitted(m.Id, p1Name, compName, m.GetString("scores"))
		if err := notify(playersOfPair(txApp, p2), nSubmit.Type, nSubmit.Title, nSubmit.Body, m.Id, true); err != nil {
			return err
		}
		nConfirm := league.NotifResultConfirmed(m.Id, p2Name, compName)
		if err := notify(playersOfPair(txApp, p1), nConfirm.Type, nConfirm.Title, nConfirm.Body, m.Id, true); err != nil {
			return err
		}
	}
	return nil
}

func notifyLiveMatches(txApp core.App, comp *core.Record, notify notifyFn) error {
	if disputed := sampleMatchByStatus(txApp, comp.Id, league.StatusDisputed); disputed != nil {
		players := append(playersOfPair(txApp, disputed.GetString("pair1")),
			playersOfPair(txApp, disputed.GetString("pair2"))...)
		if err := notify(players, "dispute", "Partido disputado", "Hay una disputa abierta en tu partido.", disputed.Id, false); err != nil {
			return err
		}
	}
	if awaiting := sampleMatchWithPendingResult(txApp, comp.Id); awaiting != nil {
		if err := notifyAwaitingResult(txApp, comp, awaiting, notify); err != nil {
			return err
		}
	}
	return nil
}

// notifyAwaitingResult notifies the pair that has not yet responded to a
// pending result proposal, mirroring the live NotifResultSubmitted copy.
func notifyAwaitingResult(txApp core.App, comp, awaiting *core.Record, notify func(userIDs []string, ntype, title, body, matchID string, read bool) error) error {
	submitterPairID := submitterPairID(txApp, awaiting)
	opponentPairID := awaiting.GetString("pair2")
	if submitterPairID == opponentPairID {
		opponentPairID = awaiting.GetString("pair1")
	}
	submitterName := league.PairNames(txApp, []string{submitterPairID})[submitterPairID]
	scores := pendingResultScores(txApp, awaiting.Id)
	n := league.NotifResultSubmitted(awaiting.Id, submitterName, comp.GetString("name"), scores)
	return notify(playersOfPair(txApp, opponentPairID), n.Type, n.Title, n.Body, awaiting.Id, false)
}

func submitterPairID(txApp core.App, match *core.Record) string {
	pairID := match.GetString("pair1")
	submittedBy := match.GetString("submitted_by")
	if submittedBy == "" {
		return pairID
	}
	if team, err := league.PlayerTeam(txApp, submittedBy, match); err == nil && team == 2 {
		return match.GetString("pair2")
	}
	return pairID
}

func pendingResultScores(txApp core.App, matchID string) string {
	msgs, _ := txApp.FindRecordsByFilter("match_messages",
		"match = {:mid} && type = 'result_submission' && proposal_status = 'pending'",
		"-created", 1, 0,
		map[string]any{"mid": matchID})
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].GetString("content")
}

func sampleMatchByStatus(txApp core.App, compID, status string) *core.Record {
	ms, _ := txApp.FindRecordsByFilter("matches",
		"competition = {:cid} && status = {:s}", "", 1, 0,
		map[string]any{"cid": compID, "s": status})
	if len(ms) == 0 {
		return nil
	}
	return ms[0]
}

func sampleMatchWithPendingResult(txApp core.App, compID string) *core.Record {
	msgs, _ := txApp.FindRecordsByFilter("match_messages",
		"type = 'result_submission' && proposal_status = 'pending'",
		"", 0, 0, nil)
	for _, msg := range msgs {
		m, err := txApp.FindRecordById("matches", msg.GetString("match"))
		if err != nil {
			continue
		}
		if m.GetString("competition") == compID {
			return m
		}
	}
	return nil
}

func firstPlayerOfPair(txApp core.App, pairID string) (string, error) {
	pair, err := txApp.FindRecordById("pairs", pairID)
	if err != nil {
		return "", err
	}
	return pair.GetString("player1"), nil
}

func playersOfPair(txApp core.App, pairID string) []string {
	pair, err := txApp.FindRecordById("pairs", pairID)
	if err != nil {
		return nil
	}
	return []string{pair.GetString("player1"), pair.GetString("player2")}
}

// createReglamentoPDFDoc creates the bundled sample reglamento as a file document
// and returns its ID, or "" when no PDF is available (link docs still get added).
func createReglamentoPDFDoc(txApp core.App, col *core.Collection, staticFS fs.FS) (string, error) {
	if staticFS == nil {
		return "", nil
	}
	b, err := fs.ReadFile(staticFS, "static/docs/reglamento-liga-amistosa.pdf")
	if err != nil {
		return "", nil
	}
	f, err := filesystem.NewFileFromBytes(b, "reglamento-liga-amistosa.pdf")
	if err != nil {
		return "", nil
	}
	rec := core.NewRecord(col)
	rec.Set("title", "Reglamento de la liga (amistosa)")
	rec.Set("is_mandatory", true)
	rec.Set("is_default", true)
	rec.Set("file", f)
	if err := txApp.Save(rec); err != nil {
		return "", fmt.Errorf("create document reglamento PDF: %w", err)
	}
	return rec.Id, nil
}

func createSampleDocuments(txApp core.App, comp *core.Record, staticFS fs.FS) error {
	col, err := txApp.FindCollectionByNameOrId("documents")
	if err != nil {
		return fmt.Errorf("find documents collection: %w", err)
	}
	var docIDs []string
	pdfID, err := createReglamentoPDFDoc(txApp, col, staticFS)
	if err != nil {
		return err
	}
	if pdfID != "" {
		docIDs = append(docIDs, pdfID)
	}
	type docSpec struct {
		title     string
		url       string
		mandatory bool
		isDefault bool
	}
	linkDocs := []docSpec{
		{title: "Reglamento FEP", url: "https://www.fep.es/noticias/reglamento-de-juego", mandatory: true, isDefault: true},
		{title: "Tutorial Padel", url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
	}
	for _, d := range linkDocs {
		rec := core.NewRecord(col)
		rec.Set("title", d.title)
		rec.Set("url", d.url)
		rec.Set("is_mandatory", d.mandatory)
		rec.Set("is_default", d.isDefault)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create document %s: %w", d.title, err)
		}
		docIDs = append(docIDs, rec.Id)
	}
	comp.Set("documents", docIDs)
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("attach documents to competition: %w", err)
	}
	return nil
}

func ackSampleDocuments(txApp core.App, comp *core.Record, pairIDs []string) error {
	allDocs := league.AttachedDocuments(txApp, comp)
	var docIDs []string
	for _, d := range allDocs {
		if d.GetBool("is_mandatory") {
			docIDs = append(docIDs, d.Id)
		}
	}
	if len(docIDs) == 0 {
		return nil
	}
	playerIDs := make(map[string]struct{})
	for _, pid := range pairIDs {
		for _, uid := range league.PlayersForPair(txApp, pid) {
			playerIDs[uid] = struct{}{}
		}
	}
	col, err := txApp.FindCollectionByNameOrId("document_acks")
	if err != nil {
		return fmt.Errorf("find document_acks collection: %w", err)
	}
	for uid := range playerIDs {
		rec := core.NewRecord(col)
		rec.Set("user", uid)
		rec.Set("competition", comp.Id)
		rec.Set("documents", docIDs)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create doc ack for %s: %w", uid, err)
		}
	}
	return nil
}

func createSamplePlayoff(txApp core.App, pairIDs []string) error {
	col, err := txApp.FindCollectionByNameOrId("competitions")
	if err != nil {
		return err
	}
	comp := core.NewRecord(col)
	comp.Set("name", "Playoff de ejemplo")
	comp.Set("type", "playoff")
	comp.Set("active", true)
	comp.Set("pairs", pairIDs)
	defaults, _ := txApp.FindRecordsByFilter("documents", "is_default = true", "", 0, 0, nil)
	if len(defaults) > 0 {
		ids := make([]string, len(defaults))
		for i, d := range defaults {
			ids[i] = d.Id
		}
		comp.Set("documents", ids)
	}
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("create sample playoff: %w", err)
	}
	return generateSampleBracket(txApp, comp.Id, pairIDs)
}
