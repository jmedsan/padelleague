// Package search provides a full-text search index with accent folding,
// fuzzy matching, and scope-based visibility filtering.
package search

import (
	"fmt"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// Build harvests searchable entries from the database and static pages.
func Build(app core.App) []Entry {
	var entries []Entry
	entries = append(entries, staticPages...)
	entries = append(entries, buildUsers(app)...)
	entries = append(entries, buildCompetitions(app)...)
	entries = append(entries, buildMatches(app)...)
	entries = append(entries, buildMessages(app)...)
	entries = append(entries, buildDocuments(app)...)
	entries = append(entries, buildVenues(app)...)
	entries = append(entries, buildPairs(app)...)
	entries = append(entries, buildPenalties(app)...)
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
		entries = append(entries, NewEntry(Entry{
			Label:    u.GetString("display_name"),
			Type:     "jugador",
			URL:      league.EntityURL("player", u.Id),
			Keywords: []string{"jugador"},
			Scope:    Scope{Public: true},
		}))
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
		compType := c.GetString("type")
		kw := []string{"competición"}
		switch compType {
		case "league":
			kw = append(kw, "liga")
		case "playoff":
			kw = append(kw, "playoff")
		}
		entries = append(entries, NewEntry(Entry{
			Label:     c.GetString("name"),
			Secondary: "",
			Type:      "competición",
			URL:       league.EntityURL("competition", c.Id),
			Keywords:  kw,
			Scope:     Scope{Public: true},
		}))
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
	compNames := make(map[string]string)
	for _, m := range matches {
		allPairIDs[m.GetString("pair1")] = struct{}{}
		allPairIDs[m.GetString("pair2")] = struct{}{}
		cid := m.GetString("competition")
		if _, ok := compNames[cid]; !ok {
			if c, err := app.FindRecordById("competitions", cid); err == nil {
				compNames[cid] = c.GetString("name")
			}
		}
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
		secondary := compNames[m.GetString("competition")]
		if score := m.GetString("scores"); score != "" {
			secondary += " · " + score
		}
		entries = append(entries, NewEntry(Entry{
			Label:     label,
			Secondary: secondary,
			Type:      "partido",
			URL:       league.EntityURL("match", m.Id),
			Keywords:  []string{"partido", fmt.Sprintf("jornada %d", round), p1, p2},
			Scope:     Scope{Public: true},
		}))
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
		entries = append(entries, NewEntry(Entry{
			Label: content,
			Type:  "mensaje",
			URL:   league.EntityURL("match", matchID),
			Scope: Scope{CompID: compID},
		}))
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
		title := d.GetString("title")
		desc := d.GetString("description")
		if len(compIDs) == 0 {
			entries = append(entries, NewEntry(Entry{
				Label: title, Secondary: desc,
				Type: "documento", URL: url,
				Keywords: []string{"documento"},
				Scope:    Scope{Admin: true},
			}))
		} else {
			for _, cid := range compIDs {
				entries = append(entries, NewEntry(Entry{
					Label: title, Secondary: desc,
					Type: "documento", URL: url,
					Keywords: []string{"documento"},
					Scope:    Scope{CompID: cid},
				}))
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
		entries = append(entries, NewEntry(Entry{
			Label:    v.GetString("name"),
			Type:     "pista",
			URL:      "/admin/venues",
			Keywords: []string{"pista"},
			Scope:    Scope{Admin: true},
		}))
	}
	return entries
}

func buildPairs(app core.App) []Entry {
	pairs, err := app.FindRecordsByFilter("pairs", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build pairs", "err", err)
		return nil
	}

	pairComps := make(map[string]map[string]struct{})
	matches, _ := app.FindRecordsByFilter("matches", "", "", 0, 0, nil)
	for _, m := range matches {
		compID := m.GetString("competition")
		for _, pid := range []string{m.GetString("pair1"), m.GetString("pair2")} {
			if pairComps[pid] == nil {
				pairComps[pid] = make(map[string]struct{})
			}
			pairComps[pid][compID] = struct{}{}
		}
	}

	var entries []Entry
	for _, p := range pairs {
		p1Name := league.PlayerName(app, p.GetString("player1"))
		p2Name := league.PlayerName(app, p.GetString("player2"))
		secondary := p1Name + " / " + p2Name
		compIDs := pairComps[p.Id]
		if len(compIDs) == 0 {
			entries = append(entries, NewEntry(Entry{
				Label: p.GetString("name"), Secondary: secondary,
				Type: "pareja", URL: league.EntityURL("pair", p.Id),
				Keywords: []string{"pareja", p1Name, p2Name},
				Scope:    Scope{Admin: true},
			}))
		} else {
			for cid := range compIDs {
				entries = append(entries, NewEntry(Entry{
					Label: p.GetString("name"), Secondary: secondary,
					Type: "pareja", URL: league.EntityURL("pair", p.Id),
					Keywords: []string{"pareja", p1Name, p2Name},
					Scope:    Scope{CompID: cid},
				}))
			}
		}
	}
	return entries
}

func buildPenalties(app core.App) []Entry {
	penalties, err := app.FindRecordsByFilter("penalties", "voided = false", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build penalties", "err", err)
		return nil
	}

	pairIDs := make([]string, 0, len(penalties))
	competitionIDs := make(map[string]struct{}, len(penalties))
	for _, penalty := range penalties {
		pairIDs = append(pairIDs, penalty.GetString("pair"))
		competitionIDs[penalty.GetString("competition")] = struct{}{}
	}
	pairNames := league.PairNames(app, pairIDs)
	competitionNames := make(map[string]string, len(competitionIDs))
	for competitionID := range competitionIDs {
		competition, err := app.FindRecordById("competitions", competitionID)
		if err == nil {
			competitionNames[competitionID] = competition.GetString("name")
		}
	}

	entries := make([]Entry, 0, len(penalties))
	for _, penalty := range penalties {
		competitionID := penalty.GetString("competition")
		entries = append(entries, NewEntry(Entry{
			Label:     penalty.GetString("reason"),
			Secondary: fmt.Sprintf("%s · %s", pairNames[penalty.GetString("pair")], competitionNames[competitionID]),
			Type:      "penalización",
			URL:       "/admin/competitions/" + competitionID,
			Keywords:  []string{competitionNames[competitionID]},
			Scope:     Scope{Admin: true},
		}))
	}
	return entries
}

// Rebuild replaces the index from a fresh build, keeping the previous index when
// the build yields nothing. Used at startup and by the rebuild cron.
func (ix *Index) Rebuild(app core.App) {
	entries := Build(app)
	if len(entries) == 0 {
		slog.Error("search: rebuild produced zero entries, keeping previous index")
		return
	}
	ix.Replace(entries)
	slog.Info("search: index rebuilt", "entries", len(entries))
}
