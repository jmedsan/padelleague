package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findLatestInvitation returns the most recently created pending invitation.
func findLatestInvitation(tb testing.TB, app *tests.TestApp) *core.Record {
	tb.Helper()
	invites, err := app.FindRecordsByFilter("invitations",
		"status = 'pending'", "-id", 1, 0, nil)
	require.NoError(tb, err)
	require.Equal(tb, 1, len(invites), "expected exactly one pending invitation")
	return invites[0]
}

// Creating invitation without competition → rejected

func TestInvitationRequiresCompetition(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations without competition is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"La competición es obligatoria"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("email=test@test.com&max_uses=1")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invites, _ := app.FindRecordsByFilter("invitations", "status = 'pending'", "", 0, 0, nil)
		assert.Equal(tb, 0, len(invites), "no invitation should be created without competition")
	}
	s.Test(t)
}

// Link invitation with max_uses=5 → stored as 5

func TestInvitationLinkMaxUses5(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations link with max_uses=5",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("max_uses=5&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		assert.Equal(tb, 5, int(inv.GetFloat("max_uses")))
	}
	s.Test(t)
}

// Link invitation with max_uses=0 → clamped to 1

func TestInvitationLinkMaxUses0Rejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations link with max_uses=0 is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"usos máximos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("max_uses=0&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// Link invitation with max_uses=-3 → rejected

func TestInvitationLinkMaxUsesNegativeRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations link with max_uses=-3 is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"usos máximos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("max_uses=-3&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// Link invitation with max_uses=abc → rejected

func TestInvitationLinkMaxUsesNonNumericRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations link with max_uses=abc is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"usos máximos"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("max_uses=abc&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// Email invitation honors max_uses from form

func TestInvitationEmailHonorsMaxUses(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations email honors max_uses",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=someone@test.com&max_uses=10&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		assert.Equal(tb, 10, int(inv.GetFloat("max_uses")),
			"email invitation should honor max_uses from form")
	}
	s.Test(t)
}

// expiration_days=3 → expires_at ~72h from now

func TestInvitationExpiration3Days(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations expiration_days=3 sets ~72h expiry",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	var beforeCreate time.Time
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		beforeCreate = time.Now()
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=exp3@test.com&expiration_days=3&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		expiresAt := inv.GetDateTime("expires_at").Time()
		earliest := beforeCreate.Add(71 * time.Hour)
		latest := beforeCreate.Add(73 * time.Hour)
		assert.True(tb, expiresAt.After(earliest),
			"expires_at %v should be after %v (71h from test start)", expiresAt, earliest)
		assert.True(tb, expiresAt.Before(latest),
			"expires_at %v should be before %v (73h from test start)", expiresAt, latest)
	}
	s.Test(t)
}

// expiration_days=0 → rejected

func TestInvitationExpiration0Rejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations expiration_days=0 is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"días hasta expirar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=exp0@test.com&expiration_days=0&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// expiration_days=-5 → rejected

func TestInvitationExpirationNegativeRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations expiration_days=-5 is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"días hasta expirar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=expneg@test.com&expiration_days=-5&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// expiration_days=abc → rejected

func TestInvitationExpirationNonNumericRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations expiration_days=abc is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"días hasta expirar"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=expbad@test.com&expiration_days=abc&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// email without @ → rejected

func TestInvitationInvalidEmailRejected(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "POST /admin/invitations with an invalid email is rejected",
		Method:          http.MethodPost,
		URL:             "/admin/invitations",
		ExpectedStatus:  200,
		ExpectedContent: []string{"email no es válido"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=not-an-email&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		invs, err := app.FindRecordsByFilter("invitations", "id != ''", "", 0, 0)
		require.NoError(tb, err)
		assert.Empty(tb, invs, "no invitation should be created on validation failure")
	}
	s.Test(t)
}

// omit expiration_days → default 7 days

func TestInvitationExpirationDefault7Days(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations default expiration is 7 days",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	var beforeCreate time.Time
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		beforeCreate = time.Now()
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=expdef@test.com&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		expiresAt := inv.GetDateTime("expires_at").Time()
		earliest := beforeCreate.Add(167 * time.Hour) // 7*24 - 1
		latest := beforeCreate.Add(169 * time.Hour)   // 7*24 + 1
		assert.True(tb, expiresAt.After(earliest),
			"expires_at %v should be after %v (167h)", expiresAt, earliest)
		assert.True(tb, expiresAt.Before(latest),
			"expires_at %v should be before %v (169h)", expiresAt, latest)
	}
	s.Test(t)
}

func TestAdminInvitationsRevoke(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations/{id}/revoke changes status",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	var invID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupFullAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		inv := makeInvitation(t, app, time.Time{})
		invID = inv.Id
		s.URL = "/admin/invitations/" + inv.Id + "/revoke"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("invitations", invID)
		assert.Error(tb, err, "invitation should be deleted")
	}
	s.Test(t)
}

func TestInvitationEmailSendsOnboardingEmail(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations with email sends invite email",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		enableSMTP(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("email=newplayer@test.com&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		require.Equal(tb, 1, app.TestMailer.TotalSend(), "expected one email sent")
		msg := app.TestMailer.LastMessage()
		assert.Equal(tb, "newplayer@test.com", msg.To[0].Address)
		assert.Contains(tb, msg.Subject, "Invitación")
		assert.Contains(tb, msg.HTML, "/register?token=")
	}
	s.Test(t)
}

func TestInvitationLinkNoEmailNoEmail(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations without email sends no email",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		enableSMTP(tb, app)
		comp := makeCompetitionTB(tb, app, "league", nil)
		s.Body = strings.NewReader("max_uses=5&competition=" + comp.Id)
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Equal(tb, 0, app.TestMailer.TotalSend(), "no email for link-only invitation")
	}
	s.Test(t)
}
