package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.SelectField{
			Name:      "roles",
			Values:    []string{"admin", "player"},
			MaxSelect: 2,
		})
		if err := app.Save(users); err != nil {
			return fmt.Errorf("add roles field: %w", err)
		}

		recs, err := app.FindRecordsByFilter("users", "id != ''", "", 0, 0)
		if err != nil {
			return fmt.Errorf("find users: %w", err)
		}
		for _, rec := range recs {
			role := rec.GetString("role")
			if role == "" {
				role = "player"
			}
			rec.Set("roles", []string{role})
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("migrate user %s: %w", rec.Id, err)
			}
		}

		users, err = app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		rolesField := users.Fields.GetByName("roles").(*core.SelectField)
		rolesField.Required = true
		users.Fields.RemoveByName("role")
		if err := app.Save(users); err != nil {
			return fmt.Errorf("finalize roles field: %w", err)
		}

		adminRule := `@request.auth.roles ?= 'admin'`
		for _, name := range []string{"pairs", "competitions"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.CreateRule = &adminRule
			c.UpdateRule = &adminRule
			c.DeleteRule = &adminRule
			if err := app.Save(c); err != nil {
				return fmt.Errorf("update %s rules: %w", name, err)
			}
		}

		return nil
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.SelectField{
			Name:      "role",
			Values:    []string{"admin", "player"},
			MaxSelect: 1,
		})
		if err := app.Save(users); err != nil {
			return fmt.Errorf("add role field: %w", err)
		}

		recs, err := app.FindRecordsByFilter("users", "id != ''", "", 0, 0)
		if err != nil {
			return fmt.Errorf("find users: %w", err)
		}
		for _, rec := range recs {
			roles := rec.GetStringSlice("roles")
			role := "player"
			if len(roles) > 0 {
				role = roles[0]
			}
			rec.Set("role", role)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("migrate user %s: %w", rec.Id, err)
			}
		}

		users, err = app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		roleField := users.Fields.GetByName("role").(*core.SelectField)
		roleField.Required = true
		users.Fields.RemoveByName("roles")
		if err := app.Save(users); err != nil {
			return fmt.Errorf("finalize role field: %w", err)
		}

		oldRule := `@request.auth.role = 'admin'`
		for _, name := range []string{"pairs", "competitions"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			c.CreateRule = &oldRule
			c.UpdateRule = &oldRule
			c.DeleteRule = &oldRule
			if err := app.Save(c); err != nil {
				return fmt.Errorf("restore %s rules: %w", name, err)
			}
		}

		return nil
	})
}
