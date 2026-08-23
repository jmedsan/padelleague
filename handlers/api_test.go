package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestLoginPage(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:            "GET /login returns login page",
		Method:          http.MethodGet,
		URL:             "/login",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Contrasena"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestLoginWrongCreds(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:            "POST /login with wrong creds shows error",
		Method:          http.MethodPost,
		URL:             "/login",
		Body:            strings.NewReader("email=wrong@test.com&password=badpassword"),
		Headers:         map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		ExpectedStatus:  200,
		ExpectedContent: []string{"alert-error"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestRegisterPage(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:            "GET /register returns register page",
		Method:          http.MethodGet,
		URL:             "/register",
		ExpectedStatus:  200,
		ExpectedContent: []string{"Registro"},
		BeforeTestFunc:  setupAuthRoutes,
	}
	scenario.Test(t)
}

func TestLoginValidCreds(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:   "POST /login with valid creds redirects",
		Method: http.MethodPost,
		URL:    "/login",
		Body:   strings.NewReader("email=testlogin@test.local&password=testpass123456"),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		ExpectedStatus: 302,
		BeforeTestFunc: func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			setupAuthRoutes(tb, app, e)
			makeUserTB(tb, app, "Login Test", "testlogin@test.local")
		},
	}
	scenario.Test(t)
}
