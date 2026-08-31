package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}

		typeField := msgs.Fields.GetByName("type").(*core.SelectField)
		typeField.Values = append(typeField.Values, "result_response")

		msgs.Fields.Add(&core.RelationField{
			Name:         "parent",
			CollectionId: msgs.Id,
			MaxSelect:    1,
		})

		if err := app.Save(msgs); err != nil {
			return err
		}

		return backfillSchedulingResponseParents(app)
	}, func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}

		typeField := msgs.Fields.GetByName("type").(*core.SelectField)
		filtered := typeField.Values[:0]
		for _, v := range typeField.Values {
			if v != "result_response" {
				filtered = append(filtered, v)
			}
		}
		typeField.Values = filtered

		msgs.Fields.RemoveByName("parent")

		return app.Save(msgs)
	})
}

func backfillSchedulingResponseParents(app core.App) error {
	responses, err := app.FindRecordsByFilter("match_messages",
		"type = 'scheduling_response' && parent = ''", "created", 0, 0, nil)
	if err != nil || len(responses) == 0 {
		return nil
	}

	for _, resp := range responses {
		matchID := resp.GetString("match")
		created := resp.GetDateTime("created").String()

		proposals, err := app.FindRecordsByFilter("match_messages",
			"match = {:mid} && type = 'scheduling_proposal' && created < {:ts}",
			"-created", 1, 0,
			map[string]any{"mid": matchID, "ts": created})
		if err != nil || len(proposals) == 0 {
			continue
		}

		resp.Set("parent", proposals[0].Id)
		if err := app.Save(resp); err != nil {
			continue
		}
	}
	return nil
}
