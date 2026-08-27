// Package routes registers all HTTP routes with their handlers.
package routes

import (
	"io/fs"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"

	"padelleague/handlers"
	"padelleague/league"
	"padelleague/middleware"
	"padelleague/notify"
	"padelleague/render"
)

// Deps holds the shared dependencies injected into all route groups.
type Deps struct {
	App         core.App
	Renderer    *render.Renderer
	Notifier    *notify.Notifier
	LeagueSvc   *league.Service
	StaticFS    fs.FS
	AppDevTools bool
}

// Register wires all application routes onto the given serve event.
func Register(se *core.ServeEvent, deps Deps) {
	se.Router.Bind(&hook.Handler[*core.RequestEvent]{
		Func:     middleware.CookieAuth,
		Priority: -1030,
	})

	auth := handlers.NewAuthHandler(deps.App, deps.Renderer.Page)
	notif := handlers.NewNotificationHandler(deps.App, deps.Renderer.Page)

	registerStaticRoutes(se, deps)
	registerAuthRoutes(se, deps, auth)
	registerPublicRoutes(se, deps)
	registerAdminRoutes(se, deps)
	registerMatchRoutes(se, deps)
	registerNotificationRoutes(se, deps, notif)
	registerProfileRoutes(se, auth, notif)
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

func registerAuthRoutes(se *core.ServeEvent, deps Deps, auth *handlers.AuthHandler) {
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
	se.Router.GET("/", pub.Home).BindFunc(middleware.RequireAuth)
	se.Router.GET("/competition/{id}", pub.Competition).BindFunc(middleware.RequireAuth)
	se.Router.POST("/competition/{id}/accept-docs", pub.AcceptDocs).BindFunc(middleware.RequireAuth)

	player := handlers.NewPlayerHandler(deps.App, deps.Renderer.Page, deps.Renderer.ErrorPage)
	se.Router.GET("/player/{id}", player.Player).BindFunc(middleware.RequireAuth)
	se.Router.GET("/h2h", player.H2H).BindFunc(middleware.RequireAuth)

	ical := handlers.NewICalHandler(deps.App)
	se.Router.GET("/ical/match/{id}", ical.Match).BindFunc(middleware.RequireAuth)
	se.Router.GET("/ical/competition/{id}", ical.Competition).BindFunc(middleware.RequireAuth)

	view := handlers.NewViewHandler()
	se.Router.GET("/view/{mode}", view.Switch).BindFunc(middleware.RequireAuth)
}

func registerAdminRoutes(se *core.ServeEvent, deps Deps) {
	g := se.Router.Group("/admin")
	g.BindFunc(middleware.RequireAuth)
	g.BindFunc(middleware.RequireAppAdmin)

	registerAdminCompetitionRoutes(g, deps)
	registerAdminDisputeRoutes(g, deps)
	registerAdminInvitationRoutes(g, deps)
	registerAdminPairRoutes(g, deps)
	registerAdminPlayerRoutes(g, deps)
	registerAdminVenueRoutes(g, deps)
	registerAdminDocumentRoutes(g, deps)
	registerAdminSettingsRoutes(g, deps)
}

func registerAdminDocumentRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	doc := handlers.NewDocumentHandler(deps.App, deps.Renderer.Page)
	g.GET("/documents", doc.Documents)
	g.POST("/documents", doc.DocumentsCreate)
	g.POST("/documents/{id}", doc.DocumentsUpdate)
	g.POST("/documents/{id}/delete", doc.DocumentsDelete)
}

func registerAdminCompetitionRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	comp := handlers.NewCompetitionHandler(deps.App, deps.LeagueSvc, deps.Renderer.Page)
	fixture := handlers.NewFixtureHandler(deps.App, deps.LeagueSvc, deps.Renderer.Page)

	g.GET("", comp.Dashboard)
	g.GET("/competitions", comp.Dashboard)
	g.POST("/competitions", comp.Create)
	g.GET("/competitions/{id}", comp.Detail)
	g.POST("/competitions/{id}", comp.Update)
	g.POST("/competitions/{id}/attach-doc", comp.AttachDocument)
	g.POST("/competitions/{id}/detach-doc/{docId}", comp.DetachDocument)
	g.POST("/competitions/{id}/toggle", comp.Toggle)
	g.POST("/competitions/{id}/finalize", comp.FinalizeCompetition)
	g.POST("/competitions/{id}/pairs", comp.AddPair)
	g.POST("/competitions/{id}/copy-pairs", comp.CopyPairs)
	g.POST("/competitions/{id}/remove-pair", comp.RemovePair)
	g.POST("/competitions/{id}/payment", comp.TogglePayment)
	g.POST("/competitions/{id}/payment-all", comp.TogglePaymentAll)
	g.POST("/competitions/{id}/penalty", comp.ApplyPenalty)
	g.POST("/competitions/{id}/generate", fixture.GenerateFixtures)
	g.POST("/competitions/{id}/round-dates", comp.UpdateRoundDates)
	g.POST("/competitions/{id}/round-dates/regenerate", comp.RegenerateRoundDates)
}

func registerAdminDisputeRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	h := handlers.NewDisputeHandler(deps.App, deps.Notifier, deps.Renderer.Page)
	g.GET("/disputes", h.Disputes)
	g.POST("/disputes/{id}/resolve", h.DisputesResolve)
	g.POST("/disputes/{id}/walkover-approve", h.WalkoverApprove)
}

func registerAdminInvitationRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	h := handlers.NewInvitationHandler(deps.App, deps.Renderer.Page)
	g.GET("/invitations", h.InvitationsList)
	g.POST("/invitations", h.InvitationsCreate)
	g.POST("/invitations/{id}/revoke", h.InvitationsRevoke)
	g.GET("/outstanding", h.Outstanding)
}

func registerAdminPairRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	h := handlers.NewPairHandler(deps.App, deps.Renderer.Page)
	g.GET("/pairs", h.Pairs)
	g.POST("/pairs", h.PairsCreate)
	g.POST("/pairs/{id}", h.PairsUpdate)
}

func registerAdminPlayerRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	h := handlers.NewAdminPlayerHandler(deps.App, deps.Renderer.Page)
	g.GET("/players", h.Players)
	g.POST("/players/pre-create", h.PlayerPreCreate)
	g.POST("/players/{id}", h.PlayerUpdate)
}

func registerAdminVenueRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	h := handlers.NewVenueHandler(deps.App, deps.Renderer.Page)
	g.GET("/venues", h.Venues)
	g.POST("/venues", h.VenuesCreate)
	g.POST("/venues/{id}", h.VenuesUpdate)
	g.POST("/venues/{id}/delete", h.VenuesDelete)
}

func registerAdminSettingsRoutes(g *router.RouterGroup[*core.RequestEvent], deps Deps) {
	settings := handlers.NewAdminSettingsHandler(deps.App, deps.AppDevTools, deps.Renderer.Page)
	g.GET("/settings", settings.Settings)
	g.POST("/settings/reset", settings.Reset)
}

func registerMatchRoutes(se *core.ServeEvent, deps Deps) {
	match := handlers.NewMatchHandler(deps.App, deps.Notifier, deps.Renderer.Page, deps.Renderer.ErrorPage)
	se.Router.GET("/match/{id}", match.MatchDetail).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/submit", match.MatchSubmit).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/confirm", match.MatchConfirm).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/dispute", match.MatchDispute).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/correct", match.MatchCorrect).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/admin-override", match.AdminOverride).BindFunc(middleware.RequireAuth).BindFunc(middleware.RequireAppAdmin)
	se.Router.POST("/match/{id}/report-unplayed", match.ReportUnplayed).BindFunc(middleware.RequireAuth)

	thread := handlers.NewThreadHandler(deps.App, deps.Notifier, deps.Renderer.Page, deps.Renderer.Partial)
	se.Router.GET("/match/{id}/thread", thread.Thread).BindFunc(middleware.RequireAuth)
	se.Router.GET("/match/{id}/thread-messages", thread.ThreadMessages).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/thread/message", thread.PostMessage).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/thread/proposal", thread.PostProposal).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/respond", thread.RespondProposal).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/thread/proposal/{msgId}/change-decision", thread.ProposalChangeDecision).BindFunc(middleware.RequireAuth)
	se.Router.POST("/match/{id}/thread/availability", thread.PostAvailability).BindFunc(middleware.RequireAuth)
}

func registerNotificationRoutes(se *core.ServeEvent, deps Deps, notif *handlers.NotificationHandler) {
	se.Router.GET("/notifications/count", notif.Count).BindFunc(middleware.RequireAuth)
	se.Router.GET("/notifications/list", notif.List).BindFunc(middleware.RequireAuth)
	se.Router.POST("/notifications/{id}/read", notif.MarkRead).BindFunc(middleware.RequireAuth)
	se.Router.POST("/notifications/read-all", notif.MarkAllRead).BindFunc(middleware.RequireAuth)

	push := handlers.NewPushHandler(deps.App, deps.Notifier)
	if push.Enabled() {
		se.Router.POST("/push/subscribe", push.Subscribe).BindFunc(middleware.RequireAuth)
		se.Router.POST("/push/unsubscribe", push.Unsubscribe).BindFunc(middleware.RequireAuth)
	}
}

func registerProfileRoutes(se *core.ServeEvent, auth *handlers.AuthHandler, notif *handlers.NotificationHandler) {
	se.Router.GET("/profile/complete", auth.ProfileComplete).BindFunc(middleware.RequireAuth)
	se.Router.POST("/profile/complete", auth.ProfileCompleteSubmit).BindFunc(middleware.RequireAuth)
	se.Router.GET("/profile/notifications", notif.Prefs).BindFunc(middleware.RequireAuth)
	se.Router.POST("/profile/notifications", notif.PrefsSave).BindFunc(middleware.RequireAuth)
}
