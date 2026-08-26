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
		s.Body = strings.NewReader("max_uses=5")
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

func TestInvitationLinkMaxUses0Clamped(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations link with max_uses=0 clamps to 1",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("max_uses=0")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		assert.Equal(tb, 1, int(inv.GetFloat("max_uses")))
	}
	s.Test(t)
}

// Link invitation with max_uses=-3 → clamped to 1

func TestInvitationLinkMaxUsesNegativeClamped(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations link with max_uses=-3 clamps to 1",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("max_uses=-3")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		assert.Equal(tb, 1, int(inv.GetFloat("max_uses")))
	}
	s.Test(t)
}

// Email invitation ignores max_uses (stays 1)

func TestInvitationEmailIgnoresMaxUses(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations email ignores max_uses",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Body = strings.NewReader("email=someone@test.com&max_uses=10")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		assert.Equal(tb, 1, int(inv.GetFloat("max_uses")),
			"email invitation must have max_uses=1 regardless of form value")
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
		s.Body = strings.NewReader("email=exp3@test.com&expiration_days=3")
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

// expiration_days=0 → clamped to 1 day

func TestInvitationExpiration0Clamped(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations expiration_days=0 clamps to 1 day",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	var beforeCreate time.Time
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		beforeCreate = time.Now()
		s.Body = strings.NewReader("email=exp0@test.com&expiration_days=0")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		expiresAt := inv.GetDateTime("expires_at").Time()
		earliest := beforeCreate.Add(23 * time.Hour)
		latest := beforeCreate.Add(25 * time.Hour)
		assert.True(tb, expiresAt.After(earliest),
			"expires_at %v should be after %v (23h)", expiresAt, earliest)
		assert.True(tb, expiresAt.Before(latest),
			"expires_at %v should be before %v (25h)", expiresAt, latest)
	}
	s.Test(t)
}

// expiration_days=-5 → clamped to 1 day

func TestInvitationExpirationNegativeClamped(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory: testAppFactory,
		Name:           "POST /admin/invitations expiration_days=-5 clamps to 1 day",
		Method:         http.MethodPost,
		URL:            "/admin/invitations",
		ExpectedStatus: 204,
	}
	var beforeCreate time.Time
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		beforeCreate = time.Now()
		s.Body = strings.NewReader("email=expneg@test.com&expiration_days=-5")
		hdrs := authHeaders(tb, admin)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		inv := findLatestInvitation(tb, app)
		expiresAt := inv.GetDateTime("expires_at").Time()
		earliest := beforeCreate.Add(23 * time.Hour)
		latest := beforeCreate.Add(25 * time.Hour)
		assert.True(tb, expiresAt.After(earliest),
			"expires_at %v should be after %v (23h)", expiresAt, earliest)
		assert.True(tb, expiresAt.Before(latest),
			"expires_at %v should be before %v (25h)", expiresAt, latest)
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
		s.Body = strings.NewReader("email=expdef@test.com")
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
