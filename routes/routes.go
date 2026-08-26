// Package routes registers all HTTP routes with their handlers.
package routes

import (
	"io/fs"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"padelleague/handlers"
	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

// Deps holds the shared dependencies injected into all route groups.
type Deps struct {
	App       core.App
	Renderer  *render.Renderer
	Notifier  *notify.Notifier
	LeagueSvc *league.Service
	StaticFS  fs.FS
}

// Register wires all application routes onto the given serve event.
func Register(se *core.ServeEvent, deps Deps) {
	se.Router.Bind(&hook.Handler[*core.RequestEvent]{
		Func:     middleware.CookieAuth,
		Priority: -1030,
	})

	registerStaticRoutes(se, deps)
	registerAuthRoutes(se, deps)
	registerPublicRoutes(se, deps)
	registerAdminRoutes(se, deps)
	registerMatchRoutes(se, deps)
	registerNotificationRoutes(se, deps)
	registerProfileRoutes(se, deps)
}

func registerStaticRoutes(se *core.ServeEvent, deps Deps) {
	se.Router.GET("/manifest.json", func(e *core.RequestEvent) error {
		data, _ := fs.ReadFile(deps.StaticFS, "static/manifest.json")
		return e.Blob(http.StatusOK, "application/manifest+json", data)
	})
	se.Router.GET("/sw.js", func(e *core.RequestEvent) error {
		data, _ := fs.ReadFile(deps.StaticFS, "static/sw.js")
		e.Response.Header().Set("Service-Worker-Allowed", "/")
		return e.Blob(http.StatusOK, "application/javascript", data)
	})

	staticSubFS, _ := fs.Sub(deps.StaticFS, "static")
	se.Router.GET("/static/{path...}", apis.Static(staticSubFS, false))
}

func registerAuthRoutes(se *core.ServeEvent, deps Deps) {
	auth := handlers.NewAuthHandler(deps.App, deps.Renderer.Page)

	se.Router.GET("/login", auth.Login)
	se.Router.POST("/login", auth.LoginSubmit)
	se.Router.GET("/register", auth.Register)
	se.Router.POST("/register", auth.RegisterSubmit)
	se.Router.POST("/logout", auth.Logout)

	pwReset := handlers.NewPasswordResetHandler(deps.App, deps.Renderer.Page)
	se.Router.GET("/forgot-password", pwReset.ForgotPassword)
	se.Router.POST("/forgot-password", pwReset.ForgotPasswordSubmit)
	se.Router.GET("/reset-password", pwReset.ResetPassword)
	se.Router.POST("/reset-password", pwReset.ResetPasswordSubmit)
}

func registerPublicRoutes(se *core.ServeEvent, deps Deps) {
	pub := handlers.NewPublicHandler(deps.App, deps.LeagueSvc, deps.Renderer.Page, deps.Renderer.ErrorPage)
	se.Router.GET("/", pub.Home).BindFunc(requireAuth)
	se.Router.GET("/competition/{id}", pub.Competition).BindFunc(requireAuth)

	player := handlers.NewPlayerHandler(deps.App, deps.Renderer.Page, deps.Renderer.ErrorPage)
	se.Router.GET("/player/{id}", player.Player).BindFunc(requireAuth)
	se.Router.GET("/h2h", player.H2H).BindFunc(requireAuth)

	ical := handlers.NewICalHandler(deps.App)
	se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(requireAuth)
	se.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(requireAuth)
}

func registerAdminRoutes(se *core.ServeEvent, deps Deps) {
	admin := handlers.NewAdminHandler(deps.App, deps.Notifier, deps.Renderer.Page)
	comp := handlers.NewCompetitionHandler(deps.App, deps.LeagueSvc, deps.Renderer.Page)
	fixture := handlers.NewFixtureHandler(deps.App, deps.LeagueSvc, deps.Renderer.Page)

	g := se.Router.Group("/admin")
	g.BindFunc(requireAuth)
	g.BindFunc(middleware.RequireAppAdmin)

	g.GET("", comp.Dashboard)
	g.GET("/competitions", comp.Dashboard)
	g.POST("/competitions", comp.Create)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/pairs", comp.AddPair)
	g.POST("/competitions/{id}/copy-pairs", comp.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", comp.RemovePair)
	g.POST("/competitions/{id}/payment", comp.TogglePayment)
	g.POST("/competitions/{id}/payment-all", comp.TogglePaymentAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)

	g.GET("/pairs", admin.Pairs)
	g.POST("/pairs", admin.PairsCreate)
	g.POST("/pairs/{id}", admin.PairsUpdate)

	g.GET("/players", admin.Players)
	g.POST("/players/pre-create", admin.PlayerPreCreate)
	g.POST("/players/{id}", admin.PlayerUpdate)

	g.GET("/invitations", admin.InvitationsList)
	g.POST("/invitations", admin.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", admin.InvitationsRevoke)

	g.GET("/disputes", admin.Disputes)
	g.POST("/disputes/{id}/resolve", admin.DisputesResolve)
	g.POST("/disputes/{id}/walkover-approve", admin.WalkoverApprove)

	g.GET("/venues", admin.Venues)
	g.POST("/venues", admin.VenuesCreate)
	g.POST("/venues/{id}", admin.VenuesUpdate)
	g.POST("/venues/{id}/delete", admin.VenuesDelete)
}

func registerMatchRoutes(se *core.ServeEvent, deps Deps) {
	match := handlers.NewMatchHandler(deps.App, deps.Notifier, deps.Renderer.Page, deps.Renderer.ErrorPage)
	se.Router.GET("/match/{id}", match.MatchDetail).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/confirm", match.MatchConfirm).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/dispute", match.MatchDispute).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/walkover", match.MatchWalkover).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/admin-override", match.AdminOverride).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/report-unplayed", match.ReportUnplayed).BindFunc(requireAuth)

	thread := handlers.NewThreadHandler(deps.App, deps.Notifier, deps.Renderer.Page, deps.Renderer.Partial)
	se.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(requireAuth)
	se.Router.GET("/match/{id}/thread-messages", thread.ThreadMessages).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/change-decision", thread.ProposalChangeDecision).BindFunc(requireAuth)
	se.Router.POST("/match/{id}/thread/availability", thread.PostAvailability).BindFunc(requireAuth)
}

func registerNotificationRoutes(se *core.ServeEvent, deps Deps) {
	notif := handlers.NewNotificationHandler(deps.App, deps.Renderer.Page)
	se.Router.GET("/notifications/count", notif.Count).BindFunc(requireAuth)
	se.Router.GET("/notifications/list", notif.List).BindFunc(requireAuth)
	se.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(requireAuth)
	se.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(requireAuth)

	push := handlers.NewPushHandler(deps.App, deps.Notifier)
	if push.Enabled() {
		se.Router.POST("/push/subscribe", push.Subscribe).BindFunc(requireAuth)
		se.Router.POST("/push/unsubscribe", push.Unsubscribe).BindFunc(requireAuth)
	}
}

func registerProfileRoutes(se *core.ServeEvent, deps Deps) {
	auth := handlers.NewAuthHandler(deps.App, deps.Renderer.Page)
	notif := handlers.NewNotificationHandler(deps.App, deps.Renderer.Page)

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
