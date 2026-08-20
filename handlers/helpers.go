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

func getPlayersForPair(app core.App, parejaID string) []string {
	pareja, err := app.FindRecordById("parejas", parejaID)
	if err != nil {
		return nil
	}
	var userIDs []string
	for _, jID := range []string{pareja.GetString("jugador1"), pareja.GetString("jugador2")} {
		if jID == "" {
			continue
		}
		jugador, err := app.FindRecordById("jugadores", jID)
		if err != nil {
			continue
		}
		if uid := jugador.GetString("user"); uid != "" {
			userIDs = append(userIDs, uid)
		}
	}
	return userIDs
}

func getNotificationPrefs(user *core.Record) map[string]any {
	defaults := map[string]any{
		"quorum_request": true,
		"dispute":        true,
		"match_assigned": true,
		"general":        true,
	}
	raw := user.Get("notification_prefs")
	if raw == nil {
		return defaults
	}
	prefs, ok := raw.(map[string]any)
	if !ok {
		return defaults
	}
	for k, v := range defaults {
		if _, exists := prefs[k]; !exists {
			prefs[k] = v
		}
	}
	return prefs
}

func notifyPlayers(app core.App, playerUserIDs []string, notifType, title, body, relatedPartidoID string) {
	notifCol, err := app.FindCollectionByNameOrId("notificaciones")
	if err != nil {
		log.Printf("notifyPlayers: notificaciones collection not found: %v", err)
		return
	}
	for _, userID := range playerUserIDs {
		user, err := app.FindRecordById("users", userID)
		if err != nil {
			continue
		}
		prefs := getNotificationPrefs(user)
		if enabled, ok := prefs[notifType]; ok {
			if b, ok := enabled.(bool); ok && !b {
				continue
			}
		}
		notif := core.NewRecord(notifCol)
		notif.Set("user", userID)
		notif.Set("type", notifType)
		notif.Set("title", title)
		notif.Set("body", body)
		if relatedPartidoID != "" {
			notif.Set("related_partido", relatedPartidoID)
		}
		if err := app.Save(notif); err != nil {
			log.Printf("notifyPlayers: failed to notify user %s: %v", userID, err)
		}
	}
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
