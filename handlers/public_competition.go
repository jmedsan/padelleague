package handlers

import (
	"fmt"
	"net/http"
	"slices"
	"sort"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/search"
)

// RoundView groups matches by round number for the competition page.
type RoundView struct {
	RoundNumber int
	Key         string // unique group key for auto-expand (e.g. "round-1" or "pending")
	Title       string // display title (e.g. "Jornada 1" or "Por jugar")
	Matches     []MatchCard
}

// BracketRound holds one round of a single-elimination bracket for display.
// Slot is the CSS vertical slot height (in px) each match in this round
// occupies, doubling each round so a match's slot always spans the midpoint
// of its two feeder matches in the previous round — see the ".bracket" CSS
// connector rules in static/css/input.css.
type BracketRound struct {
	Name    string
	Index   int
	Slot    int
	Matches []MatchCard
}

func buildBracket(rounds []RoundView, maxRound int) []BracketRound {
	var bracket []BracketRound
	for i, r := range rounds {
		bracket = append(bracket, BracketRound{
			Name:    bracketRoundName(r.RoundNumber, maxRound),
			Index:   i,
			Slot:    84 << i,
			Matches: r.Matches,
		})
	}
	return bracket
}

func bracketRoundName(round, maxRound int) string {
	remaining := maxRound - round
	switch remaining {
	case 0:
		return "Final"
	case 1:
		return "Semifinales"
	case 2:
		return "Cuartos"
	case 3:
		return "Octavos"
	case 4:
		return "Dieciseisavos"
	default:
		return fmt.Sprintf("Ronda %d", round)
	}
}

// Competition renders the public competition page with standings and fixtures.
func (h *PublicHandler) Competition(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return h.render.ErrorPage(e, http.StatusNotFound, "Competición no encontrada")
	}

	userID := e.Auth.Id
	pairs, _ := league.PairsForPlayer(h.app, userID)
	playerPairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		playerPairIDs[p.Id] = struct{}{}
	}

	if gated, err := h.docsGate(e, comp, userID, playerPairIDs); gated || err != nil {
		return err
	}

	isAdmin := isEffectiveAdmin(e)
	published := comp.GetString("calendar_status") == "published"

	var matches []*core.Record
	if published || isAdmin {
		matches = findRecordsLogged(h.app, "Competition: find matches", RecordQuery{
			Collection: "matches", Filter: "competition = {:cid}",
			Sort: "round_number,created", Params: map[string]any{"cid": id},
		})
	}

	pairNames := collectPairNames(h.app, matches)

	isPlayoff := league.IsPlayoff(comp)
	isLeveled := league.IsLeveled(comp)
	compPairIDs := comp.GetStringSlice("pairs")

	pairFilter := resolvePairFilter(e.Request.URL.Query(), pairFilterCtx{
		IsPlayoff:     isPlayoff,
		IsLeveled:     isLeveled,
		CompPairIDs:   compPairIDs,
		PlayerPairIDs: playerPairIDs,
	})

	rounds := h.buildCompRounds(matches, roundsCtx{
		PairNames:     pairNames,
		PlayerPairIDs: playerPairIDs,
		PairFilter:    pairFilter,
		IsLeveled:     isLeveled,
		Window:        leveledWindowFor(comp),
	})
	autoExpandRound := firstIncompleteRound(rounds)

	data := h.buildCompetitionData(comp, rounds, autoExpandRound, published || isAdmin)
	data["IsLeveled"] = isLeveled
	h.populateCompetitionData(data, competitionDataParams{
		e: e, comp: comp, isPlayoff: isPlayoff, compPairIDs: compPairIDs,
		playerPairIDs: playerPairIDs, pairFilter: pairFilter, rounds: rounds, userID: userID,
		isLeveled: isLeveled, matches: matches,
	})
	return h.render.Page(e, "competition.html", data)
}

type roundsCtx struct {
	PairNames     map[string]string
	PlayerPairIDs map[string]struct{}
	PairFilter    string
	IsLeveled     bool
	Window        leveledWindow
}

func (h *PublicHandler) buildCompRounds(matches []*core.Record, ctx roundsCtx) []RoundView {
	if ctx.IsLeveled {
		tz := league.Timezone(h.app)
		return buildLeveledRounds(matches, leveledRoundsCtx{
			PairNames:     ctx.PairNames,
			PlayerPairIDs: ctx.PlayerPairIDs,
			PairFilter:    ctx.PairFilter,
			Window:        ctx.Window,
		}, tz)
	}
	rounds := buildRounds(matches, ctx.PairNames, ctx.PlayerPairIDs, ctx.PairFilter)
	for i := range rounds {
		enrichWithPendingResults(h.app, rounds[i].Matches)
	}
	return rounds
}

// pairFilterCtx holds competition-level info for resolvePairFilter.
type pairFilterCtx struct {
	IsPlayoff     bool
	IsLeveled     bool
	CompPairIDs   []string
	PlayerPairIDs map[string]struct{}
}

// resolvePairFilter determines the active pair filter for the competition page.
// For leveled leagues with no URL "pair" key, defaults to the viewer's own pair.
// Playoffs always show all pairs.
func resolvePairFilter(query map[string][]string, ctx pairFilterCtx) string {
	if ctx.IsPlayoff {
		return ""
	}
	urlPair, hasPairKey := query["pair"]
	pairFilter := ""
	if hasPairKey {
		pairFilter = urlPair[0]
	}
	if pairFilter != "all" && pairFilter != "" && !slices.Contains(ctx.CompPairIDs, pairFilter) {
		pairFilter = ""
	}
	if ctx.IsLeveled && !hasPairKey {
		for pid := range ctx.PlayerPairIDs {
			if slices.Contains(ctx.CompPairIDs, pid) {
				return pid
			}
		}
	}
	return pairFilter
}

// competitionDataParams bundles the inputs populateCompetitionData needs
// beyond the data map itself.
type competitionDataParams struct {
	e             *core.RequestEvent
	comp          *core.Record
	isPlayoff     bool
	compPairIDs   []string
	playerPairIDs map[string]struct{}
	pairFilter    string
	rounds        []RoundView
	userID        string
	isLeveled     bool
	matches       []*core.Record
}

// populateCompetitionData fills in the remaining page-data fields for
// Competition that don't depend on match/round computation.
func (h *PublicHandler) populateCompetitionData(data map[string]any, p competitionDataParams) {
	comp := p.comp
	data["PageTitle"] = comp.GetString("name")
	data["PlayerPairIDs"] = p.playerPairIDs
	data["Mode"] = PlayerSummary
	data["CalendarDraft"] = comp.GetString("calendar_status") == "draft"
	data["OGImage"] = league.CompetitionLogoURL(comp.Id, comp.GetString("logo"))
	data["FooterCompetitionID"] = comp.Id
	if !p.isPlayoff {
		data["PairOptions"] = buildPairOptions(h.app, p.compPairIDs, p.playerPairIDs, p.pairFilter)
	}
	if p.pairFilter != "" && p.pairFilter != "all" {
		data["ActiveTab"] = "jornadas"
	}
	if p.pairFilter != "" && p.pairFilter != "all" && len(p.rounds) == 0 {
		data["FilterEmptyState"] = "Sin partidos para esta pareja"
	}
	if msg := leveledInfoMessage(comp, p); msg != "" {
		data["LeveledInfoMessage"] = msg
	}
	h.addCompetitionDocViews(data, comp, p.userID, fileTokenFor(p.e))
	data["Announcements"] = findRecordsLogged(h.app, "Competition: find announcements", RecordQuery{
		Collection: "announcements",
		Filter:     "competition = {:cid}",
		Sort:       "-created",
		Params:     map[string]any{"cid": comp.Id},
	})
}

// leveledInfoMessage returns the informational message telling a player
// filtered to their own pair that new matches get assigned once current
// ones are finished — shown only while their pair hasn't reached
// target_matches yet. Returns "" when not applicable (round-robin, no pair
// filter, or the pair's schedule is already complete).
func leveledInfoMessage(comp *core.Record, p competitionDataParams) string {
	if !p.isLeveled || p.pairFilter == "" || p.pairFilter == "all" {
		return ""
	}
	target := comp.GetInt("target_matches")
	var pending, total int
	for _, m := range p.matches {
		if m.GetString("pair1") != p.pairFilter && m.GetString("pair2") != p.pairFilter {
			continue
		}
		total++
		if m.GetString("status") != league.StatusFinal {
			pending++
		}
	}
	if total >= target {
		return ""
	}
	if pending == 0 {
		return "No tienes partidos pendientes. Se te asignará el siguiente en breve."
	}
	return fmt.Sprintf("Tienes %d partidos pendientes. Cuando termines uno se te asignará el siguiente.", pending)
}

// docsGate renders the mandatory-documents gate page and reports gated=true
// when the player has unacknowledged mandatory documents; the caller must
// stop processing the request in that case regardless of err.
func (h *PublicHandler) docsGate(e *core.RequestEvent, comp *core.Record, userID string, playerPairIDs map[string]struct{}) (gated bool, err error) {
	if !league.IsParticipant(comp, playerPairIDs) {
		return false, nil
	}
	pending := league.UnacknowledgedMandatory(h.app, comp, userID)
	if len(pending) == 0 {
		return false, nil
	}
	allDocs := league.AttachedDocuments(h.app, comp)
	mandatoryIDs := make([]string, len(pending))
	for i, d := range pending {
		mandatoryIDs[i] = d.Id
	}
	ft := fileTokenFor(e)
	docViews := make([]DocumentView, len(allDocs))
	for i, d := range allDocs {
		docViews[i] = NewDocumentView(d, PlayerSummary, ft)
	}
	err = h.render.Page(e, "competition-docs-gate.html", map[string]any{
		"PageTitle":           "Documentos",
		"Competition":         comp,
		"DocumentViews":       docViews,
		"MandatoryIDs":        mandatoryIDs,
		"Mode":                PlayerRow,
		"FooterCompetitionID": comp.Id,
	})
	return true, err
}

// addCompetitionDocViews always sets DocumentView entries on data — an empty
// slice when the competition has no attached documents, so the Documentos
// tab always renders (matching the admin view) with an empty state rather
// than disappearing entirely, marking which docs userID has acknowledged.
func (h *PublicHandler) addCompetitionDocViews(data map[string]any, comp *core.Record, userID, fileToken string) {
	docs := league.AttachedDocuments(h.app, comp)
	ackedSlice := league.AckedDocIDs(h.app, comp.Id, userID)
	ackedSet := make(map[string]struct{}, len(ackedSlice))
	for _, id := range ackedSlice {
		ackedSet[id] = struct{}{}
	}
	docViews := make([]DocumentView, len(docs))
	for i, d := range docs {
		docViews[i] = NewDocumentViewWithAck(d, PlayerSummary, fileToken, ackedSet)
	}
	data["DocumentViews"] = docViews
}

// AcceptDocs records that the player has read the competition's mandatory documents.
func (h *PublicHandler) AcceptDocs(e *core.RequestEvent) error {
	comp, err := h.app.FindRecordById("competitions", e.Request.PathValue("id"))
	if err != nil {
		return h.render.ErrorPage(e, http.StatusNotFound, "Competición no encontrada")
	}
	mandatoryIDs := league.MandatoryDocIDs(h.app, comp)
	ack, err := league.FindOrNewAck(h.app, comp.Id, e.Auth.Id)
	if err != nil {
		return alertError(e, "Error al registrar la lectura")
	}
	ack.Set("documents", mandatoryIDs)
	if err := h.app.Save(ack); err != nil {
		return alertError(e, "Error al registrar la lectura")
	}
	return redirectHX(e, "/competition/"+comp.Id)
}

// competitionStandings computes a league competition's standings. A nil
// result means "no rows to show yet" (fewer than 2 pairs have standings, or
// no match has been played) — the caller renders the tab's empty state, not
// hides the tab; the tab's own visibility is showStandingsTab, gated only on
// competition type and showFixtures.
func (h *PublicHandler) competitionStandings(comp *core.Record, showFixtures bool) ([]league.StandingRowFull, bool) {
	if comp.GetString("type") != "league" || !showFixtures {
		return nil, false
	}
	rows, _ := h.leagueSvc.ComputeStandings(comp.Id)
	hasPlayed, hasPenalties := false, false
	for _, s := range rows {
		if s.Played > 0 {
			hasPlayed = true
		}
		if s.Penalty > 0 {
			hasPenalties = true
		}
	}
	if len(rows) < 2 || !hasPlayed {
		return nil, hasPenalties
	}
	return rows, hasPenalties
}

func (h *PublicHandler) buildCompetitionData(comp *core.Record, rounds []RoundView, autoExpandRound string, showFixtures bool) map[string]any {
	id := comp.Id
	standings, hasPenalties := h.competitionStandings(comp, showFixtures)
	showStandingsTab := comp.GetString("type") == "league" && showFixtures

	var awards []league.Award
	if !comp.GetBool("active") && showFixtures {
		awards = h.leagueSvc.Awards(id)
	}

	isPlayoff := league.IsPlayoff(comp)
	var bracket []BracketRound
	if isPlayoff && len(rounds) > 0 {
		maxRound := rounds[len(rounds)-1].RoundNumber
		bracket = buildBracket(rounds, maxRound)
	}

	withdrawnIDs := comp.GetStringSlice("withdrawn_pairs")
	wp := make(map[string]bool, len(withdrawnIDs))
	for _, wid := range withdrawnIDs {
		wp[wid] = true
	}

	return map[string]any{
		"Competition":      comp,
		"Rounds":           rounds,
		"Standings":        standings,
		"ShowStandingsTab": showStandingsTab,
		"Awards":           awards,
		"IsArchived":       !comp.GetBool("active"),
		"AutoExpandRound":  autoExpandRound,
		"HasPenalties":     hasPenalties,
		"IsPlayoff":        isPlayoff,
		"Bracket":          bracket,
		"WithdrawnPairs":   wp,
	}
}

// PairOption is one entry in the Jornadas team filter <select>.
type PairOption struct {
	ID       string
	Name     string
	IsOwn    bool
	Selected bool
}

// buildPairOptions returns the competition's pairs for the team filter:
// the viewer's own pair(s) first (name prefixed with a star), then every
// other pair sorted alphabetically (accent-folded) for "Otras parejas".
func buildPairOptions(app core.App, compPairIDs []string, playerPairIDs map[string]struct{}, selected string) []PairOption {
	names := league.PairNames(app, compPairIDs)
	options := make([]PairOption, 0, len(compPairIDs))
	for _, pid := range compPairIDs {
		_, isOwn := playerPairIDs[pid]
		options = append(options, PairOption{
			ID:       pid,
			Name:     names[pid],
			IsOwn:    isOwn,
			Selected: pid == selected,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].IsOwn != options[j].IsOwn {
			return options[i].IsOwn
		}
		return search.Fold(options[i].Name) < search.Fold(options[j].Name)
	})
	return options
}

func collectPairNames(app core.App, matches []*core.Record) map[string]string {
	ids := make(map[string]struct{})
	for _, m := range matches {
		ids[m.GetString("pair1")] = struct{}{}
		ids[m.GetString("pair2")] = struct{}{}
	}
	slice := make([]string, 0, len(ids))
	for pid := range ids {
		slice = append(slice, pid)
	}
	return league.PairNames(app, slice)
}

func buildRounds(matches []*core.Record, pairNames map[string]string, playerPairIDs map[string]struct{}, pairFilter string) []RoundView {
	roundMap := map[int][]MatchCard{}
	for _, m := range matches {
		if pairFilter != "" && m.GetString("pair1") != pairFilter && m.GetString("pair2") != pairFilter {
			continue
		}
		mc := NewMatchRow(m, pairNames, playerPairIDs)
		rn := int(m.GetFloat("round_number"))
		roundMap[rn] = append(roundMap[rn], mc)
	}
	for rn, ms := range roundMap {
		sort.SliceStable(ms, func(i, j int) bool {
			return ms[i].IsMyMatch && !ms[j].IsMyMatch
		})
		roundMap[rn] = ms
	}
	roundNums := make([]int, 0, len(roundMap))
	for rn := range roundMap {
		roundNums = append(roundNums, rn)
	}
	sort.Ints(roundNums)
	rounds := make([]RoundView, 0, len(roundNums))
	for _, rn := range roundNums {
		key := fmt.Sprintf("round-%d", rn)
		rounds = append(rounds, RoundView{RoundNumber: rn, Key: key, Title: fmt.Sprintf("Jornada %d", rn), Matches: roundMap[rn]})
	}
	return rounds
}

func firstIncompleteRound(rounds []RoundView) string {
	for _, rv := range rounds {
		for _, mv := range rv.Matches {
			if mv.Match.GetString("status") != league.StatusFinal {
				return rv.Key
			}
		}
	}
	return ""
}
