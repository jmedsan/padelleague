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
		e.Record.Set("role", "player")
		return e.Next()
	})

	app.OnRecordUpdate("matches").BindFunc(func(e *core.RecordEvent) error {
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

	app.OnRecordAfterUpdateSuccess("matches").BindFunc(func(e *core.RecordEvent) error {
		oldStatus := e.Record.Original().GetString("status")
		newStatus := e.Record.GetString("status")
		winnerID := e.Record.GetString("winner")

		if oldStatus == newStatus || newStatus != "final" || winnerID == "" {
			return e.Next()
		}

		p1ID := e.Record.GetString("pair1")
		p2ID := e.Record.GetString("pair2")
		var loserID string
		if winnerID == p1ID {
			loserID = p2ID
		} else {
			loserID = p1ID
		}

		return app.RunInTransaction(func(txApp core.App) error {
			winnerPair, err := txApp.FindRecordById("pairs", winnerID)
			if err != nil {
				return err
			}
			loserPair, err := txApp.FindRecordById("pairs", loserID)
			if err != nil {
				return err
			}

			winnerPlayers := []string{winnerPair.GetString("player1"), winnerPair.GetString("player2")}
			loserPlayers := []string{loserPair.GetString("player1"), loserPair.GetString("player2")}

			eloCol, err := txApp.FindCollectionByNameOrId("elo_history")
			if err != nil {
				return err
			}

			for _, uid := range winnerPlayers {
				if uid == "" {
					continue
				}
				user, err := txApp.FindRecordById("users", uid)
				if err != nil {
					continue
				}
				oldELO := int(user.GetFloat("elo"))
				if oldELO == 0 {
					oldELO = 1500
				}
				// Find average opponent ELO
				oppELO := 0
				oppCount := 0
				for _, oid := range loserPlayers {
					if oid == "" {
						continue
					}
					opp, err := txApp.FindRecordById("users", oid)
					if err != nil {
						continue
					}
					oe := int(opp.GetFloat("elo"))
					if oe == 0 {
						oe = 1500
					}
					oppELO += oe
					oppCount++
				}
				if oppCount == 0 {
					oppELO = 1500
					oppCount = 1
				}
				avgOppELO := oppELO / oppCount

				newELO, _ := handlers.ComputeELO(oldELO, avgOppELO)
				user.Set("elo", newELO)
				if err := txApp.Save(user); err != nil {
					return err
				}

				hist := core.NewRecord(eloCol)
				hist.Set("player", uid)
				hist.Set("old_elo", oldELO)
				hist.Set("new_elo", newELO)
				hist.Set("delta", newELO-oldELO)
				hist.Set("match", e.Record.Id)
				if err := txApp.Save(hist); err != nil {
					return err
				}
			}

			for _, uid := range loserPlayers {
				if uid == "" {
					continue
				}
				user, err := txApp.FindRecordById("users", uid)
				if err != nil {
					continue
				}
				oldELO := int(user.GetFloat("elo"))
				if oldELO == 0 {
					oldELO = 1500
				}
				oppELO := 0
				oppCount := 0
				for _, oid := range winnerPlayers {
					if oid == "" {
						continue
					}
					opp, err := txApp.FindRecordById("users", oid)
					if err != nil {
						continue
					}
					oe := int(opp.GetFloat("elo"))
					if oe == 0 {
						oe = 1500
					}
					oppELO += oe
					oppCount++
				}
				if oppCount == 0 {
					oppELO = 1500
					oppCount = 1
				}
				avgOppELO := oppELO / oppCount

				_, newELO := handlers.ComputeELO(avgOppELO, oldELO)
				user.Set("elo", newELO)
				if err := txApp.Save(user); err != nil {
					return err
				}

				hist := core.NewRecord(eloCol)
				hist.Set("player", uid)
				hist.Set("old_elo", oldELO)
				hist.Set("new_elo", newELO)
				hist.Set("delta", newELO-oldELO)
				hist.Set("match", e.Record.Id)
				if err := txApp.Save(hist); err != nil {
					return err
				}
			}

			return nil
		})
	})

	app.OnRecordAfterUpdateSuccess("matches").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "final" {
			handlers.AutoAdvancePlayoff(app, e.Record)
		}
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

		pwReset := handlers.NewPasswordResetHandler(app, renderPage)
		se.Router.GET("/forgot-password", pwReset.ForgotPassword)
		se.Router.POST("/forgot-password", pwReset.ForgotPasswordSubmit)
		se.Router.GET("/reset-password", pwReset.ResetPassword)
		se.Router.POST("/reset-password", pwReset.ResetPasswordSubmit)

		pub := handlers.NewPublicHandler(app, renderPage)
		se.Router.GET("/", pub.Home).BindFunc(requireAuthRedirect)
		se.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuthRedirect)

		se.Router.POST("/logout", auth.Logout).Bind(apis.RequireAuth())

		admin := handlers.NewAdminHandler(app, renderPage)
		comp := handlers.NewCompetitionHandler(app, renderPage)
		adminGroup := se.Router.Group("/admin")
		adminGroup.BindFunc(requireAuthRedirect)
		adminGroup.BindFunc(middleware.RequireAppAdmin)
		adminGroup.BindFunc(func(e *core.RequestEvent) error {
			handlers.CheckQuorumTimeout(app)
			return e.Next()
		})
		adminGroup.GET("/competitions", comp.List)
		adminGroup.POST("/competitions", comp.Create)
		adminGroup.POST("/competitions/{id}", comp.Update)
		adminGroup.POST("/competitions/{id}/toggle", comp.Toggle)
		adminGroup.GET("/competitions/{id}/pairs", comp.ListPairs)
		adminGroup.POST("/competitions/{id}/pairs", comp.AddPair)
		adminGroup.POST("/competitions/{id}/copy-pairs", comp.CopyPairs)

		fixture := handlers.NewFixtureHandler(app, renderPage)
		adminGroup.POST("/competitions/{id}/generate", fixture.GenerateFixtures)

		adminGroup.GET("/pairs", admin.Pairs)
		adminGroup.POST("/pairs", admin.PairsCreate)
		adminGroup.POST("/pairs/{id}", admin.PairsUpdate)

		adminGroup.GET("/disputes", admin.Disputes)
		adminGroup.POST("/disputes/{id}/resolve", admin.DisputesResolve)

		player := handlers.NewPlayerHandler(app, renderPage)
		se.Router.GET("/player/{id}", player.Player).BindFunc(requireAuthRedirect)
		se.Router.GET("/h2h", player.H2H).BindFunc(requireAuthRedirect)

		match := handlers.NewMatchHandler(app, renderPage)
		se.Router.GET("/my-matches", match.MyMatches).BindFunc(requireAuthRedirect)
		se.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/confirm", match.MatchConfirm).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/dispute", match.MatchDispute).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/edit", match.MatchEdit).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/walkover", match.MatchWalkover).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuthRedirect)

		ical := handlers.NewICalHandler(app)
		se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuthRedirect)
		se.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(requireAuthRedirect)

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
