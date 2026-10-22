package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	entryEarlier = "2026-01-01 10:00:00.000Z"
	entryLater   = "2026-01-01 11:00:00.000Z"
)

// matchEntry is one timeline message on a match, with its created stamp fixed
// so the order of entries is deterministic.
type matchEntry struct {
	typ, content, created string
}

func setMatchResult(t *testing.T, app core.App, match *core.Record, status, reviewType, scores, winnerID string) {
	t.Helper()
	match.Set("status", status)
	match.Set("review_type", reviewType)
	match.Set("scores", scores)
	match.Set("winner", winnerID)
	require.NoError(t, app.SaveNoValidate(match))
}

func addMatchEntries(t *testing.T, app core.App, matchID, authorID string, entries []matchEntry) {
	t.Helper()
	for _, e := range entries {
		msg := newMessage(t, app, matchID, authorID, e.typ)
		msg.Set("content", e.content)
		require.NoError(t, app.SaveNoValidate(msg))
		_, err := app.DB().NewQuery("UPDATE match_messages SET created = {:c} WHERE id = {:id}").
			Bind(dbx.Params{"c": e.created, "id": msg.Id}).Execute()
		require.NoError(t, err)
	}
}

func TestWithdrawalResult_UpAddsFieldsAndDownRemovesThem(t *testing.T) {
	t.Parallel()
	app := newSlotTestApp(t)

	require.NoError(t, addWithdrawalResultFields(app))
	matches := mustCollection(t, app, "matches")
	ratingScores := matches.Fields.GetByName("rating_scores")
	require.NotNil(t, ratingScores)
	assert.Equal(t, core.FieldTypeText, ratingScores.Type())
	adminResult := matches.Fields.GetByName("admin_result")
	require.NotNil(t, adminResult)
	assert.Equal(t, core.FieldTypeBool, adminResult.Type())

	require.NoError(t, removeWithdrawalResultFields(app))
	matches = mustCollection(t, app, "matches")
	assert.Nil(t, matches.Fields.GetByName("rating_scores"))
	assert.Nil(t, matches.Fields.GetByName("admin_result"))
}

func TestBackfillAdminResult_MarksOnlyAdminSetFinals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		status     string
		reviewType string
		entries    []matchEntry
		want       bool
	}{
		{
			name:       "approved walkover",
			status:     "final",
			reviewType: "walkover",
			want:       true,
		},
		{
			name:    "admin override",
			status:  "final",
			entries: []matchEntry{{"admin_action", "Resultado establecido: 6-1 6-2", entryEarlier}},
			want:    true,
		},
		{
			name:    "dispute resolved by admin",
			status:  "final",
			entries: []matchEntry{{"result_event", "resolvió la disputa: 6-1 6-2", entryEarlier}},
			want:    true,
		},
		{
			name:   "admin entry superseded by a player response",
			status: "final",
			entries: []matchEntry{
				{"admin_action", "Resultado corregido: 6-0 6-0 → 6-1 6-2", entryEarlier},
				{"result_response", "Resultado aceptado: 6-1 6-2", entryLater},
			},
			want: false,
		},
		{
			name:    "player-produced result",
			status:  "final",
			entries: []matchEntry{{"result_submission", "6-1 6-2", entryEarlier}},
			want:    false,
		},
		{
			name:    "admin entry on an unfinished match",
			status:  "pending",
			entries: []matchEntry{{"admin_action", "Resultado establecido: 6-1 6-2", entryEarlier}},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newSlotTestApp(t)
			require.NoError(t, addWithdrawalResultFields(app))
			match, p1, _ := newPairSwapMatch(t, app)
			setMatchResult(t, app, match, tc.status, tc.reviewType, "6-1 6-2", p1.Id)
			addMatchEntries(t, app, match.Id, p1.GetString("player1"), tc.entries)

			require.NoError(t, backfillAdminResult(app))
			require.NoError(t, backfillAdminResult(app))

			got, err := app.FindRecordById("matches", match.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.GetBool("admin_result"))
		})
	}
}

func TestReorientLegacyWalkovers_FlipsUnorientedRowsOnce(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		status     string
		reviewType string
		winnerSide string // "pair1" or "pair2"
		scores     string
		want       string
	}{
		{
			name:       "legacy walkover stored from pair1's side",
			status:     "final",
			reviewType: "walkover",
			winnerSide: "pair2",
			scores:     "6-2 6-1",
			want:       "2-6 1-6",
		},
		{
			name:       "walkover already oriented to the winner",
			status:     "final",
			reviewType: "walkover",
			winnerSide: "pair2",
			scores:     "2-6 1-6",
			want:       "2-6 1-6",
		},
		{
			name:       "walkover won by pair1 is left alone",
			status:     "final",
			reviewType: "walkover",
			winnerSide: "pair1",
			scores:     "6-2 6-1",
			want:       "6-2 6-1",
		},
		{
			name:       "walkover placeholder WO is left alone",
			status:     "final",
			reviewType: "walkover",
			winnerSide: "pair2",
			scores:     "WO",
			want:       "WO",
		},
		{
			name:       "played match is left alone",
			status:     "final",
			reviewType: "",
			winnerSide: "pair2",
			scores:     "6-2 6-1",
			want:       "6-2 6-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newSlotTestApp(t)
			match, p1, p2 := newPairSwapMatch(t, app)
			winnerID := p1.Id
			if tc.winnerSide == "pair2" {
				winnerID = p2.Id
			}
			setMatchResult(t, app, match, tc.status, tc.reviewType, tc.scores, winnerID)

			require.NoError(t, reorientLegacyWalkovers(app))
			require.NoError(t, reorientLegacyWalkovers(app))

			got, err := app.FindRecordById("matches", match.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.GetString("scores"))
			assert.Equal(t, winnerID, got.GetString("winner"))
		})
	}
}
