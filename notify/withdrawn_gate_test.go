package notify

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/league"
)

// withdrawnMatch returns a pending match between pair1 and pair2 of a fresh league in which pair1
// is withdrawn, with the withdrawn pair's and the opponent's user ids.
func withdrawnMatch(t *testing.T, app core.App) (match *core.Record, withdrawn, opponent []string) {
	t.Helper()
	match = makeMatch(t, app)
	comp, err := app.FindRecordById("competitions", match.GetString("competition"))
	require.NoError(t, err)
	comp.Set("withdrawn_pairs", []string{match.GetString("pair1")})
	require.NoError(t, app.Save(comp))
	return match, league.PlayersForPair(app, match.GetString("pair1")), league.PlayersForPair(app, match.GetString("pair2"))
}

func notificationsFor(t *testing.T, app core.App, userID string) []*core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter("notifications", "user = {:uid}", "", 0, 0, map[string]any{"uid": userID})
	require.NoError(t, err)
	return recs
}

// addMatch creates a pending match between p1 and p2 in compID.
func addMatch(t *testing.T, app core.App, compID, p1, p2 string) *core.Record {
	t.Helper()
	matchCol, err := app.FindCollectionByNameOrId("matches")
	require.NoError(t, err)
	match := core.NewRecord(matchCol)
	match.Set("competition", compID)
	match.Set("pair1", p1)
	match.Set("pair2", p2)
	match.Set("status", "pending")
	match.Set("round_number", 1)
	require.NoError(t, app.Save(match))
	return match
}

// TestNotifyPlayers_WithdrawnPairGetsNoCompetitionNotice: after a withdrawal, no notice of the
// competition reaches the withdrawn pair's players, while the opponent still gets it.
func TestNotifyPlayers_WithdrawnPairGetsNoCompetitionNotice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func(match *core.Record) league.Notification
	}{
		{"chat message", func(m *core.Record) league.Notification {
			return league.NotifNewMessage(m.Id, "Ana", "hola", "Test")
		}},
		{"date proposal", func(m *core.Record) league.Notification {
			return league.NotifProposal(league.ProposalParams{MatchID: m.Id, AuthorName: "Ana",
				Date: "15/03", Time: "18:00", VenueName: "Padel 360", CompName: "Test"})
		}},
		{"scheduling reminder", func(m *core.Record) league.Notification {
			return league.NotifSchedulingReminder(league.SchedulingReminderParams{MatchID: m.Id, Opponent: "Pareja B",
				CompName: "Test", Deadline: time.Now().Add(48 * time.Hour), Level: league.WarnHeadsUp})
		}},
		{"result proposal", func(m *core.Record) league.Notification {
			return league.NotifResultSubmitted(m.Id, "Pareja B", "Test", "6-2 6-2")
		}},
		{"quorum pending", func(m *core.Record) league.Notification {
			return league.NotifResultConfirmationPending(m.Id, "Pareja B", "Test", 24)
		}},
		{"quorum auto-confirmed", func(m *core.Record) league.Notification {
			return league.NotifResultAutoConfirmed(m.Id, "Test")
		}},
		{"calendar published", func(m *core.Record) league.Notification {
			return league.NotifCalendarPublished(m.GetString("competition"), "Test")
		}},
	}
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match, withdrawn, opponent := withdrawnMatch(t, app)
			notifier.NotifyPlayers(append(append([]string{}, withdrawn...), opponent...), tc.build(match))
			for _, uid := range withdrawn {
				assert.Empty(t, notificationsFor(t, app, uid), "withdrawn player %s must get no notice", uid)
			}
			for _, uid := range opponent {
				assert.Len(t, notificationsFor(t, app, uid), 1, "opponent %s must still get the notice", uid)
			}
		})
	}
}

// TestNotifyPlayers_WithdrawnPairGetsNoEmail: the email channel obeys the same gate as the in-app one.
func TestNotifyPlayers_WithdrawnPairGetsNoEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	enableSMTP(t, app)
	match, withdrawn, opponent := withdrawnMatch(t, app)

	NewNotifier(app, "", "").NotifyPlayers(append(append([]string{}, withdrawn...), opponent...),
		league.NotifNewMessage(match.Id, "Ana", "hola", "Test"))

	require.Equal(t, len(opponent), app.TestMailer.TotalSend(), "only the opponent's players get an email")
	sent := make(map[string]bool)
	for _, m := range app.TestMailer.Messages() {
		sent[m.To[0].Address] = true
	}
	for _, uid := range withdrawn {
		u, err := app.FindRecordById("users", uid)
		require.NoError(t, err)
		assert.False(t, sent[u.Email()], "withdrawn player %s must get no email", uid)
	}
	for _, uid := range opponent {
		u, err := app.FindRecordById("users", uid)
		require.NoError(t, err)
		assert.True(t, sent[u.Email()], "opponent %s must get the email", uid)
	}
}

// TestNotifyPlayers_WithdrawnPairStillGetsOtherCompetitions: the gate is per competition.
func TestNotifyPlayers_WithdrawnPairStillGetsOtherCompetitions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	notifier := NewNotifier(app, "", "")
	first, withdrawn, _ := withdrawnMatch(t, app)
	withdrawnPair, opponentPair := first.GetString("pair1"), first.GetString("pair2")

	compCol, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	other := core.NewRecord(compCol)
	other.Set("name", "Other")
	other.Set("type", "league")
	other.Set("active", true)
	other.Set("pairs", []string{withdrawnPair, opponentPair})
	require.NoError(t, app.Save(other))
	match := addMatch(t, app, other.Id, withdrawnPair, opponentPair)

	notifier.NotifyPlayers(withdrawn, league.NotifNewMessage(match.Id, "Ana", "hola", "Other"))
	notifier.NotifyPlayers(withdrawn, league.NotifCalendarPublished(other.Id, "Other"))
	for _, uid := range withdrawn {
		assert.Len(t, notificationsFor(t, app, uid), 2, "player %s must get notices of the other competition", uid)
	}
}

// TestNotifyPlayers_WithdrawalNoticeReachesWithdrawnPair: the one notice that announces the
// withdrawal is not gated, because it is sent after the withdrawal is saved.
func TestNotifyPlayers_WithdrawalNoticeReachesWithdrawnPair(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, withdrawn, _ := withdrawnMatch(t, app)

	NewNotifier(app, "", "").NotifyPlayers(withdrawn, league.NotifPairWithdrawn("Test"))
	for _, uid := range withdrawn {
		recs := notificationsFor(t, app, uid)
		require.Len(t, recs, 1, "player %s must get the withdrawal notice", uid)
		assert.Equal(t, "Retirada de la competición", recs[0].GetString("title"))
	}
}

// TestNotifyAdmins_WithdrawnPairMatchStillReachesAdmins: admins are never gated.
func TestNotifyAdmins_WithdrawnPairMatchStillReachesAdmins(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	admin := makeUser(t, app, "admin")
	match, _, _ := withdrawnMatch(t, app)

	require.NoError(t, NewNotifier(app, "", "").NotifyAdmins(league.NotifNewMessage(match.Id, "Ana", "hola", "Test")))
	assert.Len(t, notificationsFor(t, app, admin.Id), 1)
}

// TestNotifyPlayers_WithdrawnSkipIsLogged: each gated recipient leaves one log line with the reason.
func TestNotifyPlayers_WithdrawnSkipIsLogged(t *testing.T) {
	lc := withLogCapture(t)
	app := newTestApp(t)
	match, withdrawn, _ := withdrawnMatch(t, app)

	NewNotifier(app, "", "").NotifyPlayers(withdrawn, league.NotifNewMessage(match.Id, "Ana", "hola", "Test"))
	for _, uid := range withdrawn {
		attrs, _ := lc.logged(t, "notification skipped", uid)
		assert.Equal(t, "pair withdrawn from competition", attrs["reason"])
	}
}

// TestNotifyPlayers_UnresolvableMatchReachesNobody: a notice whose match cannot be found is not
// sent to anyone, because the withdrawn gate cannot tell who to skip (fail closed).
func TestNotifyPlayers_UnresolvableMatchReachesNobody(t *testing.T) {
	app := newTestApp(t)
	_, withdrawn, opponent := withdrawnMatch(t, app)
	everyone := append(append([]string{}, withdrawn...), opponent...)

	NewNotifier(app, "", "").NotifyPlayers(everyone, league.NotifNewMessage("missing-match-id", "Ana", "hola", "Test"))
	for _, uid := range everyone {
		assert.Empty(t, notificationsFor(t, app, uid), "user %s", uid)
	}
}

// TestNotifyPlayers_UnresolvableCompetitionReachesNobody: a notice naming a competition that cannot
// be found is not sent to anyone (fail closed), the same as an unresolvable match.
func TestNotifyPlayers_UnresolvableCompetitionReachesNobody(t *testing.T) {
	app := newTestApp(t)
	_, withdrawn, opponent := withdrawnMatch(t, app)
	everyone := append(append([]string{}, withdrawn...), opponent...)

	notif := league.Notification{Type: "general", Title: "Aviso", Body: "Aviso", CompID: "missing-competition-id"}
	NewNotifier(app, "", "").NotifyPlayers(everyone, notif)
	for _, uid := range everyone {
		assert.Empty(t, notificationsFor(t, app, uid), "user %s", uid)
	}
}

// TestNotifyPlayers_CompetitionIDDecidesTheGate: a notice's own competition decides who is withdrawn,
// not the competition of the match it names.
func TestNotifyPlayers_CompetitionIDDecidesTheGate(t *testing.T) {
	app := newTestApp(t)
	match, withdrawn, _ := withdrawnMatch(t, app)
	compCol, err := app.FindCollectionByNameOrId("competitions")
	require.NoError(t, err)
	other := core.NewRecord(compCol)
	other.Set("name", "Other")
	other.Set("type", "league")
	other.Set("active", true)
	other.Set("pairs", []string{match.GetString("pair1"), match.GetString("pair2")})
	require.NoError(t, app.Save(other))

	notif := league.Notification{Type: "general", Title: "Aviso", Body: "Aviso", MatchID: match.Id, CompID: other.Id}
	NewNotifier(app, "", "").NotifyPlayers(withdrawn, notif)
	for _, uid := range withdrawn {
		assert.Len(t, notificationsFor(t, app, uid), 1, "player %s is not withdrawn from %s", uid, other.Id)
	}
}
