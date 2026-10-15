package league

import "github.com/pocketbase/pocketbase/core"

// RegisterPairStamps freezes who acted for which pair: the author's pair on
// every new match message, and the submitter's and disputer's pair on a match
// when either changes. Readers then name the same pair after its players
// change.
func RegisterPairStamps(app core.App) {
	app.OnRecordCreate("match_messages").BindFunc(func(e *core.RecordEvent) error {
		stampAuthorPair(e.App, e.Record)
		return e.Next()
	})
	app.OnRecordCreate("matches").BindFunc(func(e *core.RecordEvent) error {
		StampActorPairs(e.App, e.Record)
		return e.Next()
	})
	app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
		StampActorPairs(e.App, e.Record)
		return e.Next()
	})
}

// stampAuthorPair sets the author's current pair on a new message. An
// admin_action is the admin acting as admin, never as a player of the match,
// so it carries no pair even when the admin also plays in it.
func stampAuthorPair(app core.App, msg *core.Record) {
	authorID := msg.GetString("author")
	if authorID == "" || msg.GetString("type") == "admin_action" {
		return
	}
	match, err := app.FindRecordById("matches", msg.GetString("match"))
	if err != nil {
		return
	}
	msg.Set("author_pair", CurrentPairID(app, authorID, match))
}
