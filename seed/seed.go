// Package seed provides development/test data seeding and reset.
package seed

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"padelleague/league"

	"github.com/pocketbase/pocketbase/core"
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
		return wipeCategories(txApp, opts, &summary)
	})
	return summary, err
}

func wipeCategories(txApp core.App, opts WipeOptions, summary *WipeSummary) error {
	if opts.Matches {
		if err := wipeMatches(txApp, summary); err != nil {
			return err
		}
	}
	if opts.Competitions {
		if err := wipeCollection(txApp, "document_acks", new(int)); err != nil {
			return err
		}
		if err := wipeCollection(txApp, "documents", new(int)); err != nil {
			return err
		}
		if err := wipeCollection(txApp, "competitions", &summary.Competitions); err != nil {
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
		if err := createSampleFixtures(txApp, comp, pairIDs, opts.Matches); err != nil {
			return err
		}
		if err := saveSampleSchedule(txApp, comp); err != nil {
			return err
		}
		if err := createSampleDocuments(txApp, comp); err != nil {
			return err
		}
		if opts.Playoff {
			if err := createSamplePlayoff(txApp, pairIDs); err != nil {
				return err
			}
		}
		return nil
	})
}

func createSamplePlayers(txApp core.App) ([]string, error) {
	col, err := txApp.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 8)
	for i := range 8 {
		rec := core.NewRecord(col)
		rec.Set("email", fmt.Sprintf("sample-p%d@padelleague.com", i+1))
		rec.SetPassword(SamplePlayerPassword)
		rec.Set("roles", []string{"player"})
		rec.Set("display_name", fmt.Sprintf("Jugador %d", i+1))
		rec.SetVerified(true)
		if err := txApp.Save(rec); err != nil {
			return nil, fmt.Errorf("create player %d: %w", i+1, err)
		}
		ids[i] = rec.Id
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
	for _, pid := range pairIDs {
		payment[pid] = true
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

// createSampleFixtures builds the round-robin matches. When played is true,
// rounds 1–4 are finalized and round 5 carries a live dispute (match 0) and a
// score awaiting confirmation (match 1), so sample players get real home tasks;
// the rest stay pending. All states are set at CREATE time so the match
// status-transition hook (which only guards updates) is never violated.
func createSampleFixtures(txApp core.App, comp *core.Record, pairIDs []string, played bool) error {
	rounds := league.RoundRobin(pairIDs, true)
	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	for _, round := range rounds {
		for i, m := range round.Matches {
			f := sampleFixture{round: round.Number, idx: i, home: m.Home, away: m.Away, played: played}
			if err := createSampleMatch(txApp, matchCol, comp.Id, f); err != nil {
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

func createSampleMatch(txApp core.App, matchCol *core.Collection, compID string, f sampleFixture) error {
	match := core.NewRecord(matchCol)
	match.Set("competition", compID)
	match.Set("round_number", f.round)
	match.Set("matches_to_win", 1)
	match.Set("pair1", f.home)
	match.Set("pair2", f.away)
	if err := setSampleMatchState(txApp, match, f); err != nil {
		return err
	}
	if err := txApp.Save(match); err != nil {
		return fmt.Errorf("create match round %d: %w", f.round, err)
	}
	return nil
}

// setSampleMatchState sets a match's status at create time: rounds 1–4
// finalized when played; round 5 match 0 disputed and match 1 awaiting
// confirmation; everything else pending.
func setSampleMatchState(txApp core.App, match *core.Record, f sampleFixture) error {
	switch {
	case f.played && f.round <= 4:
		return finalizeSampleMatch(match)
	case f.played && f.round == 5 && f.idx == 0:
		return disputeSampleMatch(txApp, match)
	case f.played && f.round == 5 && f.idx == 1:
		return submitSampleScore(txApp, match, league.StatusConfirmed, "6-2 6-2")
	default:
		match.Set("status", league.StatusPending)
		return nil
	}
}

func finalizeSampleMatch(match *core.Record) error {
	winner, err := league.DetermineWinner(match, "6-3 6-3")
	if err != nil {
		return fmt.Errorf("determine winner: %w", err)
	}
	match.Set("scores", "6-3 6-3")
	match.Set("winner", winner)
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

func createSampleDocuments(txApp core.App, comp *core.Record) error {
	col, err := txApp.FindCollectionByNameOrId("documents")
	if err != nil {
		return fmt.Errorf("find documents collection: %w", err)
	}
	type docSpec struct {
		title     string
		url       string
		mandatory bool
		isDefault bool
	}
	docs := []docSpec{
		{title: "Reglamento", url: "https://www.fep.es/noticias/reglamento-de-juego", mandatory: true, isDefault: true},
		{title: "Tutorial Padel", url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", mandatory: false, isDefault: false},
	}
	var docIDs []string
	for _, d := range docs {
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
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("create sample playoff: %w", err)
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
