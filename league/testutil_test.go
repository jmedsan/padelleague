package league

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"

	_ "padelleague/migrations"
)

var (
	pairSeq atomic.Int64
	userSeq atomic.Int64
)

func newTestApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func makeUser(t *testing.T, app core.App, displayName, email string) *core.Record {
	t.Helper()
	n := userSeq.Add(1)
	col, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	if email == "" {
		email = fmt.Sprintf("leagueuser%d@test.local", n)
	}
	record := core.NewRecord(col)
	record.Set("email", email)
	record.Set("username", fmt.Sprintf("luser%d", n))
	record.Set("display_name", displayName)
	record.Set("roles", []string{"player"})
	record.SetPassword("testpass123456")
	record.SetVerified(true)
	require.NoError(t, app.Save(record))
	return record
}

func makePair(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	n := pairSeq.Add(1)
	u1 := makeUser(t, app, name+" P1", fmt.Sprintf("lpair%dp1@test.local", n))
	u2 := makeUser(t, app, name+" P2", fmt.Sprintf("lpair%dp2@test.local", n))
	col, err := app.FindCollectionByNameOrId("pairs")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", name)
	record.Set("player1", u1.Id)
	record.Set("player2", u2.Id)
	require.NoError(t, app.Save(record))
	return record
}

func makeMatch(t *testing.T, app core.App, compID, p1ID, p2ID, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", compID)
	record.Set("pair1", p1ID)
	record.Set("pair2", p2ID)
	record.Set("status", status)
	record.Set("round_number", 1)
	require.NoError(t, app.Save(record))
	return record
}

func makeCompetition(t *testing.T, app core.App, pairs []*core.Record) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("name", "Test Competition")
	record.Set("type", "league")
	record.Set("active", true)
	pairIDs := make([]string, len(pairs))
	for i, p := range pairs {
		pairIDs[i] = p.Id
	}
	record.Set("pairs", pairIDs)
	require.NoError(t, app.Save(record))
	return record
}

func makeMessage(t *testing.T, app core.App, matchID, authorID, msgType, parentID string, created time.Time) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("match", matchID)
	record.Set("author", authorID)
	record.Set("type", msgType)
	if parentID != "" {
		record.Set("parent", parentID)
	}
	dt, err := types.ParseDateTime(created)
	require.NoError(t, err)
	record.SetRaw("created", dt)
	require.NoError(t, app.Save(record))
	return record
}

func makePenalty(t *testing.T, app core.App, competitionID, pairID string, amount float64, voided bool) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("penalties")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Set("competition", competitionID)
	record.Set("pair", pairID)
	record.Set("amount", amount)
	record.Set("reason", "Prueba")
	record.Set("voided", voided)
	require.NoError(t, app.Save(record))
}
