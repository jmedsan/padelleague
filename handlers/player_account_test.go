package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

func TestPlayerNameUpdate_Success(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /player/{id}/name updates the display name",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Old Name", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/name"
		s.Body = strings.NewReader("display_name=New+Name")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Equal(tb, "New Name", user.GetString("display_name"))
		assert.Equal(tb, "/player/"+userID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestPlayerNameUpdate_EmptyRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /player/{id}/name with an empty name is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"El nombre no puede estar vacío"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Keep Name", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/name"
		s.Body = strings.NewReader("display_name=")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.Equal(tb, "Keep Name", user.GetString("display_name"))
	}
	s.Test(t)
}

func TestPlayerNameUpdate_WrongUserRejected(t *testing.T) {
	t.Parallel()
	var targetID string
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /player/{id}/name as a different user is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No puedes editar el perfil de otro jugador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		target := handlers.MakeUserTB(tb, app, "Target Player", "")
		attacker := handlers.MakeUserTB(tb, app, "Attacker Player", "")
		targetID = target.Id
		s.URL = "/player/" + target.Id + "/name"
		s.Body = strings.NewReader("display_name=Hacked")
		hdrs := handlers.AuthHeaders(tb, attacker)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", targetID)
		require.NoError(tb, err)
		assert.Equal(tb, "Target Player", user.GetString("display_name"))
	}
	s.Test(t)
}

func TestPlayerPasswordUpdate_Success(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /player/{id}/password with the correct current password sets a new one",
		Method:         http.MethodPost,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Password Owner", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/password"
		s.Body = strings.NewReader("current_password=testpass123456&new_password=newpass987654&new_password_confirm=newpass987654")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.True(tb, user.ValidatePassword("newpass987654"))
		assert.False(tb, user.ValidatePassword("testpass123456"))
		assert.Equal(tb, "/player/"+userID, res.Header.Get("HX-Redirect"))
	}
	s.Test(t)
}

func TestPlayerPasswordUpdate_WrongCurrentPasswordRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /player/{id}/password with the wrong current password is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"La contraseña actual no es correcta"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Password Owner 2", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/password"
		s.Body = strings.NewReader("current_password=wrongpassword&new_password=newpass987654&new_password_confirm=newpass987654")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.True(tb, user.ValidatePassword("testpass123456"), "password must not have changed")
	}
	s.Test(t)
}

func TestPlayerPasswordUpdate_MismatchedConfirmationRejected(t *testing.T) {
	t.Parallel()
	var userID string
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /player/{id}/password with mismatched new passwords is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"Las contraseñas nuevas no coinciden"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		user := handlers.MakeUserTB(tb, app, "Password Owner 3", "")
		userID = user.Id
		s.URL = "/player/" + user.Id + "/password"
		s.Body = strings.NewReader("current_password=testpass123456&new_password=newpass987654&new_password_confirm=different123")
		hdrs := handlers.AuthHeaders(tb, user)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", userID)
		require.NoError(tb, err)
		assert.True(tb, user.ValidatePassword("testpass123456"), "password must not have changed")
	}
	s.Test(t)
}

func TestPlayerPasswordUpdate_WrongUserRejected(t *testing.T) {
	t.Parallel()
	var targetID string
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "POST /player/{id}/password as a different user is rejected",
		Method:          http.MethodPost,
		ExpectedStatus:  200,
		ExpectedContent: []string{"No puedes cambiar la contraseña de otro jugador"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		target := handlers.MakeUserTB(tb, app, "Target Player 2", "")
		attacker := handlers.MakeUserTB(tb, app, "Attacker Player 2", "")
		targetID = target.Id
		s.URL = "/player/" + target.Id + "/password"
		s.Body = strings.NewReader("current_password=testpass123456&new_password=hacked12345&new_password_confirm=hacked12345")
		hdrs := handlers.AuthHeaders(tb, attacker)
		hdrs["Content-Type"] = "application/x-www-form-urlencoded"
		s.Headers = hdrs
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		user, err := app.FindRecordById("users", targetID)
		require.NoError(tb, err)
		assert.True(tb, user.ValidatePassword("testpass123456"), "password must not have changed")
	}
	s.Test(t)
}
