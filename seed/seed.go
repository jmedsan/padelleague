// Package seed provides development/test data seeding and reset.
package seed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"padelleague/league"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// User describes a user to seed into the database.
type User struct {
	Email       string
	Password    string
	Collection  string
	Roles       []string
	DisplayName string
}

// Run creates any users that do not already exist in the database.
func Run(app core.App, users []User) {
	for _, u := range users {
		if u.Email == "" || u.Password == "" {
			continue
		}
		existing, _ := app.FindAuthRecordByEmail(u.Collection, u.Email)
		if existing != nil {
			continue
		}
		col, err := app.FindCollectionByNameOrId(u.Collection)
		if err != nil {
			slog.Error("seed collection not found", "collection", u.Collection, "err", err)
			continue
		}
		record := core.NewRecord(col)
		record.Set("email", u.Email)
		record.SetPassword(u.Password)
		if len(u.Roles) > 0 {
			record.Set("roles", u.Roles)
		}
		if u.DisplayName != "" {
			record.Set("display_name", u.DisplayName)
		}
		record.SetVerified(true)
		if err := app.Save(record); err != nil {
			slog.Error("seed create failed", "email", maskEmail(u.Email), "collection", u.Collection, "err", err)
		} else {
			slog.Info("seed created", "email", maskEmail(u.Email), "collection", u.Collection)
		}
	}
}

// WipeSummary reports how many records were deleted per collection.
type WipeSummary struct {
	Competitions  int
	Pairs         int
	Players       int
	Matches       int
	Messages      int
	Notifications int
	Invitations   int
	Subscriptions int
}

// Total returns the sum of all deleted records.
func (s WipeSummary) Total() int {
	return s.Competitions + s.Pairs + s.Players + s.Matches +
		s.Messages + s.Notifications + s.Invitations + s.Subscriptions
}

// SamplePlayerPassword is the password used for sample league players.
// Dev/test only — never used in production.
const SamplePlayerPassword = "padel1234"

// WipeOptions controls which data categories to delete.
type WipeOptions struct {
	Players      bool
	Pairs        bool
	Competitions bool
	Matches      bool
}

// WipeSelective deletes data for the selected categories inside a transaction.
func WipeSelective(app core.App, opts WipeOptions) (WipeSummary, error) {
	var summary WipeSummary
	err := app.RunInTransaction(func(txApp core.App) error {
		return wipeAll(txApp, opts, &summary)
	})
	return summary, err
}

func wipeAll(txApp core.App, opts WipeOptions, summary *WipeSummary) error {
	if opts.Matches {
		if err := wipeMatches(txApp, summary); err != nil {
			return err
		}
	}
	if opts.Competitions {
		if err := wipeCompetitions(txApp, summary); err != nil {
			return err
		}
	}
	if opts.Pairs {
		if err := wipeCollection(txApp, "pairs", &summary.Pairs); err != nil {
			return err
		}
	}
	if opts.Players {
		return wipePlayers(txApp, summary)
	}
	return nil
}

func wipeCompetitions(txApp core.App, summary *WipeSummary) error {
	if err := wipeCollection(txApp, "document_acks", new(int)); err != nil {
		return err
	}
	if err := wipeCollection(txApp, "documents", new(int)); err != nil {
		return err
	}
	if err := wipeCollection(txApp, "penalties", new(int)); err != nil {
		return err
	}
	return wipeCollection(txApp, "competitions", &summary.Competitions)
}

func wipeMatches(txApp core.App, summary *WipeSummary) error {
	if err := wipeCollection(txApp, "match_messages", &summary.Messages); err != nil {
		return err
	}
	return wipeCollection(txApp, "matches", &summary.Matches)
}

func wipePlayers(txApp core.App, summary *WipeSummary) error {
	if err := wipeCollection(txApp, "notifications", &summary.Notifications); err != nil {
		return err
	}
	if err := wipeCollection(txApp, "invitations", &summary.Invitations); err != nil {
		return err
	}
	if err := wipeCollection(txApp, "push_subscriptions", &summary.Subscriptions); err != nil {
		return err
	}
	return wipeNonAdminUsers(txApp, &summary.Players)
}

func wipeCollection(txApp core.App, name string, count *int) error {
	recs, err := txApp.FindRecordsByFilter(name, "id != ''", "", 0, 0)
	if err != nil {
		return fmt.Errorf("find %s: %w", name, err)
	}
	for _, rec := range recs {
		if err := txApp.Delete(rec); err != nil {
			return fmt.Errorf("delete %s %s: %w", name, rec.Id, err)
		}
		*count++
	}
	return nil
}

func wipeNonAdminUsers(txApp core.App, count *int) error {
	users, err := txApp.FindRecordsByFilter("users", "id != ''", "", 0, 0)
	if err != nil {
		return fmt.Errorf("find users: %w", err)
	}
	for _, u := range users {
		if slices.Contains(u.GetStringSlice("roles"), "admin") {
			continue
		}
		if err := txApp.Delete(u); err != nil {
			return fmt.Errorf("delete user %s: %w", u.Id, err)
		}
		*count++
	}
	return nil
}

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
		playerIDs, err := createSamplePlayers(txApp)
		if err != nil || !opts.Pairs {
			return err
		}
		pairIDs, err := createSamplePairs(txApp, playerIDs)
		if err != nil || !opts.Competitions {
			return err
		}
		comp, err := createSampleCompetition(txApp, pairIDs)
		if err != nil {
			return err
		}
		if err := populateSampleLeague(txApp, comp, pairIDs, opts); err != nil {
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
	if err := createSampleVenues(txApp); err != nil {
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

func createSamplePlayers(txApp core.App) ([]string, error) {
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
	ids := make([]string, 0, 8)
	for i := range sampleCount {
		rec := core.NewRecord(col)
		rec.Set("email", fmt.Sprintf("sample-p%d@padelleague.com", i+1))
		rec.SetPassword(SamplePlayerPassword)
		rec.Set("roles", []string{"player"})
		rec.Set("display_name", fmt.Sprintf("Jugador %d", i+1))
		rec.SetVerified(true)
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

func createSampleCompetition(txApp core.App, pairIDs []string) (*core.Record, error) {
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
	comp.Set("name", "Liga de ejemplo")
	comp.Set("type", "league")
	comp.Set("active", true)
	comp.Set("play_twice", true)
	comp.Set("pairs", pairIDs)
	// Mid-season: started 20 days ago with 10 days left, so the pending rounds'
	// arrange deadlines are near and sample players see live tasks on their home.
	comp.Set("start_date", now.Add(-20*24*time.Hour))
	comp.Set("end_date", now.Add(10*24*time.Hour))
	comp.Set("payment_status", payment)
	if err := txApp.Save(comp); err != nil {
		return nil, fmt.Errorf("create competition: %w", err)
	}
	return comp, nil
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
		return submitSampleScore(txApp, match, league.StatusConfirmed, "6-2 6-2")
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
		Reason:        "Walkover aprobado",
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

	// 1. Scheduling proposal
	proposalTime := roundBase.Add(1 * 24 * time.Hour)
	pdJSON := fmt.Sprintf(`{"date":"%s","time":"20:00","venue_name":"%s"}`,
		playDate.Format("02/01/2006"), venue)
	proposal := core.NewRecord(sc.msgCol)
	proposal.Set("match", match.Id)
	proposal.Set("author", proposer)
	proposal.Set("type", "scheduling_proposal")
	proposal.Set("proposal_data", pdJSON)
	proposal.Set("proposal_status", "accepted")
	proposal.Set("created", proposalTime.Format(time.RFC3339))
	if err := sc.app.Save(proposal); err != nil {
		return fmt.Errorf("save proposal: %w", err)
	}

	responderLabel := sampleLabel(sc.app, responder, match)
	proposerName := league.PlayerName(sc.app, proposer)
	acceptTime := proposalTime.Add(4 * time.Hour)
	acceptDetail := fmt.Sprintf("%s aceptó la propuesta de %s (%s, %s, %s)",
		responderLabel, proposerName, playDate.Format("02/01/2006"), "20:00", venue)
	resp := core.NewRecord(sc.msgCol)
	resp.Set("match", match.Id)
	resp.Set("author", responder)
	resp.Set("type", "scheduling_response")
	resp.Set("content", acceptDetail)
	resp.Set("parent", proposal.Id)
	resp.Set("created", acceptTime.Format(time.RFC3339))
	if err := sc.app.Save(resp); err != nil {
		return fmt.Errorf("save scheduling response: %w", err)
	}

	return sc.createResultEntries(resultContext{match, f, proposer, responder, playDate})
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
			action: "accept", content: confirmerLabel + " aceptó el resultado", ts: confirmTime,
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
			action: "reject", content: disputerLabel + " rechazó el resultado", ts: disputeTime,
		}); err != nil {
			return err
		}
		_, err = sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.responder, "6-4 4-6 5-7", "pending", disputeTime})
		return err

	case rc.f.round == 5 && rc.f.idx == 1:
		submitTime := rc.playDate.Add(22 * time.Hour)
		_, err := sc.saveResultProposal(resultProposalArgs{rc.match.Id, rc.submitter, scores, "pending", submitTime})
		return err

	case rc.f.round == 6:
		return nil
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
	rec.Set("proposal_data", pdJSON)
	rec.Set("proposal_status", a.status)
	rec.Set("created", a.ts.Format(time.RFC3339))
	if err := sc.app.Save(rec); err != nil {
		return nil, fmt.Errorf("save result proposal: %w", err)
	}
	return rec, nil
}

type resultResponseArgs struct {
	matchID, authorID, parentID, action, content string
	ts                                           time.Time
}

func (sc *sampleCtx) saveResultResponse(a resultResponseArgs) error {
	rec := core.NewRecord(sc.msgCol)
	rec.Set("match", a.matchID)
	rec.Set("author", a.authorID)
	rec.Set("type", "result_response")
	rec.Set("content", a.content)
	rec.Set("parent", a.parentID)
	rec.Set("proposal_data", fmt.Sprintf(`{"action":"%s"}`, a.action))
	rec.Set("created", a.ts.Format(time.RFC3339))
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

// createSampleNotifications files bell notifications for the enriched round-5
// matches: the disputed one (both pairs) and the awaiting-confirmation one (the
// opponent who confirms).
func createSampleNotifications(txApp core.App, comp *core.Record) error {
	col, err := txApp.FindCollectionByNameOrId("notifications")
	if err != nil {
		return err
	}
	notify := func(userIDs []string, ntype, title, body string) error {
		for _, uid := range userIDs {
			n := core.NewRecord(col)
			n.Set("user", uid)
			n.Set("type", ntype)
			n.Set("title", title)
			n.Set("body", body)
			n.Set("read", false)
			if err := txApp.Save(n); err != nil {
				return fmt.Errorf("create sample notification: %w", err)
			}
		}
		return nil
	}
	if disputed := sampleMatchByStatus(txApp, comp.Id, league.StatusDisputed); disputed != nil {
		players := append(playersOfPair(txApp, disputed.GetString("pair1")),
			playersOfPair(txApp, disputed.GetString("pair2"))...)
		if err := notify(players, "dispute", "Partido disputado", "Hay una disputa abierta en tu partido."); err != nil {
			return err
		}
	}
	if awaiting := sampleMatchByStatus(txApp, comp.Id, league.StatusConfirmed); awaiting != nil {
		if err := notify(playersOfPair(txApp, awaiting.GetString("pair2")),
			"quorum_request", "Resultado por confirmar", "Tu rival ha enviado un resultado. Confírmalo o dispútalo."); err != nil {
			return err
		}
	}
	return nil
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

func generateSampleBracket(txApp core.App, compID string, pairIDs []string) error {
	n := len(pairIDs)
	if n < 2 {
		return nil
	}
	numRounds := int(math.Ceil(math.Log2(float64(n))))
	bracketSize := 1 << numRounds

	slots := make([]string, bracketSize)
	copy(slots, pairIDs)

	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}

	advancers := make([]string, bracketSize/2)
	for i := 0; i < bracketSize/2; i++ {
		p1, p2 := slots[i], slots[bracketSize-1-i]
		if p1 == "" && p2 == "" {
			continue
		}
		if p2 == "" {
			advancers[i] = p1
			continue
		}
		if p1 == "" {
			advancers[i] = p2
			continue
		}
		match := core.NewRecord(matchCol)
		match.Set("competition", compID)
		match.Set("round_number", 1)
		match.Set("matches_to_win", 1)
		match.Set("pair1", p1)
		match.Set("pair2", p2)
		match.Set("status", league.StatusPending)
		if err := txApp.Save(match); err != nil {
			return fmt.Errorf("create playoff match r1: %w", err)
		}
	}

	for r := 2; len(advancers) >= 2; r++ {
		numMatches := len(advancers) / 2
		nextAdvancers := make([]string, numMatches)
		for i := 0; i < numMatches; i++ {
			p1 := advancers[i*2]
			p2 := advancers[i*2+1]
			match := core.NewRecord(matchCol)
			match.Set("competition", compID)
			match.Set("round_number", r)
			match.Set("matches_to_win", 1)
			if p1 != "" {
				match.Set("pair1", p1)
			}
			if p2 != "" {
				match.Set("pair2", p2)
			}
			match.Set("status", league.StatusPending)
			if err := txApp.Save(match); err != nil {
				return fmt.Errorf("create playoff match r%d: %w", r, err)
			}
			nextAdvancers[i] = ""
		}
		advancers = nextAdvancers
	}
	return nil
}

func createSampleVenues(txApp core.App) error {
	existing, _ := txApp.FindRecordsByFilter("venues", "id != ''", "", 1, 0)
	if len(existing) > 0 {
		return nil
	}
	col, err := txApp.FindCollectionByNameOrId("venues")
	if err != nil {
		return err
	}
	venues := []struct{ name, address string }{
		{"Padel 360", "Calle del Deporte 12"},
		{"Wurko", "Avda. de la Constitución 45"},
		{"Tecnisur", "Camino Viejo de Málaga 8"},
	}
	for _, v := range venues {
		rec := core.NewRecord(col)
		rec.Set("name", v.name)
		rec.Set("address", v.address)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create sample venue %s: %w", v.name, err)
		}
	}
	return nil
}

func createSampleInvitations(txApp core.App, compID string) error {
	col, err := txApp.FindCollectionByNameOrId("invitations")
	if err != nil {
		return err
	}
	admins, _ := txApp.FindRecordsByFilter("users", "roles ~ 'admin'", "", 1, 0)
	var creatorID string
	if len(admins) > 0 {
		creatorID = admins[0].Id
	} else {
		users, _ := txApp.FindRecordsByFilter("users", "id != ''", "", 1, 0)
		if len(users) == 0 {
			return nil
		}
		creatorID = users[0].Id
	}

	for _, inv := range []struct {
		email  string
		status string
	}{
		{"invitado@example.com", "pending"},
		{"", "pending"},
	} {
		token := make([]byte, 16)
		if _, err := rand.Read(token); err != nil {
			return fmt.Errorf("generate invitation token: %w", err)
		}
		rec := core.NewRecord(col)
		rec.Set("token", hex.EncodeToString(token))
		rec.Set("email", inv.email)
		rec.Set("competition", compID)
		rec.Set("created_by", creatorID)
		rec.Set("status", inv.status)
		rec.Set("max_uses", 1)
		rec.Set("use_count", 0)
		rec.Set("expires_at", time.Now().UTC().Add(7*24*time.Hour))
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create sample invitation: %w", err)
		}
	}
	return nil
}

func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	prefix := email[:1]
	if at > 1 {
		prefix = email[:2]
	}
	return prefix + "***" + email[at:]
}

func saveSampleSchedule(txApp core.App, comp *core.Record) error {
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	rounds := comp.GetInt("rounds")
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("save competition schedule: %w", err)
	}
	return nil
}
