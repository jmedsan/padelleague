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
// shows that competition's identity and sponsors. When empty, it loads all
// active competitions; if exactly one is active, it promotes to the
// in-context shape (full logo + sponsors).
func FooterContext(app core.App, compID string) FooterData {
	if compID != "" {
		return footerForComp(app, compID)
	}
	active, _ := app.FindRecordsByFilter("competitions", "active = true", "name", 0, 0, nil)
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
