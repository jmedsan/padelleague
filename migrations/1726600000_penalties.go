package migrations

import (
	"encoding/json"
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
		competitions, err := app.FindCollectionByNameOrId("competitions")
		if err != nil {
			return err
		}

		penalties := core.NewBaseCollection("penalties")
		penalties.Fields.Add(
			&core.RelationField{Name: "competition", CollectionId: competitions.Id, Required: true, MaxSelect: 1},
			&core.TextField{Name: "pair", Required: true, Max: 50},
			&core.NumberField{Name: "amount", Required: true},
			&core.TextField{Name: "reason", Required: true, Max: 500},
			&core.RelationField{Name: "applied_by", CollectionId: users.Id, MaxSelect: 1},
			&core.BoolField{Name: "voided"},
		)
		if err := app.Save(penalties); err != nil {
			return err
		}

		if err := migratePenaltyPoints(app, penalties); err != nil {
			return err
		}

		return dropPenaltyPointsField(app, competitions)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId("penalties"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}

func migratePenaltyPoints(app core.App, penaltyCol *core.Collection) error {
	comps, err := app.FindRecordsByFilter("competitions", "", "", 0, 0, nil)
	if err != nil {
		return err
	}
	for _, comp := range comps {
		raw := comp.Get("penalty_points")
		if raw == nil {
			continue
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("marshal penalty points for competition %s: %w", comp.Id, err)
		}
		var pm map[string]float64
		if err := json.Unmarshal(b, &pm); err != nil {
			return fmt.Errorf("unmarshal penalty points for competition %s: %w", comp.Id, err)
		}
		for pairID, amount := range pm {
			if amount <= 0 {
				continue
			}
			rec := core.NewRecord(penaltyCol)
			rec.Set("competition", comp.Id)
			rec.Set("pair", pairID)
			rec.Set("amount", amount)
			rec.Set("reason", "Migrado (penalización previa)")
			rec.Set("applied_by", "")
			rec.Set("voided", false)
			if err := app.Save(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func dropPenaltyPointsField(app core.App, competitions *core.Collection) error {
	f := competitions.Fields.GetByName("penalty_points")
	if f == nil {
		return nil
	}
	competitions.Fields.RemoveByName(f.GetName())
	return app.Save(competitions)
}
