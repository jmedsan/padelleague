package handlers

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// AdminBroadcast saves an announcement for a competition and notifies its players.
func (h *CompetitionHandler) AdminBroadcast(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	title := strings.TrimSpace(e.Request.FormValue("title"))
	body := strings.TrimSpace(e.Request.FormValue("body"))
	if title == "" || body == "" {
		return alertError(e, "El título y el mensaje son obligatorios")
	}

	col, err := h.app.FindCollectionByNameOrId("announcements")
	if err != nil {
		return alertError(e, "Error interno")
	}
	ann := core.NewRecord(col)
	ann.Set("competition", comp.Id)
	ann.Set("title", title)
	ann.Set("body", body)
	ann.Set("created_by", e.Auth.Id)
	if err := h.app.Save(ann); err != nil {
		slog.Error("save announcement failed", "competition", comp.Id, "err", err)
		return alertError(e, "Error al guardar el aviso")
	}

	seen := make(map[string]struct{})
	var players []string
	for _, pid := range comp.GetStringSlice("pairs") {
		for _, uid := range league.PlayersForPair(h.app, pid) {
			if _, ok := seen[uid]; !ok {
				seen[uid] = struct{}{}
				players = append(players, uid)
			}
		}
	}

	h.notifier.NotifyPlayers(players, league.Notification{
		Type:     "announcement",
		Title:    title,
		Body:     body,
		CompName: comp.GetString("name"),
		Link:     "/competition/" + comp.Id + "#anuncios",
	})

	slog.Info("broadcast sent", "competition", comp.Id, "players", len(players))
	flash(e, "Aviso enviado a "+strconv.Itoa(len(players))+" jugadores")
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}

// AdminDeleteAnnouncement removes an announcement from a competition.
func (h *CompetitionHandler) AdminDeleteAnnouncement(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}
	ann, err := h.app.FindRecordById("announcements", e.Request.PathValue("annId"))
	if err != nil || ann.GetString("competition") != comp.Id {
		return alertError(e, "Aviso no encontrado")
	}
	if err := h.app.Delete(ann); err != nil {
		return alertError(e, "Error al eliminar el aviso")
	}
	flash(e, "Aviso eliminado")
	return redirectHX(e, "/admin/competitions/"+comp.Id)
}
