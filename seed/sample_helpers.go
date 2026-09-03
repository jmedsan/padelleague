package seed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

func generateSampleBracket(txApp core.App, compID string, pairIDs []string) error {
	n := len(pairIDs)
	if n < 2 {
		return nil
	}
	numRounds := int(math.Ceil(math.Log2(float64(n))))
	bracketSize := 1 << numRounds

	slots := make([]string, bracketSize)
	copy(slots, pairIDs)

	matchCol, err := txApp.FindCollectionByNameOrId("matches")
	if err != nil {
		return err
	}

	advancers, err := bracketFirstRound(txApp, matchCol, compID, slots)
	if err != nil {
		return err
	}
	return bracketLaterRounds(txApp, matchCol, compID, advancers)
}

func bracketFirstRound(txApp core.App, matchCol *core.Collection, compID string, slots []string) ([]string, error) {
	n := len(slots)
	advancers := make([]string, n/2)
	semiDate := time.Now().UTC().Add(5 * 24 * time.Hour).Format("2006-01-02")
	for i := 0; i < n/2; i++ {
		p1, p2 := slots[i], slots[n-1-i]
		if p1 == "" && p2 == "" {
			continue
		}
		if p2 == "" {
			advancers[i] = p1
			continue
		}
		if p1 == "" {
			advancers[i] = p2
			continue
		}
		match := core.NewRecord(matchCol)
		match.Set("competition", compID)
		match.Set("round_number", 1)
		match.Set("matches_to_win", 1)
		match.Set("pair1", p1)
		match.Set("pair2", p2)
		match.Set("date", semiDate)
		match.Set("status", league.StatusScheduled)
		if err := txApp.Save(match); err != nil {
			return nil, fmt.Errorf("create playoff match r1: %w", err)
		}
	}
	return advancers, nil
}

func bracketLaterRounds(txApp core.App, matchCol *core.Collection, compID string, advancers []string) error {
	for r := 2; len(advancers) >= 2; r++ {
		numMatches := len(advancers) / 2
		nextAdvancers := make([]string, numMatches)
		roundDate := time.Now().UTC().Add(time.Duration(r*5) * 24 * time.Hour).Format("2006-01-02")
		for i := 0; i < numMatches; i++ {
			p1 := advancers[i*2]
			p2 := advancers[i*2+1]
			match := core.NewRecord(matchCol)
			match.Set("competition", compID)
			match.Set("round_number", r)
			match.Set("matches_to_win", 1)
			match.Set("date", roundDate)
			if p1 != "" {
				match.Set("pair1", p1)
			}
			if p2 != "" {
				match.Set("pair2", p2)
			}
			match.Set("status", league.StatusScheduled)
			if err := txApp.Save(match); err != nil {
				return fmt.Errorf("create playoff match r%d: %w", r, err)
			}
			nextAdvancers[i] = ""
		}
		advancers = nextAdvancers
	}
	return nil
}

func createSampleVenues(txApp core.App) error {
	existing, _ := txApp.FindRecordsByFilter("venues", "id != ''", "", 1, 0)
	if len(existing) > 0 {
		return nil
	}
	col, err := txApp.FindCollectionByNameOrId("venues")
	if err != nil {
		return err
	}
	venues := []struct{ name, address string }{
		{"Padel 360", "Calle del Deporte 12"},
		{"Wurko", "Avda. de la Constitución 45"},
		{"Tecnisur", "Camino Viejo de Málaga 8"},
	}
	for _, v := range venues {
		rec := core.NewRecord(col)
		rec.Set("name", v.name)
		rec.Set("address", v.address)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create sample venue %s: %w", v.name, err)
		}
	}
	return nil
}

func createSampleInvitations(txApp core.App, compID string) error {
	col, err := txApp.FindCollectionByNameOrId("invitations")
	if err != nil {
		return err
	}
	admins, _ := txApp.FindRecordsByFilter("users", "roles ~ 'admin'", "", 1, 0)
	var creatorID string
	if len(admins) > 0 {
		creatorID = admins[0].Id
	} else {
		users, _ := txApp.FindRecordsByFilter("users", "id != ''", "", 1, 0)
		if len(users) == 0 {
			return nil
		}
		creatorID = users[0].Id
	}

	for _, inv := range []struct {
		email  string
		status string
	}{
		{"invitado@example.com", "pending"},
		{"", "pending"},
	} {
		token := make([]byte, 16)
		if _, err := rand.Read(token); err != nil {
			return fmt.Errorf("generate invitation token: %w", err)
		}
		rec := core.NewRecord(col)
		rec.Set("token", hex.EncodeToString(token))
		rec.Set("email", inv.email)
		rec.Set("competition", compID)
		rec.Set("created_by", creatorID)
		rec.Set("status", inv.status)
		rec.Set("max_uses", 1)
		rec.Set("use_count", 0)
		rec.Set("expires_at", time.Now().UTC().Add(7*24*time.Hour))
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create sample invitation: %w", err)
		}
	}
	return nil
}

func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	prefix := email[:1]
	if at > 1 {
		prefix = email[:2]
	}
	return prefix + "***" + email[at:]
}

func saveSampleSchedule(txApp core.App, comp *core.Record) error {
	start := comp.GetDateTime("start_date").Time()
	end := comp.GetDateTime("end_date").Time()
	rounds := comp.GetInt("rounds")
	comp.Set("round_arrange_dates", league.StoreRoundSchedule(start, end, rounds))
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("save competition schedule: %w", err)
	}
	return nil
}
