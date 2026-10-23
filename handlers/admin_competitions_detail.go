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
		h.addLeagueExtras(data, comp, matches)
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

func (h *CompetitionHandler) addLeagueExtras(data map[string]any, comp *core.Record, matches []*core.Record) {
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
			Kind:         "event",
			AuthorPlayer: actorName,
			Content:      ev.GetString("detail"),
			CreatedAt:    render.FmtShortTime(created),
			CreatedRel:   created.Format(time.RFC3339),
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

// WithdrawPair marks a pair as withdrawn, then gives every match of the pair the official
// withdrawal result (or releases it, in a leveled league) in one transaction.
func (h *CompetitionHandler) WithdrawPair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	// The guards and the match list are read inside the transaction.
	var out withdrawalOutcome
	var comp *core.Record
	var w pairWithdrawal
	var refusal string
	if err := h.app.RunInTransaction(func(txApp core.App) error {
		var err error
		comp, err = txApp.FindRecordById("competitions", compID)
		if err != nil {
			refusal = "Competición no encontrada"
			return nil
		}
		if refusal = withdrawalRefusal(comp, pairID); refusal != "" {
			return nil
		}
		pair, err := txApp.FindRecordById("pairs", pairID)
		if err != nil {
			refusal = "Pareja no encontrada"
			return nil
		}
		w = pairWithdrawal{adminID: e.Auth.Id, pairID: pairID, pairName: pair.GetString("name"),
			woScore: league.WalkoverScore(comp)}
		matches, err := findPairMatches(txApp, compID, pairID)
		if err != nil {
			return err
		}
		out, err = applyWithdrawal(txApp, comp, matches, w)
		return err
	}); err != nil {
		return alertError(e, "Error al procesar el retiro")
	}
	if refusal != "" {
		return alertError(e, refusal)
	}

	for range out.released {
		league.LogAssignmentReleased(h.app, compID, e.Auth.Id)
	}
	h.notifyWithdrawal(withdrawalNotice{
		compID: compID, pairID: w.pairID,
		pairName: w.pairName, compName: comp.GetString("name"), woScore: w.woScore,
		changed: out.changed, released: out.released,
	})
	flashWithdrawal(e, w.pairName, out)
	return redirectHX(e, "/admin/competitions/"+compID)
}

// pairWithdrawal is the withdrawing pair and the admin and walkover score it is recorded with.
type pairWithdrawal struct {
	adminID  string
	pairID   string
	pairName string
	woScore  string
}

// withdrawalOutcome is what applyWithdrawal did to the matches of the withdrawn pair.
type withdrawalOutcome struct {
	changed, released []*core.Record
}

// withdrawalRefusal is why pairID cannot be withdrawn from comp, or "" when it can.
func withdrawalRefusal(comp *core.Record, pairID string) string {
	switch {
	case comp.GetBool("finalized"):
		return "La competición está finalizada; no se puede retirar una pareja"
	case !slices.Contains(comp.GetStringSlice("pairs"), pairID):
		return "La pareja no pertenece a esta competición"
	case slices.Contains(comp.GetStringSlice("withdrawn_pairs"), pairID):
		return "La pareja ya está retirada"
	}
	return ""
}

// applyWithdrawal gives every match of the withdrawn pair its official result, or releases it when
// the league is leveled and the match has no real score. It runs on txApp, the caller's transaction.
func applyWithdrawal(txApp core.App, comp *core.Record, matches []*core.Record, w pairWithdrawal) (withdrawalOutcome, error) {
	var out withdrawalOutcome
	comp.Set("withdrawn_pairs", append(comp.GetStringSlice("withdrawn_pairs"), w.pairID))
	if err := txApp.Save(comp); err != nil {
		return out, err
	}
	proposed := league.ProvisionalIDs(txApp, matches)
	for _, m := range matches {
		switch league.WithdrawalPlan(comp, m, w.pairID, proposed) {
		case league.WithdrawalRelease:
			if err := releaseMatchTx(txApp, m.Id); err != nil {
				return out, err
			}
			out.released = append(out.released, m)
		case league.WithdrawalOfficial:
			if err := setOfficialResult(txApp, w, m); err != nil {
				return out, err
			}
			out.changed = append(out.changed, m)
		}
	}
	return out, nil
}

// setOfficialResult gives match m the withdrawal walkover for w.pairID. A played score is kept
// on the match as rating_scores and its review type is kept, so the rating and the quota
// still count the match as played.
func setOfficialResult(txApp core.App, w pairWithdrawal, m *core.Record) error {
	opponent := league.OpponentOf(m, w.pairID)
	detail := league.WithdrawalEventText(w.pairName)
	_, _, err := writeAsAdmin(txApp, adminWrite{
		matchID: m.Id, adminID: w.adminID,
		apply: func(app core.App, fresh *core.Record) (adminChange, error) {
			played, err := league.RealScore(app, fresh)
			if err != nil {
				return adminChange{}, err
			}
			reviewType := fresh.GetString("review_type")
			league.ApplyWalkoverResult(fresh, opponent, w.woScore)
			if played != "" {
				fresh.Set("rating_scores", played)
				fresh.Set("review_type", reviewType)
			}
			return adminChange{kind: "result_event", detail: detail,
				decides: []string{proposalResult, proposalScheduling}}, nil
		},
	})
	return err
}

// findPairMatches returns every match of pairID in the competition, whatever its status.
func findPairMatches(app core.App, compID, pairID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("matches",
		"competition = {:cid} && (pair1 = {:pid} || pair2 = {:pid})",
		"created", 0, 0, map[string]any{"cid": compID, "pid": pairID})
}

type withdrawalNotice struct {
	compID   string
	pairID   string
	pairName string
	compName string
	woScore  string
	changed  []*core.Record
	released []*core.Record
}

func (h *CompetitionHandler) notifyWithdrawal(n withdrawalNotice) {
	for _, match := range n.changed {
		h.notifyOpponent(n, match, match.Id)
	}
	for _, match := range n.released {
		h.notifyOpponent(n, match, "")
	}
	withdrawnPlayers := league.PlayersForPair(h.app, n.pairID)
	h.notifier.NotifyPlayers(withdrawnPlayers, league.NotifPairWithdrawn(n.compName))
}

// notifyOpponent tells the opponent of match that the pair withdrew. matchID is empty for a released match.
func (h *CompetitionHandler) notifyOpponent(n withdrawalNotice, match *core.Record, matchID string) {
	opponentID := match.GetString("pair2")
	if opponentID == n.pairID {
		opponentID = match.GetString("pair1")
	}
	h.notifier.NotifyPlayers(league.PlayersForPair(h.app, opponentID), league.NotifOpponentWithdrawn(league.OpponentWithdrawnParams{
		CompID: n.compID, MatchID: matchID, PairName: n.pairName, CompName: n.compName, WoScore: n.woScore,
	}))
}

func flashWithdrawal(e *core.RequestEvent, pairName string, out withdrawalOutcome) {
	msg := fmt.Sprintf("%s retirada (%s por incomparecencia", pairName,
		countPhrase(len(out.changed), "partido registrado", "partidos registrados"))
	if len(out.released) > 0 {
		msg += ", " + countPhrase(len(out.released), "partido sin resultado liberado", "partidos sin resultado liberados")
	}
	msg += ")"
	flash(e, msg)
}

// countPhrase is "1 <singular>" for one and "<n> <plural>" otherwise, zero included.
func countPhrase(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
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

func anyNoBolas(entries []pairEntry) bool {
	for _, pe := range entries {
		if !pe.Balls {
			return true
		}
	}
	return false
}

func countNoBolas(entries []pairEntry) int {
	n := 0
	for _, pe := range entries {
		if !pe.Balls {
			n++
		}
	}
	return n
}
