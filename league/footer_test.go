package league

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFooterContext_ExplicitCompID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "FooterA")
	p2 := makePair(t, app, "FooterB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	sponsor := makeSponsor(t, app, "Decathlon", "https://www.decathlon.es")
	comp.Set("sponsors", []string{sponsor.Id})
	require.NoError(t, app.Save(comp))

	fd := FooterContext(app, comp.Id, "", false)

	require.NotNil(t, fd.Competition)
	assert.Equal(t, comp.Id, fd.Competition.ID)
	assert.Equal(t, "Test Competition", fd.Competition.Name)
	require.Len(t, fd.Sponsors, 1)
	assert.Equal(t, "Decathlon", fd.Sponsors[0].Name)
	assert.Equal(t, "https://www.decathlon.es", fd.Sponsors[0].URL)
	assert.Equal(t, SponsorLogoURL(sponsor.Id, sponsor.GetString("logo")), fd.Sponsors[0].LogoURL)
	assert.Nil(t, fd.Active, "in-context footer must not populate the Active list")
}

func TestFooterContext_NoCompID_SingleActive_PromotesToInContext(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "FooterC")
	p2 := makePair(t, app, "FooterD")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	sponsor := makeSponsor(t, app, "Wurko", "https://www.wurko.es")
	comp.Set("sponsors", []string{sponsor.Id})
	require.NoError(t, app.Save(comp))

	fd := FooterContext(app, "", "", false)

	require.NotNil(t, fd.Competition, "single active competition must be promoted to the in-context shape")
	assert.Equal(t, comp.Id, fd.Competition.ID)
	require.Len(t, fd.Sponsors, 1)
	assert.Equal(t, "Wurko", fd.Sponsors[0].Name)
	assert.Nil(t, fd.Active)
}

func TestFooterContext_NoCompID_MultipleActive_ReturnsActiveList(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "FooterE")
	p2 := makePair(t, app, "FooterF")
	p3 := makePair(t, app, "FooterG")
	p4 := makePair(t, app, "FooterH")
	comp1 := makeCompetition(t, app, []*core.Record{p1, p2})
	comp2 := makeCompetition(t, app, []*core.Record{p3, p4})

	fd := FooterContext(app, "", "", false)

	assert.Nil(t, fd.Competition, "multiple active competitions must not resolve a single in-context identity")
	assert.Nil(t, fd.Sponsors)
	require.Len(t, fd.Active, 2)
	ids := []string{fd.Active[0].ID, fd.Active[1].ID}
	assert.ElementsMatch(t, []string{comp1.Id, comp2.Id}, ids)
}

func TestFooterContext_NoCompID_ZeroActive_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "FooterI")
	p2 := makePair(t, app, "FooterJ")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	comp.Set("active", false)
	require.NoError(t, app.Save(comp))

	fd := FooterContext(app, "", "", false)

	assert.Equal(t, FooterData{}, fd)
}

func TestBranding_OutOfContext_UsesLeagueDefaultsAndGlobalSponsorsOnly(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	global := makeSponsor(t, app, "Decathlon", "https://www.decathlon.es")
	global.Set("is_global", true)
	require.NoError(t, app.Save(global))
	makeSponsor(t, app, "NonGlobal", "https://www.example.com")

	bd := Branding(app, "")

	assert.Equal(t, "Liga Dale Fuerte", bd.Name)
	assert.Equal(t, "A La Pelota", bd.Tagline)
	assert.Nil(t, bd.Competition)
	require.Len(t, bd.Sponsors, 1)
	assert.Equal(t, "Decathlon", bd.Sponsors[0].Name)
}

func TestBranding_InContext_CompetitionLogoTakesPriorityOverLeagueLogo(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "BrandA")
	p2 := makePair(t, app, "BrandB")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	f, err := filesystem.NewFileFromBytes([]byte("fake-png-bytes"), "complogo.png")
	require.NoError(t, err)
	comp.Set("logo", f)
	require.NoError(t, app.Save(comp))

	bd := Branding(app, comp.Id)

	require.NotNil(t, bd.Competition)
	assert.Equal(t, CompetitionLogoURL(comp.Id, comp.GetString("logo")), bd.Competition.LogoURL)
}

func TestBranding_InContext_FallsBackToLeagueLogoWhenCompetitionHasNone(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "BrandC")
	p2 := makePair(t, app, "BrandD")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})

	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	require.NoError(t, err)
	require.Len(t, records, 1)
	settings := records[0]
	f, err := filesystem.NewFileFromBytes([]byte("fake-png-bytes"), "leaguelogo.png")
	require.NoError(t, err)
	settings.Set("league_logo", f)
	require.NoError(t, app.Save(settings))

	bd := Branding(app, comp.Id)

	require.NotNil(t, bd.Competition)
	assert.Equal(t, SettingsLogoURL(settings.Id, settings.GetString("league_logo")), bd.Competition.LogoURL)
}

func TestBranding_InContext_MergesCompetitionAndGlobalSponsorsDedupedByID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	p1 := makePair(t, app, "BrandE")
	p2 := makePair(t, app, "BrandF")
	comp := makeCompetition(t, app, []*core.Record{p1, p2})
	global := makeSponsor(t, app, "Decathlon", "https://www.decathlon.es")
	global.Set("is_global", true)
	require.NoError(t, app.Save(global))
	own := makeSponsor(t, app, "Wurko", "https://www.wurko.es")
	comp.Set("sponsors", []string{global.Id, own.Id})
	require.NoError(t, app.Save(comp))

	bd := Branding(app, comp.Id)

	require.Len(t, bd.Sponsors, 2)
	names := []string{bd.Sponsors[0].Name, bd.Sponsors[1].Name}
	assert.ElementsMatch(t, []string{"Wurko", "Decathlon"}, names)
}
