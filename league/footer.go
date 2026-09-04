package league

import "github.com/pocketbase/pocketbase/core"

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

// FooterContext resolves footer data. When compID is non-empty, the footer
// shows that competition's identity and sponsors. When empty, it loads the
// active competitions userID participates in (all active ones for an admin
// or an anonymous/system caller with userID == ""); if exactly one matches,
// it promotes to the in-context shape (full logo + sponsors).
func FooterContext(app core.App, compID, userID string, isAdmin bool) FooterData {
	if compID != "" {
		return footerForComp(app, compID)
	}
	active, _ := app.FindRecordsByFilter("competitions", "active = true", "name", 0, 0, nil)
	if userID != "" && !isAdmin {
		active = filterCompetitionsForPlayer(app, active, userID)
	}
	if len(active) == 1 {
		return footerForComp(app, active[0].Id)
	}
	var fd FooterData
	seen := make(map[string]struct{})
	for _, c := range active {
		fd.Active = append(fd.Active, FooterCompIdent{
			ID:      c.Id,
			Name:    c.GetString("name"),
			LogoURL: CompetitionLogoURL(c.Id, c.GetString("logo")),
		})
		for _, sid := range c.GetStringSlice("sponsors") {
			if _, ok := seen[sid]; ok {
				continue
			}
			seen[sid] = struct{}{}
			s, err := app.FindRecordById("sponsors", sid)
			if err != nil {
				continue
			}
			fd.Sponsors = append(fd.Sponsors, FooterSponsor{
				Name:    s.GetString("name"),
				LogoURL: SponsorLogoURL(s.Id, s.GetString("logo")),
				URL:     s.GetString("url"),
			})
		}
	}
	return fd
}

func filterCompetitionsForPlayer(app core.App, comps []*core.Record, userID string) []*core.Record {
	pairs, _ := PairsForPlayer(app, userID)
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
	comp, err := app.FindRecordById("competitions", compID)
	if err != nil {
		return FooterData{}
	}
	fd := FooterData{
		Competition: &FooterCompIdent{
			ID:      comp.Id,
			Name:    comp.GetString("name"),
			LogoURL: CompetitionLogoURL(comp.Id, comp.GetString("logo")),
		},
	}
	for _, sid := range comp.GetStringSlice("sponsors") {
		s, err := app.FindRecordById("sponsors", sid)
		if err != nil {
			continue
		}
		fd.Sponsors = append(fd.Sponsors, FooterSponsor{
			Name:    s.GetString("name"),
			LogoURL: SponsorLogoURL(s.Id, s.GetString("logo")),
			URL:     s.GetString("url"),
		})
	}
	return fd
}
