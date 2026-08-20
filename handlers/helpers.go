package handlers

import (
	"fmt"
	"log"

	"github.com/pocketbase/pocketbase/core"
)

func findJugadorByUser(app core.App, userID string) (*core.Record, error) {
	records, err := app.FindRecordsByFilter("jugadores", "user = {:uid}", "", 1, 0,
		map[string]any{"uid": userID})
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("jugador not found for user %s", userID)
	}
	return records[0], nil
}

func getJugadorTeam(app core.App, jugadorID string, partido *core.Record) (int, error) {
	pareja1, err := app.FindRecordById("parejas", partido.GetString("pareja1"))
	if err != nil {
		return 0, fmt.Errorf("pareja1 not found: %w", err)
	}
	if pareja1.GetString("jugador1") == jugadorID || pareja1.GetString("jugador2") == jugadorID {
		return 1, nil
	}
	pareja2, err := app.FindRecordById("parejas", partido.GetString("pareja2"))
	if err != nil {
		return 0, fmt.Errorf("pareja2 not found: %w", err)
	}
	if pareja2.GetString("jugador1") == jugadorID || pareja2.GetString("jugador2") == jugadorID {
		return 2, nil
	}
	return 0, fmt.Errorf("jugador %s is not a participant", jugadorID)
}

func notifyAdmins(app core.App, notifType, title, body, relatedPartidoID string) error {
	notifCol, err := app.FindCollectionByNameOrId("notificaciones")
	if err != nil {
		return fmt.Errorf("notificaciones collection not found: %w", err)
	}
	admins, err := app.FindRecordsByFilter("users", "role = 'admin'", "", 0, 0, nil)
	if err != nil {
		return fmt.Errorf("failed to find admins: %w", err)
	}
	for _, admin := range admins {
		notif := core.NewRecord(notifCol)
		notif.Set("user", admin.Id)
		notif.Set("type", notifType)
		notif.Set("title", title)
		notif.Set("body", body)
		if relatedPartidoID != "" {
			notif.Set("related_partido", relatedPartidoID)
		}
		if err := app.Save(notif); err != nil {
			log.Printf("failed to notify admin %s: %v", admin.Id, err)
		}
	}
	return nil
}
