// Package seed provides development and test data seeding.
package seed

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

type User struct {
	Email       string
	Password    string
	Collection  string
	Role        string
	DisplayName string
}

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
		if u.Role != "" {
			record.Set("role", u.Role)
		}
		if u.DisplayName != "" {
			record.Set("display_name", u.DisplayName)
		}
		record.SetVerified(true)
		if err := app.Save(record); err != nil {
			slog.Error("seed create failed", "email", u.Email, "collection", u.Collection, "err", err)
		} else {
			slog.Info("seed created", "email", u.Email, "collection", u.Collection)
		}
	}
}
