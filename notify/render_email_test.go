package notify

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAppURL = "https://example.com"

func setAppURL(t *testing.T, app core.App) {
	t.Helper()
	app.Settings().Meta.AppURL = testAppURL
	require.NoError(t, app.Save(app.Settings()))
}

func leagueSettings(t *testing.T, app core.App) *core.Record {
	t.Helper()
	records, err := app.FindRecordsByFilter("app_settings", "", "", 1, 0, nil)
	require.NoError(t, err)
	require.Len(t, records, 1)
	return records[0]
}

func TestRenderEmail_UsesLeagueNameAndTagline(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)

	got := RenderEmail(app, "", "<p>body</p>")

	assert.Contains(t, got, "Liga Dale Fuerte")
	assert.Contains(t, got, "A La Pelota")
	assert.Contains(t, got, "<p>body</p>")
	assert.Contains(t, got, testAppURL)
}

func TestRenderEmail_LogoURLIsAbsolute(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)
	settings := leagueSettings(t, app)
	f, err := filesystem.NewFileFromBytes([]byte("fake-png-bytes"), "leaguelogo.png")
	require.NoError(t, err)
	settings.Set("league_logo", f)
	require.NoError(t, app.Save(settings))

	got := RenderEmail(app, "", "<p>body</p>")

	assert.Contains(t, got, testAppURL+"/api/files/app_settings/"+settings.Id+"/")
}

func TestRenderEmail_NoLogo_OmitsImgTag(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)

	got := RenderEmail(app, "", "<p>body</p>")

	assert.NotContains(t, got, "<img")
}

func TestRenderEmail_GlobalSponsor_ShownWithAbsoluteLogoAndSingularHeading(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)
	col, err := app.FindCollectionByNameOrId("sponsors")
	require.NoError(t, err)
	sponsor := core.NewRecord(col)
	sponsor.Set("name", "Decathlon")
	sponsor.Set("url", "https://decathlon.es")
	sponsor.Set("is_global", true)
	f, err := filesystem.NewFileFromBytes([]byte("fake-png-bytes"), "sponsorlogo.png")
	require.NoError(t, err)
	sponsor.Set("logo", f)
	require.NoError(t, app.Save(sponsor))

	got := RenderEmail(app, "", "<p>body</p>")

	assert.Contains(t, got, "Patrocinado por")
	assert.NotContains(t, got, "Patrocinadores")
	assert.Contains(t, got, testAppURL+"/api/files/sponsors/"+sponsor.Id+"/")
}

func TestRenderEmail_MultipleSponsors_UsesPluralHeading(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)
	col, err := app.FindCollectionByNameOrId("sponsors")
	require.NoError(t, err)
	for _, name := range []string{"Decathlon", "Wurko"} {
		sponsor := core.NewRecord(col)
		sponsor.Set("name", name)
		sponsor.Set("is_global", true)
		f, err := filesystem.NewFileFromBytes([]byte("fake-png-bytes"), name+"logo.png")
		require.NoError(t, err)
		sponsor.Set("logo", f)
		require.NoError(t, app.Save(sponsor))
	}

	got := RenderEmail(app, "", "<p>body</p>")

	assert.Contains(t, got, "Patrocinadores")
}

func TestRenderEmail_NoSponsors_OmitsSponsorSection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)

	got := RenderEmail(app, "", "<p>body</p>")

	assert.NotContains(t, got, "Patrocinado")
}

func TestRenderEmail_EscapesLeagueName(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setAppURL(t, app)
	settings := leagueSettings(t, app)
	settings.Set("league_name", "<script>alert(1)</script>")
	require.NoError(t, app.Save(settings))

	got := RenderEmail(app, "", "<p>body</p>")

	assert.NotContains(t, got, "<script>alert(1)</script>")
	assert.Contains(t, got, "&lt;script&gt;")
}
