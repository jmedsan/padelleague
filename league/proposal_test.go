package league

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProposalData_ValidJSON(t *testing.T) {
	t.Parallel()
	raw := `{"date":"2027-10-15","time":"19:30","venue_id":"abc123","venue_name":"Padel 360","venue_text":""}`
	pd := ParseProposalData(raw)
	require.NotNil(t, pd)
	assert.Equal(t, "2027-10-15", pd.Date)
	assert.Equal(t, "19:30", pd.Time)
	assert.Equal(t, "abc123", pd.VenueID)
	assert.Equal(t, "Padel 360", pd.VenueName)
}

func TestParseProposalData_Map(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"date":       "2026-11-01",
		"time":       "20:00",
		"venue_id":   "",
		"venue_name": "Mi casa",
		"venue_text": "Mi casa",
	}
	pd := ParseProposalData(raw)
	require.NotNil(t, pd)
	assert.Equal(t, "2026-11-01", pd.Date)
	assert.Equal(t, "Mi casa", pd.VenueName)
}

func TestParseProposalData_Nil(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData(nil)
	assert.Nil(t, pd)
}

func TestParseProposalData_EmptyString(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData("")
	assert.Nil(t, pd)
}

func TestParseProposalData_Malformed(t *testing.T) {
	t.Parallel()
	pd := ParseProposalData("not json")
	assert.Nil(t, pd)
}

func TestHasDateAndPlace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	col, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	for _, tt := range []struct {
		date, club string
		want       bool
	}{{"2026-10-20", "Padel 360", true}, {"2026-10-20", "", false}, {"", "Padel 360", false}, {"", "", false}} {
		m := core.NewRecord(col)
		m.Set("date", tt.date)
		m.Set("club", tt.club)
		assert.Equal(t, tt.want, HasDateAndPlace(m), "%q %q", tt.date, tt.club)
	}
}

func TestPendingSchedulingProposal(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1, p2 := makePair(t, app, "A"), makePair(t, app, "B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	m1 := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	m2 := makeMatch(t, app, comp.Id, p2.Id, p1.Id, "pending")
	col, err := app.FindCollectionByNameOrId("match_messages")
	require.NoError(t, err)
	author := makeUser(t, app, "Ana", "ana@test.local")
	add := func(match *core.Record, typ, status, marker string) {
		msg := core.NewRecord(col)
		msg.Set("match", match.Id)
		msg.Set("author", author.Id)
		msg.Set("type", typ)
		msg.Set("proposal_status", status)
		msg.Set("content", marker)
		require.NoError(t, app.Save(msg))
	}

	assert.Nil(t, PendingSchedulingProposal(app, m1.Id), "no messages")
	add(m1, "scheduling_proposal", "accepted", "accepted")
	add(m1, "chat", "pending", "wrong type")
	add(m2, "scheduling_proposal", "pending", "other match")
	assert.Nil(t, PendingSchedulingProposal(app, m1.Id), "only non-matching messages")

	add(m1, "scheduling_proposal", "pending", "older")
	time.Sleep(10 * time.Millisecond)
	add(m1, "scheduling_proposal", "pending", "newest")
	got := PendingSchedulingProposal(app, m1.Id)
	require.NotNil(t, got)
	assert.Equal(t, "newest", got.GetString("content"))
}

// MatchPlayersExcluding is the one recipient rule of every match-thread action:
// all four players but the actor, and all four when the actor is an admin.
func TestMatchPlayersExcluding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "MPE A")
	p2 := makePair(t, app, "MPE B")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	match := makeMatch(t, app, comp.Id, p1.Id, p2.Id, "pending")
	all := []string{p1.GetString("player1"), p1.GetString("player2"), p2.GetString("player1"), p2.GetString("player2")}

	assert.ElementsMatch(t, all[1:], MatchPlayersExcluding(app, match, all[0]), "the actor's partner stays in")
	assert.ElementsMatch(t, all[:3], MatchPlayersExcluding(app, match, all[3]))
	assert.ElementsMatch(t, all, MatchPlayersExcluding(app, match, "an-admin"), "a non-player actor excludes nobody")
}
