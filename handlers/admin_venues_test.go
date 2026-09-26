package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"

	"padelleague/handlers"
)

func TestVenueEditFormReturnsFragment(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/venues/{id}/edit returns edit form fragment",
		Method:          http.MethodGet,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Editar club", "name"},
	}
	var venueID string
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		venue := handlers.MakeVenueTB(tb, app, "Club Test")
		venueID = venue.Id
		s.URL = "/admin/venues/" + venue.Id + "/edit"
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		body := handlers.ReadBody(tb, res)
		assert.Contains(tb, body, "Club Test", "form contains venue name")
		assert.Contains(tb, body, venueID, "form posts to the correct venue")
	}
	s.Test(t)
}

func TestVenueEditFormUnknownID(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /admin/venues/{id}/edit returns 404 for unknown venue",
		Method:          http.MethodGet,
		URL:             "/admin/venues/nonexistent123456/edit",
		ExpectedStatus:  404,
		ExpectedContent: []string{"resource"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		admin := makeAdminUser(tb, app)
		s.Headers = handlers.AuthHeaders(tb, admin)
	}
	s.Test(t)
}
