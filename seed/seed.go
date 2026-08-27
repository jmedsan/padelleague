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

// Wipe deletes all in-scope data inside a transaction, preserving admin users,
// venues, and superusers.
func Wipe(app core.App) (WipeSummary, error) {
	var summary WipeSummary

	type target struct {
		name    string
		counter *int
	}
	targets := []target{
		{"match_messages", &summary.Messages},
		{"notifications", &summary.Notifications},
		{"matches", &summary.Matches},
		{"competitions", &summary.Competitions},
		{"invitations", &summary.Invitations},
		{"push_subscriptions", &summary.Subscriptions},
		{"pairs", &summary.Pairs},
	}

	err := app.RunInTransaction(func(txApp core.App) error {
		for _, t := range targets {
			if err := wipeCollection(txApp, t.name, t.counter); err != nil {
				return err
			}
		}
		return wipeNonAdminUsers(txApp, &summary.Players)
	})

	return summary, err
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

// SampleLeague creates a sample league with 8 players, 4 pairs, 1 competition,
// and 12 matches (rounds 1–4 finalized, rounds 5–6 pending).
func SampleLeague(app core.App) error {
	return app.RunInTransaction(func(txApp core.App) error {
		playerIDs, err := createSamplePlayers(txApp)
		if err != nil {
			return err
		}
		pairIDs, err := createSamplePairs(txApp, playerIDs)
		if err != nil {
			return err
		}
		comp, err := createSampleCompetition(txApp, pairIDs)
		if err != nil {
			return err
		}
		if err := createSampleFixtures(txApp, comp, pairIDs); err != nil {
			return err
		}
		return saveSampleSchedule(txApp, comp)
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
		rec.SetPassword("padel1234")
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
	comp := core.NewRecord(col)
	comp.Set("name", "Liga de ejemplo")
	comp.Set("type", "league")
	comp.Set("active", true)
	comp.Set("play_twice", true)
	comp.Set("pairs", pairIDs)
	comp.Set("start_date", now)
	comp.Set("end_date", now.Add(30*24*time.Hour))
	if err := txApp.Save(comp); err != nil {
		return nil, fmt.Errorf("create competition: %w", err)
	}
	return comp, nil
}

func createSampleFixtures(txApp core.App, comp *core.Record, pairIDs []string) error {
	rounds := league.RoundRobin(pairIDs, true)
	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}
	for _, round := range rounds {
		for _, m := range round.Matches {
			match := core.NewRecord(matchCol)
			match.Set("competition", comp.Id)
			match.Set("round_number", round.Number)
			match.Set("matches_to_win", 1)
			match.Set("pair1", m.Home)
			match.Set("pair2", m.Away)
			if round.Number <= 4 {
				winner, err := league.DetermineWinner(match, "6-3 6-3")
				if err != nil {
					return fmt.Errorf("determine winner round %d: %w", round.Number, err)
				}
				match.Set("scores", "6-3 6-3")
				match.Set("winner", winner)
				match.Set("status", league.StatusFinal)
			} else {
				match.Set("status", league.StatusPending)
			}
			if err := txApp.Save(match); err != nil {
				return fmt.Errorf("create match round %d: %w", round.Number, err)
			}
		}
	}
	comp.Set("rounds", len(rounds))
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
