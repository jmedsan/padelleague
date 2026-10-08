package notify

import (
	"fmt"
	"html"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// Email palette: the shell's brand colors (see RenderEmail) plus neutrals.
const (
	emailAccent = "#b5d334"
	emailInk    = "#0b0b0b"
	emailMuted  = "#666666"
	emailPanel  = "#f7f7f7"
	emailRule   = "#e5e5e5"
)

// matchPair is a pair as the context card shows it: its name and players.
type matchPair struct {
	Name    string
	Players string
}

// Schedule states of a match, as the app labels them: Confirmada /
// Propuesta (the same words and colors as the dateBox badges).
const (
	scheduleConfirmed = "confirmed"
	scheduleProposed  = "proposed"
)

// matchInfo is the match part of the context card. Status is
// scheduleConfirmed when the match has a date and place (league.HasDateAndPlace),
// else scheduleProposed when a scheduling proposal is pending
// (league.PendingSchedulingProposal), else "".
// When and Place hold the confirmed or the proposed values.
type matchInfo struct {
	Pair1, Pair2 matchPair
	Status       string
	When, Place  string
}

// loadMatchInfo reads the match for emails. ok is false when it is not found.
func loadMatchInfo(app core.App, matchID string) (info matchInfo, ok bool) {
	match, err := app.FindRecordById("matches", matchID)
	if err != nil {
		return matchInfo{}, false
	}
	info = matchInfo{
		Pair1: loadMatchPair(app, match.GetString("pair1")),
		Pair2: loadMatchPair(app, match.GetString("pair2")),
	}
	if league.HasDateAndPlace(match) {
		info.Status = scheduleConfirmed
		info.When = whenText(match.GetString("date"), match.GetString("time"))
		info.Place = match.GetString("club")
		if court := match.GetString("court_number"); court != "" {
			info.Place += ", pista " + court
		}
	} else if pd := pendingProposal(app, matchID); pd != nil {
		info.Status = scheduleProposed
		info.When = whenText(pd.Date, pd.Time)
		info.Place = pd.VenueName
		if info.Place == "" {
			info.Place = pd.VenueText
		}
	}
	return info, true
}

func loadMatchPair(app core.App, pairID string) matchPair {
	pair, err := app.FindRecordById("pairs", pairID)
	if err != nil {
		return matchPair{Name: "Pareja desconocida"}
	}
	var names []string
	for _, id := range league.PlayersForPair(app, pairID) {
		names = append(names, league.PlayerName(app, id))
	}
	return matchPair{Name: pair.GetString("name"), Players: strings.Join(names, " / ")}
}

// contextCardHTML renders the single context card of a notification email:
// the competition name as a small uppercase label, then (when m is set) the
// two pairs around a "vs" and the date and place rows. It returns "" when
// there is nothing to show. Tables and inline styles only, for mail clients.
func contextCardHTML(compName string, m *matchInfo) string {
	if compName == "" && m == nil {
		return ""
	}
	var rows []string
	if compName != "" {
		rows = append(rows, fmt.Sprintf(`<tr><td style="padding:14px 16px;">
<div style="font-size:11px;line-height:14px;letter-spacing:1px;text-transform:uppercase;color:%s;">Competición</div>
<div style="font-size:16px;line-height:22px;font-weight:bold;color:%s;">%s</div></td></tr>`,
			emailMuted, emailInk, html.EscapeString(compName)))
	}
	if m != nil {
		rows = append(rows, fmt.Sprintf(`<tr><td style="padding:14px 16px;border-top:1px solid %s;" align="center">
%s
<div style="margin:6px 0;font-size:12px;line-height:16px;font-weight:bold;letter-spacing:1px;text-transform:uppercase;color:%s;">vs</div>
%s</td></tr>`, emailRule, pairBlockHTML(m.Pair1), emailMuted, pairBlockHTML(m.Pair2)))
		if m.Status != "" {
			rows = append(rows, fmt.Sprintf(`<tr><td style="padding:12px 16px;border-top:1px solid %s;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
<tr><td colspan="2" style="padding:0 0 6px;font-size:11px;line-height:20px;letter-spacing:1px;text-transform:uppercase;color:%s;">Fecha y lugar &nbsp;%s</td></tr>
%s%s</table></td></tr>`, emailRule, emailMuted, scheduleBadgeHTML(m.Status), detailRowHTML("Fecha", m.When), detailRowHTML("Lugar", m.Place)))
		}
	}
	return fmt.Sprintf(`<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="margin:16px 0;background:%s;border:1px solid %s;border-radius:8px;">
%s
</table>`, emailPanel, emailRule, strings.Join(rows, "\n"))
}

func pairBlockHTML(p matchPair) string {
	players := ""
	if p.Players != "" {
		players = fmt.Sprintf(`<div style="font-size:13px;line-height:18px;color:%s;">%s</div>`, emailMuted, html.EscapeString(p.Players))
	}
	return fmt.Sprintf(`<div style="font-size:16px;line-height:22px;font-weight:bold;color:%s;">%s</div>%s`,
		emailInk, html.EscapeString(p.Name), players)
}

func detailRowHTML(label, value string) string {
	return fmt.Sprintf(`<tr><td width="64" valign="top" style="padding:3px 0;font-size:13px;line-height:20px;color:%s;">%s</td><td style="padding:3px 0;font-size:15px;line-height:20px;font-weight:bold;color:%s;">%s</td></tr>
`, emailMuted, label, emailInk, html.EscapeString(value))
}

// quoteHTML renders text as a quoted block: accent left border, light background.
func quoteHTML(text string) string {
	return fmt.Sprintf(`<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="margin:8px 0 16px;"><tr><td style="padding:12px 16px;background:%s;border-left:4px solid %s;border-radius:4px;font-size:15px;line-height:22px;color:%s;">%s</td></tr></table>`,
		emailPanel, emailAccent, emailInk, html.EscapeString(text))
}

func whenText(date, clock string) string {
	when := render.FmtDate(date)
	if clock != "" {
		when += " " + clock
	}
	return when
}

// pendingProposal returns the parsed data of the match's newest pending
// scheduling proposal, or nil.
func pendingProposal(app core.App, matchID string) *league.ProposalData {
	msg := league.PendingSchedulingProposal(app, matchID)
	if msg == nil {
		return nil
	}
	pd := league.ParseProposalData(msg.GetString("proposal_data"))
	if pd == nil || pd.Date == "" {
		return nil
	}
	return pd
}

// scheduleBadgeHTML is the status pill: the app's badge-soft-success /
// badge-soft-warning (the light theme's success and warning at 15%).
func scheduleBadgeHTML(status string) string {
	label, bg, fg := "Propuesta", "#f9ebda", "#D97706"
	if status == scheduleConfirmed {
		label, bg, fg = "Confirmada", "#dcf1e4", "#16A34A"
	}
	return fmt.Sprintf(`<span style="display:inline-block;padding:2px 10px;border-radius:999px;background:%s;color:%s;font-size:13px;line-height:20px;font-weight:bold;letter-spacing:0;text-transform:none;white-space:nowrap;">%s</span>`, bg, fg, label)
}
