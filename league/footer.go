package league

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// FooterCompIdent identifies a competition in the footer.
type FooterCompIdent struct {
	ID      string
	Name    string
	LogoURL string
}

// FooterSponsor represents a sponsor tile in the footer.
type FooterSponsor struct {
	Name    string
	LogoURL string
	URL     string
}

// FooterData holds everything the site footer template needs.
type FooterData struct {
	Competition *FooterCompIdent
	Sponsors    []FooterSponsor
	Active      []FooterCompIdent
}

// BrandingData holds the league's identity and sponsor list for a given
// context. Out of competition context, Name/Tagline/LogoURL reflect the
// league-wide defaults and Sponsors holds only global sponsors. In
// competition context, LogoURL falls back to the league logo when the
// competition has none, and Sponsors merges the competition's own sponsors
// with the global ones, deduplicated by id.
type BrandingData struct {
	Name        string
	Tagline     string
	LogoURL     string
	Sponsors    []FooterSponsor
	Competition *FooterCompIdent
}

// Branding resolves the league's branding for the given context. When compID
// is non-empty, it reflects that competition's identity, falling back to the
// league logo when the competition has none, plus its sponsors merged with
// the global ones. When compID is empty, it reflects the league-wide
// defaults and global sponsors only.
func Branding(app core.App, compID string) BrandingData {
	settings := leagueSettingsRecord(app)
	bd := BrandingData{
		Name:    settingsString(settings, "league_name", "Liga Dale Fuerte"),
		Tagline: settingsString(settings, "league_tagline", "A La Pelota"),
	}
	if settings != nil {
		bd.LogoURL = SettingsLogoURL(settings.Id, settings.GetString("league_logo"))
	}

	globalSponsors, globalIDs := globalSponsors(app)
	if compID == "" {
		bd.Sponsors = globalSponsors
		return bd
	}

	comp, err := app.FindRecordById("competitions", compID)
	if err != nil {
		bd.Sponsors = globalSponsors
		return bd
	}
	bd.Competition = &FooterCompIdent{
		ID:      comp.Id,
		Name:    comp.GetString("name"),
		LogoURL: CompetitionLogoURL(comp.Id, comp.GetString("logo")),
	}
	if bd.Competition.LogoURL == "" {
		bd.Competition.LogoURL = bd.LogoURL
	}

	bd.Sponsors = append(bd.Sponsors, compSponsors(app, comp, globalIDs)...)
	bd.Sponsors = append(bd.Sponsors, globalSponsors...)
	return bd
}

// leagueSettingsRecord returns the app_settings singleton, or nil if none exists.
func leagueSettingsRecord(app core.App) *core.Record {
	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return nil
	}
	return records[0]
}

func settingsString(rec *core.Record, field, fallback string) string {
	if rec == nil {
		return fallback
	}
	if v := rec.GetString(field); v != "" {
		return v
	}
	return fallback
}

// globalSponsors returns every sponsor flagged is_global, plus the set of
// their ids for deduplication against a competition's own sponsor list.
func globalSponsors(app core.App) ([]FooterSponsor, map[string]struct{}) {
	recs, err := app.FindRecordsByFilter("sponsors", "is_global = true", "name", 0, 0, nil)
	if err != nil {
		slog.Warn("branding: load global sponsors", "err", err)
		return nil, nil
	}
	ids := make(map[string]struct{}, len(recs))
	sponsors := make([]FooterSponsor, 0, len(recs))
	for _, s := range recs {
		ids[s.Id] = struct{}{}
		sponsors = append(sponsors, FooterSponsor{
			Name:    s.GetString("name"),
			LogoURL: SponsorLogoURL(s.Id, s.GetString("logo")),
			URL:     s.GetString("url"),
		})
	}
	return sponsors, ids
}

// compSponsors returns comp's own sponsors, excluding any already present in
// globalIDs so a sponsor flagged is_global never appears twice.
func compSponsors(app core.App, comp *core.Record, globalIDs map[string]struct{}) []FooterSponsor {
	var sponsors []FooterSponsor
	for _, sid := range comp.GetStringSlice("sponsors") {
		if _, ok := globalIDs[sid]; ok {
			continue
		}
		s, err := app.FindRecordById("sponsors", sid)
		if err != nil {
			continue
		}
		sponsors = append(sponsors, FooterSponsor{
			Name:    s.GetString("name"),
			LogoURL: SponsorLogoURL(s.Id, s.GetString("logo")),
			URL:     s.GetString("url"),
		})
	}
	return sponsors
}

// FooterContext resolves footer data. When compID is non-empty, the footer
// shows that competition's identity and sponsors. When empty, it loads the
// active competitions userID participates in (all active ones for an admin
// or an anonymous/system caller with userID == ""); if exactly one matches,
// it promotes to the in-context shape (full logo + sponsors).
func FooterContext(app core.App, compID, userID string, isAdmin bool) FooterData {
	if compID != "" {
		return footerForComp(app, compID)
	}
	active, err := app.FindRecordsByFilter("competitions", "active = true", "name", 0, 0, nil)
	if err != nil {
		slog.Warn("footer: load active competitions", "err", err)
		return FooterData{}
	}
	if userID != "" && !isAdmin {
		active = filterCompetitionsForPlayer(app, active, userID)
	}
	if len(active) == 1 {
		return footerForComp(app, active[0].Id)
	}
	var fd FooterData
	for _, c := range active {
		fd.Active = append(fd.Active, FooterCompIdent{
			ID:      c.Id,
			Name:    c.GetString("name"),
			LogoURL: CompetitionLogoURL(c.Id, c.GetString("logo")),
		})
	}
	return fd
}

func filterCompetitionsForPlayer(app core.App, comps []*core.Record, userID string) []*core.Record {
	pairs, err := PairsForPlayer(app, userID)
	if err != nil {
		slog.Warn("footer: load pairs for player", "user", userID, "err", err)
		return nil
	}
	pairIDs := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		pairIDs[p.Id] = struct{}{}
	}
	var filtered []*core.Record
	for _, c := range comps {
		for _, pid := range c.GetStringSlice("pairs") {
			if _, ok := pairIDs[pid]; ok {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered
}

func footerForComp(app core.App, compID string) FooterData {
	bd := Branding(app, compID)
	if bd.Competition == nil {
		return FooterData{}
	}
	return FooterData{
		Competition: bd.Competition,
		Sponsors:    bd.Sponsors,
	}
}
