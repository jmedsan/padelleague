package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds match_messages.author_pair, the pair the author belonged to when the
// message was written, so timeline labels survive a pair's player swap.
// Existing messages are backfilled from the current membership.
func init() {
	m.Register(func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		pairs, err := app.FindCollectionByNameOrId("pairs")
		if err != nil {
			return err
		}
		msgs.Fields.Add(&core.RelationField{Name: "author_pair", CollectionId: pairs.Id, MaxSelect: 1})
		if err := app.Save(msgs); err != nil {
			return err
		}
		return backfillAuthorPair(app)
	}, func(app core.App) error {
		msgs, err := app.FindCollectionByNameOrId("match_messages")
		if err != nil {
			return err
		}
		msgs.Fields.RemoveByName("author_pair")
		return app.Save(msgs)
	})
}

func backfillAuthorPair(app core.App) error {
	msgs, err := app.FindRecordsByFilter("match_messages", "author != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		match, err := app.FindRecordById("matches", msg.GetString("match"))
		if err != nil {
			continue
		}
		pairID := authorPairID(app, msg.GetString("author"), match)
		if pairID == "" {
			continue
		}
		msg.Set("author_pair", pairID)
		if err := app.SaveNoValidate(msg); err != nil {
			return err
		}
	}
	return nil
}

func authorPairID(app core.App, userID string, match *core.Record) string {
	for _, field := range []string{"pair1", "pair2"} {
		pair, err := app.FindRecordById("pairs", match.GetString(field))
		if err != nil {
			continue
		}
		if pair.GetString("player1") == userID || pair.GetString("player2") == userID {
			return pair.Id
		}
	}
	return ""
}
