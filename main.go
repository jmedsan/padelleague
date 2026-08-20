package main

import (
	"embed"
	"fmt"
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
		if _, ok := data["Verified"]; !ok {
			data["Verified"] = e.Auth.Verified()
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

	app.OnRecordUpdate("partidos").BindFunc(func(e *core.RecordEvent) error {
		oldStatus := e.Record.Original().GetString("status")
		newStatus := e.Record.GetString("status")
		if oldStatus != newStatus {
			validTransitions := map[string][]string{
				"pending":   {"confirmed", "final"},
				"confirmed": {"final", "disputed"},
				"disputed":  {"final"},
			}
			allowed, ok := validTransitions[oldStatus]
			if !ok {
				return fmt.Errorf("invalid transition from %s", oldStatus)
			}
			valid := false
			for _, s := range allowed {
				if s == newStatus {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid transition: %s → %s", oldStatus, newStatus)
			}
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("partidos").BindFunc(func(e *core.RecordEvent) error {
		oldStatus := e.Record.Original().GetString("status")
		newStatus := e.Record.GetString("status")
		winnerID := e.Record.GetString("winner")

		if oldStatus == newStatus || newStatus != "final" || winnerID == "" {
			return e.Next()
		}

		p1ID := e.Record.GetString("pareja1")
		p2ID := e.Record.GetString("pareja2")
		var loserID string
		if winnerID == p1ID {
			loserID = p2ID
		} else {
			loserID = p1ID
		}

		return app.RunInTransaction(func(txApp core.App) error {
			winnerPair, err := txApp.FindRecordById("parejas", winnerID)
			if err != nil {
				return err
			}
			loserPair, err := txApp.FindRecordById("parejas", loserID)
			if err != nil {
				return err
			}

			oldWinnerELO := int(winnerPair.GetFloat("elo"))
			oldLoserELO := int(loserPair.GetFloat("elo"))
			if oldWinnerELO == 0 {
				oldWinnerELO = 1500
			}
			if oldLoserELO == 0 {
				oldLoserELO = 1500
			}

			newWinnerELO, newLoserELO := handlers.ComputeELO(oldWinnerELO, oldLoserELO)

			winnerPair.Set("elo", newWinnerELO)
			if err := txApp.Save(winnerPair); err != nil {
				return err
			}
			loserPair.Set("elo", newLoserELO)
			if err := txApp.Save(loserPair); err != nil {
				return err
			}

			eloCol, err := txApp.FindCollectionByNameOrId("elo_history")
			if err != nil {
				return err
			}

			winnerHistory := core.NewRecord(eloCol)
			winnerHistory.Set("pareja", winnerID)
			winnerHistory.Set("old_elo", oldWinnerELO)
			winnerHistory.Set("new_elo", newWinnerELO)
			winnerHistory.Set("delta", newWinnerELO-oldWinnerELO)
			winnerHistory.Set("partido", e.Record.Id)
			if err := txApp.Save(winnerHistory); err != nil {
				return err
			}

			loserHistory := core.NewRecord(eloCol)
			loserHistory.Set("pareja", loserID)
			loserHistory.Set("old_elo", oldLoserELO)
			loserHistory.Set("new_elo", newLoserELO)
			loserHistory.Set("delta", newLoserELO-oldLoserELO)
			loserHistory.Set("partido", e.Record.Id)
			if err := txApp.Save(loserHistory); err != nil {
				return err
			}

			return nil
		})
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

		pwReset := handlers.NewPasswordResetHandler(app, renderPage)
		se.Router.GET("/forgot-password", pwReset.ForgotPassword)
		se.Router.POST("/forgot-password", pwReset.ForgotPasswordSubmit)
		se.Router.GET("/reset-password", pwReset.ResetPassword)
		se.Router.POST("/reset-password", pwReset.ResetPasswordSubmit)


		pub := handlers.NewPublicHandler(app, renderPage)
		se.Router.GET("/", pub.Home).BindFunc(requireAuthRedirect)
		se.Router.GET("/categoria/{id}", pub.Categoria).BindFunc(requireAuthRedirect)
		se.Router.GET("/temporada/{id}", pub.Temporada).BindFunc(requireAuthRedirect)

		se.Router.POST("/logout", auth.Logout).Bind(apis.RequireAuth())

		admin := handlers.NewAdminHandler(app, renderPage)
		adminGroup := se.Router.Group("/admin")
		adminGroup.BindFunc(requireAuthRedirect)
		adminGroup.BindFunc(middleware.RequireAppAdmin)
		adminGroup.BindFunc(func(e *core.RequestEvent) error {
			handlers.CheckQuorumTimeout(app)
			return e.Next()
		})
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
		adminGroup.GET("/disputas", admin.Disputas)
		adminGroup.POST("/disputas/{id}/resolve", admin.DisputasResolve)

		player := handlers.NewPlayerHandler(app, renderPage)
		se.Router.GET("/jugador/{id}", player.Player).BindFunc(requireAuthRedirect)
		se.Router.GET("/h2h", player.H2H).BindFunc(requireAuthRedirect)

		match := handlers.NewMatchHandler(app, renderPage)
		se.Router.GET("/mis-partidos", match.MisPartidos).BindFunc(requireAuthRedirect)
		se.Router.GET("/partido/{id}", match.Partido).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/submit", match.PartidoSubmit).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/confirm", match.PartidoConfirm).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/dispute", match.PartidoDispute).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/edit", match.PartidoEdit).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/walkover", match.PartidoWalkover).BindFunc(requireAuthRedirect)
		se.Router.POST("/partido/{id}/correct", match.PartidoCorrect).BindFunc(requireAuthRedirect)

		ical := handlers.NewICalHandler(app)
		se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuthRedirect)
		se.Router.GET("/ical/season/{id}", ical.Season).BindFunc(requireAuthRedirect)

		notif := handlers.NewNotificationHandler(app, renderPage)
		se.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuthRedirect)
		se.Router.GET("/notifications/list", notif.List).BindFunc(requireAuthRedirect)
		se.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(requireAuthRedirect)
		se.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(requireAuthRedirect)
		se.Router.GET("/profile/notifications", notif.Prefs).BindFunc(requireAuthRedirect)
		se.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(requireAuthRedirect)

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
