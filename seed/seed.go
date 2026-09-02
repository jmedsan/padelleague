// Package seed provides development/test data seeding and reset.
package seed

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// User describes a user to seed into the database.
type User struct {
	Email       string
	Password    string
	Collection  string
	Roles       []string
	DisplayName string
	Gender      string
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
		if err := createSeedUser(app, u); err != nil {
			slog.Error("seed create failed", "email", maskEmail(u.Email), "collection", u.Collection, "err", err)
		} else {
			slog.Info("seed created", "email", maskEmail(u.Email), "collection", u.Collection)
		}
	}
}

func createSeedUser(app core.App, u User) error {
	col, err := app.FindCollectionByNameOrId(u.Collection)
	if err != nil {
		return err
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
	if u.Gender != "" {
		record.Set("gender", u.Gender)
	}
	record.SetVerified(true)
	return app.Save(record)
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
