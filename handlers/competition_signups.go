package handlers

import (
	"log/slog"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// SignupHandler handles admin management of competition signups (players
// who signed up via an invitation with a competition hint, awaiting pairing).
type SignupHandler struct {
	app core.App
}

// NewSignupHandler creates a SignupHandler with the given dependencies.
func NewSignupHandler(app core.App) *SignupHandler {
	return &SignupHandler{app: app}
}

// SignupView is a pending signup with the player's display info resolved.
type SignupView struct {
	Record *core.Record
	Name   string
	Phone  string
	Gender string
	Source string
}

// pendingSignups returns a competition's pending signups with player info
// resolved, oldest first.
func pendingSignups(app core.App, compID string) []SignupView {
	rows := findRecordsLogged(app, "pendingSignups: find signups", RecordQuery{
		Collection: "competition_signups",
		Filter:     "competition = {:cid} && status = 'pending'",
		Sort:       "created",
		Params:     map[string]any{"cid": compID},
	})
	views := make([]SignupView, 0, len(rows))
	for _, r := range rows {
		user, err := app.FindRecordById("users", r.GetString("user"))
		if err != nil {
			continue
		}
		views = append(views, SignupView{
			Record: r,
			Name:   user.GetString("display_name"),
			Phone:  user.GetString("phone"),
			Gender: user.GetString("gender"),
			Source: r.GetString("source"),
		})
	}
	return views
}

// unsignedUpPlayers returns registered players who have no pending or paired
// signup for the given competition, for the "Añadir jugador" dropdown.
func unsignedUpPlayers(app core.App, compID string) []*core.Record {
	existing := findRecordsLogged(app, "unsignedUpPlayers: find signups", RecordQuery{
		Collection: "competition_signups",
		Filter:     "competition = {:cid} && (status = 'pending' || status = 'paired')",
		Params:     map[string]any{"cid": compID},
	})
	seen := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		seen[r.GetString("user")] = struct{}{}
	}
	players := findRecordsLogged(app, "unsignedUpPlayers: find players", RecordQuery{
		Collection: "users", Filter: "roles ~ 'player'", Sort: "display_name",
	})
	out := make([]*core.Record, 0, len(players))
	for _, p := range players {
		if _, ok := seen[p.Id]; !ok {
			out = append(out, p)
		}
	}
	return out
}

// AddSignup creates an admin-sourced signup for a registered player.
func (h *SignupHandler) AddSignup(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	userID := e.Request.FormValue("user")
	if userID == "" {
		return alertError(e, "Debes seleccionar un jugador")
	}

	col, err := h.app.FindCollectionByNameOrId("competition_signups")
	if err != nil {
		return alertError(e, "Error interno")
	}

	rec := core.NewRecord(col)
	rec.Set("competition", compID)
	rec.Set("user", userID)
	rec.Set("status", "pending")
	rec.Set("source", "admin")
	if err := h.app.Save(rec); err != nil {
		slog.Error("add signup failed", "competition", compID, "user", userID, "err", err)
		return alertError(e, "Error al añadir el jugador")
	}

	flash(e, "Jugador añadido a inscritos")
	return redirectHX(e, "/admin/competitions/"+compID)
}

// PairSignups forms a pair from two pending signups, creates the pair
// record, adds it to the competition, and marks both signups as paired.
func (h *SignupHandler) PairSignups(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	signupIDs := e.Request.Form["signup"]
	if len(signupIDs) != 2 {
		return alertError(e, "Debes seleccionar exactamente dos jugadores")
	}

	signups := make([]*core.Record, 0, 2)
	for _, sid := range signupIDs {
		s, err := h.app.FindRecordById("competition_signups", sid)
		if err != nil {
			return alertError(e, "Inscripción no encontrada")
		}
		signups = append(signups, s)
	}

	user1, err := h.app.FindRecordById("users", signups[0].GetString("user"))
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}
	user2, err := h.app.FindRecordById("users", signups[1].GetString("user"))
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	if pairErr := league.ValidatePairComposition(comp.GetString("gender_type"), user1.GetString("gender"), user2.GetString("gender")); pairErr != nil {
		return alertError(e, pairErr.Error()) //nolint:goerr113 // user-facing Spanish
	}

	if errMsg := h.createPairFromSignups(comp, user1, user2, signups); errMsg != "" {
		return alertError(e, errMsg)
	}

	flash(e, "Pareja formada")
	return redirectHX(e, "/admin/competitions/"+compID)
}

// createPairFromSignups creates the pair record, attaches it to the
// competition, and marks both signups as paired. Returns a ready-to-display
// Spanish error message (empty on success).
func (h *SignupHandler) createPairFromSignups(comp, user1, user2 *core.Record, signups []*core.Record) string {
	pairCol, err := h.app.FindCollectionByNameOrId("pairs")
	if err != nil {
		return "Error interno"
	}
	pair := core.NewRecord(pairCol)
	pair.Set("name", strings.TrimSpace(user1.GetString("display_name")+" / "+user2.GetString("display_name")))
	pair.Set("player1", user1.Id)
	pair.Set("player2", user2.Id)
	if err := h.app.Save(pair); err != nil {
		slog.Error("create pair from signups failed", "err", err)
		return "Error al crear la pareja"
	}

	comp.Set("pairs", league.AppendUnique(comp.GetStringSlice("pairs"), pair.Id))
	if err := h.app.Save(comp); err != nil {
		slog.Error("attach pair to competition failed", "err", err)
		return "Error al añadir la pareja a la competición"
	}

	for _, s := range signups {
		s.Set("status", "paired")
		if err := h.app.Save(s); err != nil {
			slog.Error("mark signup paired failed", "signup", s.Id, "err", err)
		}
	}
	return ""
}

// RejectSignup marks a signup as declined.
func (h *SignupHandler) RejectSignup(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	signupID := e.Request.PathValue("signupId")

	s, err := h.app.FindRecordById("competition_signups", signupID)
	if err != nil {
		return alertError(e, "Inscripción no encontrada")
	}
	s.Set("status", "declined")
	if err := h.app.Save(s); err != nil {
		slog.Error("reject signup failed", "signup", signupID, "err", err)
		return alertError(e, "Error al rechazar la inscripción")
	}

	flash(e, "Inscripción rechazada")
	return redirectHX(e, "/admin/competitions/"+compID)
}
