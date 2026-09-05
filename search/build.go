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
		entries = append(entries, buildUserEntry(u))
	}
	return entries
}

func buildUserEntry(u *core.Record) Entry {
	return NewEntry(Entry{
		Label:    u.GetString("display_name"),
		Type:     "jugador",
		URL:      league.EntityURL("player", u.Id),
		Keywords: []string{"jugador"},
		Scope:    Scope{Public: true},
		RecordID: u.Id,
	})
}

func buildCompetitions(app core.App) []Entry {
	comps, err := app.FindRecordsByFilter("competitions", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build competitions", "err", err)
		return nil
	}
	entries := make([]Entry, 0, len(comps))
	for _, c := range comps {
		entries = append(entries, buildCompetitionEntry(c))
	}
	return entries
}

func buildCompetitionEntry(c *core.Record) Entry {
	compType := c.GetString("type")
	kw := []string{"competición"}
	switch compType {
	case "league":
		kw = append(kw, "liga")
	case "playoff":
		kw = append(kw, "playoff")
	}
	return NewEntry(Entry{
		Label:     c.GetString("name"),
		Secondary: "",
		Type:      "competición",
		URL:       league.EntityURL("competition", c.Id),
		Keywords:  kw,
		Scope:     Scope{Public: true},
		RecordID:  c.Id,
	})
}

func buildMatches(app core.App) []Entry {
	matches, err := app.FindRecordsByFilter("matches", "", "", 0, 0, nil)
	if err != nil {
		slog.Error("search: build matches", "err", err)
		return nil
	}

	entries := make([]Entry, 0, len(matches))
	for _, m := range matches {
		entries = append(entries, buildMatchEntry(app, m))
	}
	return entries
}

func buildMatchEntry(app core.App, m *core.Record) Entry {
	pairNames := league.PairNames(app, []string{m.GetString("pair1"), m.GetString("pair2")})
	p1 := pairNames[m.GetString("pair1")]
	p2 := pairNames[m.GetString("pair2")]
	round := int(m.GetFloat("round_number"))
	label := fmt.Sprintf("%s vs %s (J%d)", p1, p2, round)
	secondary := league.CompetitionName(app, m.GetString("competition"))
	if score := m.GetString("scores"); score != "" {
		secondary += " · " + score
	}
	return NewEntry(Entry{
		Label:     label,
		Secondary: secondary,
		Type:      "partido",
		URL:       league.EntityURL("match", m.Id),
		Keywords:  []string{"partido", fmt.Sprintf("jornada %d", round), p1, p2},
		Scope:     Scope{Public: true},
		RecordID:  m.Id,
	})
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
			Label:    content,
			Type:     "mensaje",
			URL:      league.EntityURL("match", matchID),
			Scope:    Scope{CompID: compID},
			RecordID: msg.Id,
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
				RecordID: d.Id,
			}))
		} else {
			for _, cid := range compIDs {
				entries = append(entries, NewEntry(Entry{
					Label: title, Secondary: desc,
					Type: "documento", URL: url,
					Keywords: []string{"documento"},
					Scope:    Scope{CompID: cid},
					RecordID: d.Id,
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
		entries = append(entries, buildVenueEntry(v))
	}
	return entries
}

func buildVenueEntry(v *core.Record) Entry {
	return NewEntry(Entry{
		Label:    v.GetString("name"),
		Type:     "pista",
		URL:      "/admin/venues",
		Keywords: []string{"pista"},
		Scope:    Scope{Admin: true},
		RecordID: v.Id,
	})
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
		compIDs := make([]string, 0, len(pairComps[p.Id]))
		for cid := range pairComps[p.Id] {
			compIDs = append(compIDs, cid)
		}
		entries = append(entries, buildPairEntries(app, p, compIDs)...)
	}
	return entries
}

// buildPairEntries builds the searchable entries for one pair, fanned out
// once per competition it has played in (or a single admin-only entry when
// it hasn't played any yet).
func buildPairEntries(app core.App, p *core.Record, compIDs []string) []Entry {
	p1Name := league.PlayerName(app, p.GetString("player1"))
	p2Name := league.PlayerName(app, p.GetString("player2"))
	secondary := p1Name + " / " + p2Name
	if len(compIDs) == 0 {
		return []Entry{NewEntry(Entry{
			Label: p.GetString("name"), Secondary: secondary,
			Type: "pareja", URL: league.EntityURL("pair", p.Id),
			Keywords: []string{"pareja", p1Name, p2Name},
			Scope:    Scope{Admin: true},
			RecordID: p.Id,
		})}
	}
	entries := make([]Entry, 0, len(compIDs))
	for _, cid := range compIDs {
		entries = append(entries, NewEntry(Entry{
			Label: p.GetString("name"), Secondary: secondary,
			Type: "pareja", URL: league.EntityURL("pair", p.Id),
			Keywords: []string{"pareja", p1Name, p2Name},
			Scope:    Scope{CompID: cid},
			RecordID: p.Id,
		}))
	}
	return entries
}

// pairCompetitionIDs returns the distinct competitions a pair has played
// matches in, by scanning all matches. Used to scope a pair's search
// entries; a hook handler should prefer a filtered query when updating a
// single pair.
func pairCompetitionIDs(app core.App, pairID string) []string {
	matches, _ := app.FindRecordsByFilter(
		"matches", "pair1 = {:pid} || pair2 = {:pid}", "", 0, 0,
		map[string]any{"pid": pairID},
	)
	seen := make(map[string]struct{})
	var ids []string
	for _, m := range matches {
		cid := m.GetString("competition")
		if _, ok := seen[cid]; !ok {
			seen[cid] = struct{}{}
			ids = append(ids, cid)
		}
	}
	return ids
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
			RecordID:  penalty.Id,
		}))
	}
	return entries
}

// UpsertRecord refreshes the search entries for a single created or updated
// record, without waiting for the next full rebuild. Unknown collections are
// ignored. The periodic Rebuild remains the safety net that reconciles any
// drift (deletes, fan-out changes from unrelated records).
func UpsertRecord(ix *Index, app core.App, collection string, record *core.Record) {
	switch collection {
	case "users":
		ix.Upsert(record.Id, []Entry{buildUserEntry(record)})
	case "competitions":
		ix.Upsert(record.Id, []Entry{buildCompetitionEntry(record)})
	case "matches":
		ix.Upsert(record.Id, []Entry{buildMatchEntry(app, record)})
	case "venues":
		ix.Upsert(record.Id, []Entry{buildVenueEntry(record)})
	case "pairs":
		compIDs := pairCompetitionIDs(app, record.Id)
		ix.Upsert(record.Id, buildPairEntries(app, record, compIDs))
	}
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
