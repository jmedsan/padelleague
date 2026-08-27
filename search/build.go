package search

import (
	"fmt"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

func Build(app core.App) []Entry {
	var entries []Entry
	entries = append(entries, staticPages...)
	entries = append(entries, buildUsers(app)...)
	entries = append(entries, buildCompetitions(app)...)
	entries = append(entries, buildMatches(app)...)
	entries = append(entries, buildMessages(app)...)
	entries = append(entries, buildDocuments(app)...)
	entries = append(entries, buildVenues(app)...)
	return entries
}

func buildUsers(app core.App) []Entry {
	users, err := app.FindRecordsByFilter("users", "roles ~ 'player'", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build users", "err", err)
		return nil
	}
	entries := make([]Entry, 0, len(users))
	for _, u := range users {
		entries = append(entries, NewEntry(
			u.GetString("display_name"),
			"",
			"jugador",
			"/player/"+u.Id,
			Scope{Public: true},
		))
	}
	return entries
}

func buildCompetitions(app core.App) []Entry {
	comps, err := app.FindRecordsByFilter("competitions", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build competitions", "err", err)
		return nil
	}
	entries := make([]Entry, 0, len(comps))
	for _, c := range comps {
		entries = append(entries, NewEntry(
			c.GetString("name"),
			c.GetString("category"),
			"competición",
			"/competition/"+c.Id,
			Scope{Public: true},
		))
	}
	return entries
}

func buildMatches(app core.App) []Entry {
	matches, err := app.FindRecordsByFilter("matches", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build matches", "err", err)
		return nil
	}

	allPairIDs := make(map[string]struct{})
	for _, m := range matches {
		allPairIDs[m.GetString("pair1")] = struct{}{}
		allPairIDs[m.GetString("pair2")] = struct{}{}
	}
	ids := make([]string, 0, len(allPairIDs))
	for id := range allPairIDs {
		ids = append(ids, id)
	}
	pairNames := league.PairNames(app, ids)

	entries := make([]Entry, 0, len(matches))
	for _, m := range matches {
		p1 := pairNames[m.GetString("pair1")]
		p2 := pairNames[m.GetString("pair2")]
		round := int(m.GetFloat("round_number"))
		label := fmt.Sprintf("%s vs %s (J%d)", p1, p2, round)
		secondary := m.GetString("scores")
		entries = append(entries, NewEntry(
			label,
			secondary,
			"partido",
			"/match/"+m.Id,
			Scope{Public: true},
		))
	}
	return entries
}

func buildMessages(app core.App) []Entry {
	msgs, err := app.FindRecordsByFilter("match_messages", "type = 'chat'", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build messages", "err", err)
		return nil
	}

	matchCompMap := make(map[string]string)
	for _, msg := range msgs {
		matchCompMap[msg.GetString("match")] = ""
	}
	for matchID := range matchCompMap {
		m, err := app.FindRecordById("matches", matchID)
		if err != nil {
			continue
		}
		matchCompMap[matchID] = m.GetString("competition")
	}

	entries := make([]Entry, 0, len(msgs))
	for _, msg := range msgs {
		content := msg.GetString("content")
		if len(content) > 100 {
			content = content[:100]
		}
		matchID := msg.GetString("match")
		compID := matchCompMap[matchID]
		entries = append(entries, NewEntry(
			content,
			"",
			"mensaje",
			"/match/"+matchID,
			Scope{CompID: compID},
		))
	}
	return entries
}

func buildDocuments(app core.App) []Entry {
	docs, err := app.FindRecordsByFilter("documents", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build documents", "err", err)
		return nil
	}

	comps, _ := app.FindRecordsByFilter("competitions", "", "", 0, 0, nil)
	docCompMap := make(map[string][]string)
	for _, c := range comps {
		for _, did := range c.GetStringSlice("documents") {
			docCompMap[did] = append(docCompMap[did], c.Id)
		}
	}

	var entries []Entry
	for _, d := range docs {
		compIDs := docCompMap[d.Id]
		url := d.GetString("url")
		if url == "" {
			url = "/admin/documents"
		}
		if len(compIDs) == 0 {
			entries = append(entries, NewEntry(
				d.GetString("title"),
				d.GetString("description"),
				"documento",
				url,
				Scope{Admin: true},
			))
		} else {
			for _, cid := range compIDs {
				entries = append(entries, NewEntry(
					d.GetString("title"),
					d.GetString("description"),
					"documento",
					url,
					Scope{CompID: cid},
				))
			}
		}
	}
	return entries
}

func buildVenues(app core.App) []Entry {
	venues, err := app.FindRecordsByFilter("venues", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build venues", "err", err)
		return nil
	}
	entries := make([]Entry, 0, len(venues))
	for _, v := range venues {
		entries = append(entries, NewEntry(
			v.GetString("name"),
			"",
			"pista",
			"/admin/venues",
			Scope{Admin: true},
		))
	}
	return entries
}
