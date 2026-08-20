package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/template"

	"padelleague/handlers"
	_ "padelleague/migrations"
	"padelleague/middleware"
)

//go:embed views/*.html
var viewsFS embed.FS

//go:embed all:static/*
var staticFS embed.FS

var registry = template.NewRegistry()

func renderPage(e *core.RequestEvent, page string, data map[string]any) error {
	html, err := registry.LoadFS(viewsFS, "views/layout.html", "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}

func requireAuthRedirect(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/login")
	}
	return e.Next()
}

func main() {
	app := pocketbase.New()

	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		e.Record.Set("role", "user")
		return e.Next()
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Func:     middleware.CookieAuth,
			Priority: -1030,
		})

		auth := handlers.NewAuthHandler(app, renderPage)

		staticSubFS, _ := fs.Sub(staticFS, "static")
		se.Router.GET("/static/{path...}", apis.Static(staticSubFS, false))

		se.Router.GET("/login", auth.Login)
		se.Router.POST("/login", auth.LoginSubmit)
		se.Router.GET("/register", auth.Register)
		se.Router.POST("/register", auth.RegisterSubmit)

		se.Router.GET("/", func(e *core.RequestEvent) error {
			displayName := ""
			if e.Auth != nil {
				displayName = e.Auth.GetString("display_name")
			}
			return renderPage(e, "home.html", map[string]any{
				"DisplayName": displayName,
			})
		}).BindFunc(requireAuthRedirect)

		se.Router.POST("/logout", auth.Logout).Bind(apis.RequireAuth())

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
