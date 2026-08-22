package seed

import (
	"log"

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
			log.Printf("seed: collection %s not found: %v", u.Collection, err)
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
			log.Printf("seed: failed to create %s (%s): %v", u.Email, u.Collection, err)
		} else {
			log.Printf("seed: created %s in %s", u.Email, u.Collection)
		}
	}
}
