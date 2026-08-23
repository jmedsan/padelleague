package routes

import (
	"io/fs"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"padelleague/handlers"
	"padelleague/middleware"
	"padelleague/render"
)

func Register(app core.App, se *core.ServeEvent, r *render.Renderer, notifier *handlers.Notifier, staticFS fs.FS) {
	se.Router.Bind(&hook.Handler[*core.RequestEvent]{
		Func:     middleware.CookieAuth,
		Priority: -1030,
	})

	auth := handlers.NewAuthHandler(app, r.Page)

	se.Router.GET("/manifest.json", func(e *core.RequestEvent) error {
		data, _ := fs.ReadFile(staticFS, "static/manifest.json")
		return e.Blob(http.StatusOK, "application/manifest+json", data)
	})
	se.Router.GET("/sw.js", func(e *core.RequestEvent) error {
		data, _ := fs.ReadFile(staticFS, "static/sw.js")
		e.Response.Header().Set("Service-Worker-Allowed", "/")
		return e.Blob(http.StatusOK, "application/javascript", data)
	})

	staticSubFS, _ := fs.Sub(staticFS, "static")
	se.Router.GET("/static/{path...}", apis.Static(staticSubFS, false))

	se.Router.GET("/login", auth.Login)
	se.Router.POST("/login", auth.LoginSubmit)
	se.Router.GET("/register", auth.Register)
	se.Router.POST("/register", auth.RegisterSubmit)

	pwReset := handlers.NewPasswordResetHandler(app, r.Page)
	se.Router.GET("/forgot-password", pwReset.ForgotPassword)
	se.Router.POST("/forgot-password", pwReset.ForgotPasswordSubmit)
	se.Router.GET("/reset-password", pwReset.ResetPassword)
	se.Router.POST("/reset-password", pwReset.ResetPasswordSubmit)

	pub := handlers.NewPublicHandler(app, r.Page)
	se.Router.GET("/", pub.Home).BindFunc(requireAuth)
	se.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuth)

	se.Router.POST("/logout", auth.Logout)

	admin := handlers.NewAdminHandler(app, notifier, r.Page)
	comp := handlers.NewCompetitionHandler(app, r.Page)
	adminGroup := se.Router.Group("/admin")
	adminGroup.BindFunc(requireAuth)
	adminGroup.BindFunc(middleware.RequireAppAdmin)
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
	adminGroup.POST("/competitions/{id}/payment-all", comp.TogglePaymentAll)
	adminGroup.POST("/competitions/{id}/penalty", comp.ApplyPenalty)

	fixture := handlers.NewFixtureHandler(app, r.Page)
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

	player := handlers.NewPlayerHandler(app, r.Page, r.ErrorPage)
	se.Router.GET("/player/{id}", player.Player).BindFunc(requireAuth)
	se.Router.GET("/h2h", player.H2H).BindFunc(requireAuth)

	match := handlers.NewMatchHandler(app, notifier, r.Page, r.ErrorPage)
	se.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/confirm", match.MatchConfirm).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/dispute", match.MatchDispute).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/edit", match.MatchEdit).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/walkover", match.MatchWalkover).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/admin-override", match.AdminOverride).BindFunc(requireAuth)

	thread := handlers.NewThreadHandler(app, notifier, r.Page, r.Partial)
	se.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(requireAuth)
	se.Router.GET("/match/{id}/thread-messages", thread.ThreadMessages).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/change-decision", thread.ProposalChangeDecision).BindFunc(requireAuth)

	ical := handlers.NewICalHandler(app)
	se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuth)
	se.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(requireAuth)

	notif := handlers.NewNotificationHandler(app, r.Page)
	se.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuth)
	se.Router.GET("/notifications/list", notif.List).BindFunc(requireAuth)
	se.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(requireAuth)
	se.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(requireAuth)

	push := handlers.NewPushHandler(app, notifier)
	if push.Enabled() {
		se.Router.POST("/push/subscribe", push.Subscribe).BindFunc(requireAuth)
		se.Router.POST("/push/unsubscribe", push.Unsubscribe).BindFunc(requireAuth)
	}
	se.Router.GET("/profile/complete", auth.ProfileComplete).BindFunc(requireAuth)
	se.Router.POST("/profile/complete", auth.ProfileCompleteSubmit).BindFunc(requireAuth)
	se.Router.GET("/profile/notifications", notif.Prefs).BindFunc(requireAuth)
	se.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(requireAuth)
}

func requireAuth(e *core.RequestEvent) error {
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
