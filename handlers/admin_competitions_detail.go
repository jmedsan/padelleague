package handlers

import (
	"fmt"
	"slices"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

func (h *CompetitionHandler) addDetailExtras(data map[string]any, comp *core.Record, matches []*core.Record, fileToken string) {
	if comp.GetString("type") == "league" {
		rows, _ := h.leagueSvc.ComputeStandings(comp.Id)
		hasPlayed := false
		for _, s := range rows {
			if s.Played > 0 {
				hasPlayed = true
			}
			if s.Penalty > 0 {
				data["HasPenalties"] = true
			}
		}
		if len(rows) >= 2 && hasPlayed {
			data["Standings"] = rows
		}
		if len(matches) > 0 {
			data["RoundDates"] = h.buildRoundDates(comp)
		}
		withdrawnIDs := comp.GetStringSlice("withdrawn_pairs")
		wp := make(map[string]bool, len(withdrawnIDs))
		for _, id := range withdrawnIDs {
			wp[id] = true
		}
		data["WithdrawnPairs"] = wp
	}
	attachedViews, unattachedDocs := h.buildDetailDocs(comp, fileToken)
	data["AttachedDocViews"] = attachedViews
	data["UnattachedDocs"] = unattachedDocs

	attachedSponsors, unattachedSponsors := h.buildDetailSponsors(comp)
	data["AttachedSponsors"] = attachedSponsors
	data["UnattachedSponsors"] = unattachedSponsors

	data["Announcements"] = findRecordsLogged(h.app, "addDetailExtras: find announcements", RecordQuery{
		Collection: "announcements", Filter: "competition = {:cid}", Sort: "-created", Params: map[string]any{"cid": comp.Id},
	})

	data["Activity"] = h.buildActivityTimeline(comp.Id)
}

// buildActivityTimeline maps the competition's activity log (competition_events)
// to the same TimelineEntryVM shape the match thread uses, so the admin
// detail page can reuse the timelineEntry template.
func (h *CompetitionHandler) buildActivityTimeline(compID string) []TimelineEntryVM {
	events := findRecordsLogged(h.app, "buildActivityTimeline: find competition_events", RecordQuery{
		Collection: "competition_events", Filter: "competition = {:cid}", Sort: "-created", Limit: 50, Params: map[string]any{"cid": compID},
	})
	entries := make([]TimelineEntryVM, 0, len(events))
	for _, ev := range events {
		actorName := "Sistema"
		if aid := ev.GetString("actor"); aid != "" {
			actorName = league.PlayerName(h.app, aid)
		}
		created := ev.GetDateTime("created").Time()
		entries = append(entries, TimelineEntryVM{
			Kind:       "event",
			AuthorName: actorName,
			Content:    ev.GetString("detail"),
			CreatedAt:  render.FmtShortTime(created),
			CreatedRel: created.Format(time.RFC3339),
		})
	}
	return entries
}

// buildDetailSponsors returns the sponsors shown as "attached" (the
// competition's own picks plus every global sponsor, which the Branding
// pipeline shows on every competition regardless of attachment) and the
// sponsors still available to attach (non-global, not already attached —
// attaching a global sponsor would be a no-op since it's already shown).
func (h *CompetitionHandler) buildDetailSponsors(comp *core.Record) ([]*core.Record, []*core.Record) {
	attachedIDs := comp.GetStringSlice("sponsors")
	attachedSet := make(map[string]struct{}, len(attachedIDs))
	var attached []*core.Record
	for _, sid := range attachedIDs {
		attachedSet[sid] = struct{}{}
		if s, err := h.app.FindRecordById("sponsors", sid); err == nil {
			attached = append(attached, s)
		}
	}
	allSponsors := findRecordsLogged(h.app, "buildDetailSponsors: find sponsors", RecordQuery{
		Collection: "sponsors", Sort: "name",
	})
	var unattached []*core.Record
	for _, s := range allSponsors {
		if _, ok := attachedSet[s.Id]; ok {
			continue
		}
		if s.GetBool("is_global") {
			attached = append(attached, s)
			continue
		}
		unattached = append(unattached, s)
	}
	return attached, unattached
}

func (h *CompetitionHandler) buildDetailDocs(comp *core.Record, fileToken string) ([]DocumentView, []*core.Record) {
	attachedIDs := comp.GetStringSlice("documents")
	attachMode := Mode{Admin: true, Editable: true, Row: true}
	var views []DocumentView
	for _, did := range attachedIDs {
		if doc, err := h.app.FindRecordById("documents", did); err == nil {
			dv := NewDocumentView(doc, attachMode, fileToken)
			dv.CompetitionID = comp.Id
			views = append(views, dv)
		}
	}
	attachedSet := make(map[string]struct{}, len(attachedIDs))
	for _, did := range attachedIDs {
		attachedSet[did] = struct{}{}
	}
	allDocs := findRecordsLogged(h.app, "buildDetailDocs: find documents", RecordQuery{
		Collection: "documents", Sort: "title",
	})
	var unattached []*core.Record
	for _, d := range allDocs {
		if _, ok := attachedSet[d.Id]; !ok {
			unattached = append(unattached, d)
		}
	}
	return views, unattached
}

// WithdrawPair bulk-finalizes all pre-score matches for a pair and marks it as withdrawn.
func (h *CompetitionHandler) WithdrawPair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	if !slices.Contains(comp.GetStringSlice("pairs"), pairID) {
		return alertError(e, "La pareja no pertenece a esta competición")
	}
	if slices.Contains(comp.GetStringSlice("withdrawn_pairs"), pairID) {
		return alertError(e, "La pareja ya está retirada")
	}

	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return alertError(e, "Pareja no encontrada")
	}
	pairName := pair.GetString("name")

	woScore := comp.GetString("walkover_score")
	if woScore == "" {
		woScore = "6-0 6-0"
	}

	params := map[string]any{"cid": compID, "pid": pairID}
	preScoreMatches, err := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && (status = 'pending' || status = 'scheduled') && (pair1 = {:pid} || pair2 = {:pid})",
		"", 0, 0, params,
	)
	if err != nil {
		return alertError(e, "Error al buscar partidos")
	}
	manualMatches, err := h.app.FindRecordsByFilter("matches",
		"competition = {:cid} && (status = 'confirmed' || status = 'disputed') && (pair1 = {:pid} || pair2 = {:pid})",
		"", 0, 0, params,
	)
	if err != nil {
		return alertError(e, "Error al buscar partidos")
	}

	compName := comp.GetString("name")
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		if err := finalizeMatchesAsWalkovers(txApp, e, preScoreMatches, pairID, woScore, pairName, compName); err != nil {
			return err
		}
		withdrawn := comp.GetStringSlice("withdrawn_pairs")
		withdrawn = append(withdrawn, pairID)
		comp.Set("withdrawn_pairs", withdrawn)
		return txApp.Save(comp)
	}); err != nil {
		return alertError(e, "Error al procesar el retiro")
	}

	// Notify after transaction commits.
	for _, match := range preScoreMatches {
		opponentID := match.GetString("pair2")
		if opponentID == pairID {
			opponentID = match.GetString("pair1")
		}
		opponentPlayers := league.PlayersForPair(h.app, opponentID)
		h.notifier.NotifyPlayers(opponentPlayers, league.NotifOpponentWithdrawn(match.Id, pairName, compName, woScore))
	}
	withdrawnPlayers := league.PlayersForPair(h.app, pairID)
	h.notifier.NotifyPlayers(withdrawnPlayers, league.NotifPairWithdrawn(compName))

	msg := fmt.Sprintf("%s retirada (%d partidos finalizados por incomparecencia", pairName, len(preScoreMatches))
	if len(manualMatches) > 0 {
		msg += fmt.Sprintf(". %d partidos en estado confirmado/disputa requieren atención manual", len(manualMatches))
	}
	msg += ")"
	flash(e, msg)
	return redirectHX(e, "/admin/competitions/"+compID)
}

func finalizeMatchesAsWalkovers(app core.App, e *core.RequestEvent, matches []*core.Record, pairID, woScore, pairName, compName string) error {
	detail := fmt.Sprintf("Incomparecencia — %s se ha retirado de la competición", pairName)
	for _, match := range matches {
		opponentID := match.GetString("pair2")
		if opponentID == pairID {
			opponentID = match.GetString("pair1")
		}
		match.Set("scores", woScore)
		match.Set("winner", opponentID)
		match.Set("status", league.StatusFinal)
		match.Set("review_type", "walkover")
		match.Set("carried_sets", "")
		if err := app.Save(match); err != nil {
			return alertError(e, "Error al finalizar partido")
		}
		addTimelineEntry(app, timelineEntry{
			MatchID: match.Id, ActorID: e.Auth.Id, Kind: "result_event",
			Detail: detail,
		})
	}
	return nil
}

func anyUnpaid(entries []pairEntry) bool {
	for _, pe := range entries {
		if !pe.Paid {
			return true
		}
	}
	return false
}

func countUnpaid(entries []pairEntry) int {
	n := 0
	for _, pe := range entries {
		if !pe.Paid {
			n++
		}
	}
	return n
}
