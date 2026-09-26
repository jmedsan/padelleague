package handlers

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/internal/testapp"
	"padelleague/league"
	"padelleague/notify"
)

// thread_internal_test.go holds thread-related unit tests that exercise
// unexported ThreadHandler internals (resolveVenue, notifyProposal,
// buildThreadData) and unexported package-level helpers (ParseProposalData,
// ProposalActions, PlayerTeamOf) directly, with no HTTP round trip. These
// stayed in package handlers instead of moving to package handlers_test
// because none of them touch routes.Register — routing_test.go's
// setupProductionRoutes is for tests that need real route wiring.

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

func TestProposalActions(t *testing.T) {
	t.Parallel()

	const prop = "scheduling_proposal"

	cases := []struct {
		name                    string
		msgType, matchStatus    string
		sameTeamOrOutsider      bool
		proposalStatus          string
		wantRespond, wantChange bool
	}{
		{"opponent, pending", prop, league.StatusPending, false, "pending", true, false},
		{"opponent, accepted", prop, league.StatusPending, false, "accepted", false, true},
		{"opponent, rejected", prop, league.StatusPending, false, "rejected", false, true},
		{"own proposal, pending", prop, league.StatusPending, true, "pending", false, false},
		{"own proposal, accepted", prop, league.StatusPending, true, "accepted", false, false},
		{"outsider", prop, league.StatusPending, true, "pending", false, false},
		{"confirmed match", prop, league.StatusConfirmed, false, "pending", false, false},
		{"disputed match", prop, league.StatusDisputed, false, "pending", false, false},
		{"final match", prop, league.StatusFinal, false, "pending", false, false},
		{"chat message", "chat", league.StatusPending, false, "pending", false, false},
		{"score discussion", "score_discussion", league.StatusPending, false, "pending", false, false},
		{"superseded proposal", prop, league.StatusPending, false, "superseded", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			respond, change := proposalActions(tc.msgType, tc.matchStatus, tc.sameTeamOrOutsider, tc.proposalStatus)
			assert.Equal(t, tc.wantRespond, respond, "canRespond")
			assert.Equal(t, tc.wantChange, change, "canChange")
		})
	}
}

// playerTeamOf decides which side of a match a user is on. Returning the
// wrong side, or zero for a real participant, silently removes every action
// that participation grants.

func TestPlayerTeamOf(t *testing.T) {
	t.Parallel()

	p1 := []string{"alice", "bob"}
	p2 := []string{"carol", "dave"}

	cases := []struct {
		name string
		uid  string
		want int
	}{
		{"first player of pair 1", "alice", 1},
		{"second player of pair 1", "bob", 1},
		{"first player of pair 2", "carol", 2},
		{"second player of pair 2", "dave", 2},
		{"not in either pair", "eve", 0},
		{"empty user id", "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, playerTeamOf(tc.uid, p1, p2))
		})
	}
}

// A player listed in both pairs should resolve to pair 1, since that is the
// first match found. Pinning it stops the order silently reversing.

func TestPlayerTeamOfPrefersPairOne(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, playerTeamOf("alice", []string{"alice"}, []string{"alice"}))
}

// resolveVenue turns a form's venue selection into a stored venue id and a
// display name. The "otro" option means the user typed a free-text venue, so
// no id is stored and their text is kept verbatim.

func TestResolveVenue(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	h := NewThreadHandler(ThreadDeps{App: app})

	col, err := app.FindCollectionByNameOrId("venues")
	require.NoError(t, err)
	venue := core.NewRecord(col)
	venue.Set("name", "Padel 360")
	require.NoError(t, app.Save(venue))

	t.Run("known venue keeps its id and uses its name", func(t *testing.T) {
		id, name := h.resolveVenue(venue.Id, "ignored text")
		assert.Equal(t, venue.Id, id)
		assert.Equal(t, "Padel 360", name)
	})

	t.Run("otro keeps the typed text and stores no id", func(t *testing.T) {
		id, name := h.resolveVenue("otro", "Club del barrio")
		assert.Empty(t, id)
		assert.Equal(t, "Club del barrio", name)
	})

	t.Run("empty selection keeps the typed text", func(t *testing.T) {
		id, name := h.resolveVenue("", "Otro sitio")
		assert.Empty(t, id)
		assert.Equal(t, "Otro sitio", name)
	})

	t.Run("unknown venue id falls back to the typed text", func(t *testing.T) {
		id, name := h.resolveVenue("nonexistent", "Respaldo")
		assert.Empty(t, id)
		assert.Equal(t, "Respaldo", name)
	})
}

// A scheduling proposal must notify the opposing pair, never the proposer's
// own partner. Getting the side wrong means the person who needs to respond
// is never told, and the proposal sits unanswered.

func TestProposalNotifiesOpposingPair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "Notif P1")
	p2 := makePairTB(t, app, "Notif P2")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	notifier := notify.NewNotifier(app, "", "")
	h := NewThreadHandler(ThreadDeps{App: app, Notifier: notifier})

	// A member of pair 1 proposes; pair 2 must hear about it.
	author := p1.GetString("player1")
	h.notifyProposal(match, 1, proposalNotice{AuthorID: author, Date: "2027-09-20", Time: "19:00", VenueName: "Club Test"})

	notifs, err := app.FindRecordsByFilter("notifications",
		"type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	require.NotEmpty(t, notifs, "a scheduling notification should exist")

	got := map[string]bool{}
	for _, n := range notifs {
		got[n.GetString("user")] = true
	}

	for _, uid := range league.PlayersForPair(app, p2.Id) {
		assert.True(t, got[uid], "opposing pair member %s should be notified", uid)
	}
	for _, uid := range league.PlayersForPair(app, p1.Id) {
		assert.False(t, got[uid], "proposer's own pair member %s must not be notified", uid)
	}
}

// The mirror case: a member of pair 2 proposes, so pair 1 is notified.

func TestProposalFromPairTwoNotifiesPairOne(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "Mirror P1")
	p2 := makePairTB(t, app, "Mirror P2")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	h := NewThreadHandler(ThreadDeps{App: app, Notifier: notify.NewNotifier(app, "", "")})
	h.notifyProposal(match, 2, proposalNotice{AuthorID: p2.GetString("player1"), Date: "2027-09-20", Time: "19:00", VenueName: "Club Test"})

	notifs, err := app.FindRecordsByFilter("notifications", "type = 'scheduling'", "", 0, 0, nil)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, n := range notifs {
		got[n.GetString("user")] = true
	}
	for _, uid := range league.PlayersForPair(app, p1.Id) {
		assert.True(t, got[uid], "pair 1 member %s should be notified", uid)
	}
	for _, uid := range league.PlayersForPair(app, p2.Id) {
		assert.False(t, got[uid], "proposer's own pair member %s must not be notified", uid)
	}
}

func TestProposalSendsEmail(t *testing.T) {
	t.Parallel()
	testApp := testapp.New(t)

	testApp.Settings().SMTP.Enabled = true
	testApp.Settings().SMTP.Host = "smtp.test.local"
	testApp.Settings().SMTP.Port = 587
	require.NoError(t, testApp.Save(testApp.Settings()))

	p1 := makePairTB(t, testApp, "Email P1")
	p2 := makePairTB(t, testApp, "Email P2")
	comp := makeCompetitionTB(t, testApp, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, testApp, comp.Id, p1.Id, p2.Id, "pending")

	h := NewThreadHandler(ThreadDeps{App: testApp, Notifier: notify.NewNotifier(testApp, "", "")})
	h.notifyProposal(match, 1, proposalNotice{
		AuthorID: p1.GetString("player1"), Date: "2027-09-20", Time: "19:00", VenueName: "Club Test",
	})

	assert.Greater(t, testApp.TestMailer.TotalSend(), 0, "scheduling proposal must send email")
	found := false
	for _, msg := range testApp.TestMailer.Messages() {
		if msg.Subject == "Propuesta de fecha" {
			found = true
			break
		}
	}
	assert.True(t, found, "email subject should be 'Propuesta de fecha'")
}

func TestBuildThreadData_TimelineReadOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "TL-RO A")
	p2 := makePairTB(t, app, "TL-RO B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	proposer := p1.GetString("player1")
	responder := p2.GetString("player1")

	prop := makeProposal(t, app, match.Id, proposer)
	makeProposalWithStatus(t, app, match.Id, proposer, "rejected")
	accepted := makeProposalWithStatus(t, app, match.Id, proposer, "accepted")
	makeSchedulingResponse(t, app, match.Id, responder, accepted.Id,
		responder+" aceptó la propuesta de "+proposer)

	h := NewThreadHandler(ThreadDeps{App: app})
	td := h.buildThreadData(match, match.Id, threadViewerCtx{myTeam: 2, compModifiable: true})

	require.NotEmpty(t, td.Timeline, "timeline must have entries")

	var hasAcceptResponse bool
	for _, entry := range td.Timeline {
		if entry.Kind == "response" {
			hasAcceptResponse = true
			assert.Equal(t, "confirmó fecha y lugar", entry.Content)
		}
		assert.NotEmpty(t, entry.CreatedAt, "entry must have CreatedAt")
	}
	assert.True(t, hasAcceptResponse, "acceptance response must appear as top-level timeline entry (P7)")

	var proposalCount int
	for _, entry := range td.Timeline {
		if entry.Kind == "proposal" {
			proposalCount++
		}
	}
	assert.GreaterOrEqual(t, proposalCount, 1, "proposals must appear in timeline")

	// Viewer is team 2; proposer is team 1 → first entry's IsMyTeam must be false
	assert.False(t, td.Timeline[0].IsMyTeam, "team-1 author must not be IsMyTeam for team-2 viewer")

	require.NotEmpty(t, td.SchedProposals)
	found := false
	for _, sp := range td.SchedProposals {
		if sp.RecordID == prop.Id {
			found = true
			assert.Equal(t, "pending", sp.Status)
			assert.True(t, sp.CanRespond, "opposing team can respond to pending proposal")
		}
		if sp.RecordID == accepted.Id {
			assert.Equal(t, "accepted", sp.Status, "accepted proposal status")
		}
	}
	assert.True(t, found, "pending proposal must be in SchedProposals")
}

func TestBuildThreadData_HidesRejectedSched(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "HR A")
	p2 := makePairTB(t, app, "HR B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "pending")

	proposer := p1.GetString("player1")

	pending := makeProposal(t, app, match.Id, proposer)
	rejected := makeProposalWithStatus(t, app, match.Id, proposer, "rejected")

	h := NewThreadHandler(ThreadDeps{App: app})
	td := h.buildThreadData(match, match.Id, threadViewerCtx{myTeam: 2, compModifiable: true})

	var ids []string
	for _, sp := range td.SchedProposals {
		ids = append(ids, sp.RecordID)
	}
	assert.Contains(t, ids, pending.Id, "pending proposal must appear")
	assert.NotContains(t, ids, rejected.Id, "rejected proposal must be hidden (P2)")
}

func TestBuildThreadData_NoFinalUntilQuorum(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "NF A")
	p2 := makePairTB(t, app, "NF B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")

	proposer := p1.GetString("player1")
	makeResultProposal(t, app, match.Id, proposer, "6-3 6-4")

	h := NewThreadHandler(ThreadDeps{App: app})

	td := h.buildThreadData(match, match.Id, threadViewerCtx{myTeam: 2, compModifiable: true})
	assert.False(t, td.ResultPanel.HasFinal, "must not show final before quorum (P4)")
	require.Len(t, td.ResultPanel.Live, 1, "one live result proposal")
	assert.Equal(t, "6-3 6-4", td.ResultPanel.Live[0].Score)

	winner, _ := league.DetermineWinner(match, "6-3 6-4")
	match.Set("status", league.StatusFinal)
	match.Set("scores", "6-3 6-4")
	match.Set("winner", winner)
	require.NoError(t, app.Save(match))

	td2 := h.buildThreadData(match, match.Id, threadViewerCtx{myTeam: 2, compModifiable: true})
	assert.True(t, td2.ResultPanel.HasFinal, "must show final after quorum (P4)")
	assert.Equal(t, "6-3 6-4", td2.ResultPanel.FinalScore)
	assert.NotEmpty(t, td2.ResultPanel.WinnerName)
	assert.Empty(t, td2.ResultPanel.Live, "no live proposals when final")
}

func TestBuildThreadData_BothLiveProposals(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	p1 := makePairTB(t, app, "BL A")
	p2 := makePairTB(t, app, "BL B")
	comp := makeCompetitionTB(t, app, "league", []*core.Record{p1, p2})
	match := makeMatchTB(t, app, comp.Id, p1.Id, p2.Id, "scheduled")

	p1Player := p1.GetString("player1")
	p2Player := p2.GetString("player1")

	makeResultProposal(t, app, match.Id, p1Player, "6-3 6-4")
	makeResultProposal(t, app, match.Id, p2Player, "3-6 6-4 6-3")

	h := NewThreadHandler(ThreadDeps{App: app})

	td := h.buildThreadData(match, match.Id, threadViewerCtx{myTeam: 2, compModifiable: true})
	require.Len(t, td.ResultPanel.Live, 2, "deadlock: both proposals live (P5)")

	for _, lp := range td.ResultPanel.Live {
		assert.NotEmpty(t, lp.PairLabel, "each live proposal must have PairLabel")
	}

	awaitingCount := 0
	for _, lp := range td.ResultPanel.Live {
		if lp.AwaitingMe {
			awaitingCount++
			assert.True(t, lp.CanRespond, "awaiting proposal must be respondable")
		}
	}
	assert.Equal(t, 1, awaitingCount, "exactly one proposal awaiting viewer (P5)")
	// The proposal from the opposing team (p1) is the one awaiting the viewer (team 2)
	assert.True(t, td.ResultPanel.Live[0].AwaitingMe, "proposal from p1 (team 1) awaits team-2 viewer")
	assert.False(t, td.ResultPanel.Live[1].AwaitingMe, "proposal from p2 (team 2) does not await same-team viewer")
}
