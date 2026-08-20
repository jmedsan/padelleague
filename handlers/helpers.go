package handlers

import (
	"fmt"
	"log"

	"github.com/pocketbase/pocketbase/core"
)

func getPlayerTeam(app core.App, userID string, match *core.Record) (int, error) {
	pair1, err := app.FindRecordById("pairs", match.GetString("pair1"))
	if err != nil {
		return 0, fmt.Errorf("pair1 not found: %w", err)
	}
	if pair1.GetString("player1") == userID || pair1.GetString("player2") == userID {
		return 1, nil
	}
	pair2, err := app.FindRecordById("pairs", match.GetString("pair2"))
	if err != nil {
		return 0, fmt.Errorf("pair2 not found: %w", err)
	}
	if pair2.GetString("player1") == userID || pair2.GetString("player2") == userID {
		return 2, nil
	}
	return 0, fmt.Errorf("user %s is not a participant", userID)
}

func expandPairNames(app core.App, pairIDs []string) (map[string]string, error) {
	names := make(map[string]string, len(pairIDs))
	for _, id := range pairIDs {
		if id == "" {
			continue
		}
		pair, err := app.FindRecordById("pairs", id)
		if err != nil {
			names[id] = "Pareja desconocida"
			continue
		}
		names[id] = pair.GetString("name")
	}
	return names, nil
}

func resolvePlayerName(app core.App, userID string) string {
	if userID == "" {
		return "?"
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return "?"
	}
	return user.GetString("display_name")
}

func getPlayersForPair(app core.App, pairID string) []string {
	pair, err := app.FindRecordById("pairs", pairID)
	if err != nil {
		return nil
	}
	var userIDs []string
	if p1 := pair.GetString("player1"); p1 != "" {
		userIDs = append(userIDs, p1)
	}
	if p2 := pair.GetString("player2"); p2 != "" {
		userIDs = append(userIDs, p2)
	}
	return userIDs
}

func findPairsForPlayer(app core.App, userID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("pairs",
		"player1 = {:uid} || player2 = {:uid}",
		"", 0, 0,
		map[string]any{"uid": userID})
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

func notifyPlayers(app core.App, playerUserIDs []string, notifType, title, body, relatedMatchID string) {
	notifCol, err := app.FindCollectionByNameOrId("notifications")
	if err != nil {
		log.Printf("notifyPlayers: notifications collection not found: %v", err)
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
		if relatedMatchID != "" {
			notif.Set("related_match", relatedMatchID)
		}
		if err := app.Save(notif); err != nil {
			log.Printf("notifyPlayers: failed to notify user %s: %v", userID, err)
		}
	}
}

func notifyAdmins(app core.App, notifType, title, body, relatedMatchID string) error {
	notifCol, err := app.FindCollectionByNameOrId("notifications")
	if err != nil {
		return fmt.Errorf("notifications collection not found: %w", err)
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
		if relatedMatchID != "" {
			notif.Set("related_match", relatedMatchID)
		}
		if err := app.Save(notif); err != nil {
			log.Printf("failed to notify admin %s: %v", admin.Id, err)
		}
	}
	return nil
}
