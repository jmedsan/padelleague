package handlers

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
)

func TestVenueEditFormReturnsFragment(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/venues/{id}/edit returns edit form fragment",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Editar club", "name"},
	}
	var venueID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		venue := makeVenueTB(tb, app, "Club Test")
		venueID = venue.Id
		s.URL = "/admin/venues/" + venue.Id + "/edit"
		s.Headers = authHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := readBody(tb, res)
		assert.Contains(tb, body, "Club Test", "form contains venue name")
		assert.Contains(tb, body, venueID, "form posts to the correct venue")
	}
	s.Test(t)
}

func TestVenueEditFormUnknownID(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  testAppFactory,
		Name:            "GET /admin/venues/{id}/edit returns 404 for unknown venue",
		Method:          http.MethodGet,
		URL:             "/admin/venues/nonexistent123456/edit",
		ExpectedStatus:  404,
		ExpectedContent: []string{"resource"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupAdminRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = authHeaders(tb, admin)
	}
	s.Test(t)
}
