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

//go:embed all:views
var viewsFS embed.FS

//go:embed all:static/*
var staticFS embed.FS

var registry = template.NewRegistry()

func renderPage(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	if e.Auth != nil {
		if _, ok := data["DisplayName"]; !ok {
			data["DisplayName"] = e.Auth.GetString("display_name")
		}
		if _, ok := data["IsAdmin"]; !ok {
			data["IsAdmin"] = e.Auth.GetString("role") == "admin"
		}
	}
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

		pub := handlers.NewPublicHandler(app, renderPage)
		se.Router.GET("/", pub.Home).BindFunc(requireAuthRedirect)
		se.Router.GET("/categoria/{id}", pub.Categoria).BindFunc(requireAuthRedirect)
		se.Router.GET("/temporada/{id}", pub.Temporada).BindFunc(requireAuthRedirect)

		se.Router.POST("/logout", auth.Logout).Bind(apis.RequireAuth())

		admin := handlers.NewAdminHandler(app, renderPage)
		adminGroup := se.Router.Group("/admin")
		adminGroup.BindFunc(requireAuthRedirect)
		adminGroup.BindFunc(middleware.RequireAppAdmin)
		adminGroup.GET("/categorias", admin.Categorias)
		adminGroup.POST("/categorias", admin.CategoriasCreate)
		adminGroup.POST("/categorias/{id}", admin.CategoriasUpdate)
		adminGroup.GET("/temporadas", admin.Temporadas)
		adminGroup.POST("/temporadas", admin.TemporadasCreate)
		adminGroup.POST("/temporadas/{id}/toggle", admin.TemporadasToggle)

		fixture := handlers.NewFixtureHandler(app, renderPage)
		adminGroup.POST("/temporadas/{id}/generate", fixture.GenerateFixtures)

		adminGroup.GET("/jugadores", admin.Jugadores)
		adminGroup.POST("/jugadores", admin.JugadoresCreate)
		adminGroup.GET("/parejas", admin.Parejas)
		adminGroup.POST("/parejas", admin.ParejasCreate)
		adminGroup.GET("/playoffs", admin.Playoffs)
		adminGroup.POST("/playoffs", admin.PlayoffsCreate)

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
