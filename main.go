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

func renderPartial(e *core.RequestEvent, page string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	if e.Auth != nil {
		if _, ok := data["IsAdmin"]; !ok {
			data["IsAdmin"] = e.Auth.GetString("role") == "admin"
		}
	}
	html, err := registry.LoadFS(viewsFS, "views/"+page).Render(data)
	if err != nil {
		return err
	}
	return e.HTML(http.StatusOK, html)
}

func requireAuthRedirect(e *core.RequestEvent) error {
	if e.Auth == nil {
		if e.Request.Header.Get("HX-Request") == "true" {
			e.Response.Header().Set("HX-Redirect", "/login")
			return e.NoContent(http.StatusNoContent)
		}
		return e.Redirect(http.StatusFound, "/login")
	}
	if e.Auth.GetString("display_name") == "" &&
		e.Request.URL.Path != "/profile/complete" {
		if e.Request.Header.Get("HX-Request") == "true" {
			e.Response.Header().Set("HX-Redirect", "/profile/complete")
			return e.NoContent(http.StatusNoContent)
		}
		return e.Redirect(http.StatusFound, "/profile/complete")
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

		se.Router.GET("/logout", auth.Logout)
		se.Router.POST("/logout", auth.Logout)

		admin := handlers.NewAdminHandler(app, renderPage)
		comp := handlers.NewCompetitionHandler(app, renderPage)
		adminGroup := se.Router.Group("/admin")
		adminGroup.BindFunc(requireAuthRedirect)
		adminGroup.BindFunc(middleware.RequireAppAdmin)
		adminGroup.BindFunc(func(e *core.RequestEvent) error {
			handlers.CheckQuorumTimeout(app)
			return e.Next()
		})
		adminGroup.GET("", comp.Dashboard)
		adminGroup.GET("/competitions", comp.Dashboard)
		adminGroup.POST("/competitions", comp.Create)
		adminGroup.GET("/competitions/{id}", comp.Detail)
		adminGroup.POST("/competitions/{id}", comp.Update)
		adminGroup.POST("/competitions/{id}/toggle", comp.Toggle)
		adminGroup.POST("/competitions/{id}/pairs", comp.AddPair)
		adminGroup.POST("/competitions/{id}/copy-pairs", comp.CopyPairs)
		adminGroup.POST("/competitions/{id}/remove-pair", comp.RemovePair)
		adminGroup.POST("/competitions/{id}/payment", comp.TogglePayment)
		adminGroup.POST("/competitions/{id}/penalty", comp.ApplyPenalty)

		fixture := handlers.NewFixtureHandler(app, renderPage)
		adminGroup.POST("/competitions/{id}/generate", fixture.GenerateFixtures)

		adminGroup.GET("/pairs", admin.Pairs)
		adminGroup.POST("/pairs", admin.PairsCreate)
		adminGroup.POST("/pairs/{id}", admin.PairsUpdate)

		adminGroup.GET("/players", admin.Players)
		adminGroup.POST("/players/pre-create", admin.PlayerPreCreate)
		adminGroup.POST("/players/{id}", admin.PlayerUpdate)

		adminGroup.GET("/invitations", admin.InvitationsList)
		adminGroup.POST("/invitations", admin.InvitationsCreate)
		adminGroup.POST("/invitations/{id}/revoke", admin.InvitationsRevoke)

		adminGroup.GET("/disputes", admin.Disputes)
		adminGroup.POST("/disputes/{id}/resolve", admin.DisputesResolve)

		adminGroup.GET("/venues", admin.Venues)
		adminGroup.POST("/venues", admin.VenuesCreate)
		adminGroup.POST("/venues/{id}", admin.VenuesUpdate)
		adminGroup.POST("/venues/{id}/delete", admin.VenuesDelete)

		player := handlers.NewPlayerHandler(app, renderPage)
		se.Router.GET("/player/{id}", player.Player).BindFunc(requireAuthRedirect)
		se.Router.GET("/h2h", player.H2H).BindFunc(requireAuthRedirect)

		match := handlers.NewMatchHandler(app, renderPage)
		se.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/confirm", match.MatchConfirm).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/dispute", match.MatchDispute).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/edit", match.MatchEdit).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/walkover", match.MatchWalkover).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuthRedirect)

		thread := handlers.NewThreadHandler(app, renderPage, renderPartial)
		se.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(requireAuthRedirect)
		se.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(requireAuthRedirect)

		ical := handlers.NewICalHandler(app)
		se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuthRedirect)
		se.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(requireAuthRedirect)

		notif := handlers.NewNotificationHandler(app, renderPage)
		se.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuthRedirect)
		se.Router.GET("/notifications/list", notif.List).BindFunc(requireAuthRedirect)
		se.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(requireAuthRedirect)
		se.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(requireAuthRedirect)
		se.Router.GET("/profile/complete", auth.ProfileComplete).BindFunc(requireAuthRedirect)
		se.Router.POST("/profile/complete", auth.ProfileCompleteSubmit).BindFunc(requireAuthRedirect)
		se.Router.GET("/profile/notifications", notif.Prefs).BindFunc(requireAuthRedirect)
		se.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(requireAuthRedirect)

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
