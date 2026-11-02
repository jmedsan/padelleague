package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
)

const (
	promoPast   = "2000-01-01 12:00:00.000Z"
	promoFuture = "2999-12-31 12:00:00.000Z"
)

// makePromotionTB saves a promotion with a fixed ID so a scenario can name its
// URL up front. expiresOn is a stored date ("" means no expiry).
func makePromotionTB(tb testing.TB, app core.App, id, title, rawURL string, active bool, expiresOn string) {
	tb.Helper()
	col, err := app.FindCollectionByNameOrId("promotions")
	require.NoError(tb, err)
	rec := core.NewRecord(col)
	rec.Id = id
	rec.Set("title", title)
	rec.Set("description", "Descripción de "+title)
	rec.Set("url", rawURL)
	rec.Set("active", active)
	if expiresOn != "" {
		rec.Set("expires_on", expiresOn)
	}
	require.NoError(tb, app.Save(rec))
}

// promoCounters returns the stored views and clicks of a promotion.
func promoCounters(tb testing.TB, app core.App, id string) (views, clicks int) {
	tb.Helper()
	rec, err := app.FindRecordById("promotions", id)
	require.NoError(tb, err)
	return rec.GetInt("views"), rec.GetInt("clicks")
}

func TestPromoClickRedirectsAndCounts(t *testing.T) {
	t.Parallel()
	const id = "promoclick00001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/{id} on a current offer redirects to its URL and counts one click",
		Method:         http.MethodGet,
		URL:            "/promo/" + id,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es/palas?ref=liga", true, promoFuture)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "https://www.decathlon.es/palas?ref=liga", res.Header.Get("Location"))
		views, clicks := promoCounters(tb, app, id)
		assert.Equal(tb, 1, clicks, "clicks")
		assert.Equal(tb, 0, views, "views")
	}
	s.Test(t)
}

func TestPromoClickUnavailable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		id        string
		seed      bool
		rawURL    string
		active    bool
		expiresOn string
		tamperURL string // written straight to the table, as a superuser bypassing validation
	}{
		{name: "unknown id", id: "promomissing001"},
		{name: "inactive", id: "promoinactive01", seed: true, rawURL: "https://example.com/a", active: false, expiresOn: promoFuture},
		{name: "expired", id: "promoexpired001", seed: true, rawURL: "https://example.com/a", active: true, expiresOn: promoPast},
		{name: "tampered javascript URL", id: "promotampered01", seed: true, rawURL: "https://example.com/a", active: true, expiresOn: promoFuture, tamperURL: "javascript:alert(1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory:  handlers.TestAppFactory,
				Name:            "GET /promo/{id}: " + tc.name,
				Method:          http.MethodGet,
				URL:             "/promo/" + tc.id,
				ExpectedStatus:  404,
				ExpectedContent: []string{"Promoción no disponible"},
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				if tc.seed {
					makePromotionTB(tb, app, tc.id, "Oferta", tc.rawURL, tc.active, tc.expiresOn)
					if tc.tamperURL != "" {
						_, err := app.DB().NewQuery("UPDATE promotions SET url = {:url} WHERE id = {:id}").
							Bind(dbx.Params{"url": tc.tamperURL, "id": tc.id}).Execute()
						require.NoError(tb, err)
					}
				}
				s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
			}
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
				if !tc.seed {
					return
				}
				views, clicks := promoCounters(tb, app, tc.id)
				assert.Equal(tb, 0, clicks, "clicks")
				assert.Equal(tb, 0, views, "views")
			}
			s.Test(t)
		})
	}
}

func TestPromoClickAdminViewDoesNotCount(t *testing.T) {
	t.Parallel()
	const id = "promoadmin00001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/{id} as admin redirects without counting",
		Method:         http.MethodGet,
		URL:            "/promo/" + id,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, "")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "https://www.decathlon.es", res.Header.Get("Location"))
		_, clicks := promoCounters(tb, app, id)
		assert.Equal(tb, 0, clicks, "clicks")
	}
	s.Test(t)
}

func TestPromoClickAdminInPlayerViewCounts(t *testing.T) {
	t.Parallel()
	const id = "promoasplayer01"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/{id} as an admin in player view counts the click",
		Method:         http.MethodGet,
		URL:            "/promo/" + id,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, "")
		s.Headers = playerViewHeaders(tb, handlers.MakeAdminUserTB(tb, app))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, clicks := promoCounters(tb, app, id)
		assert.Equal(tb, 1, clicks, "clicks")
	}
	s.Test(t)
}

func TestPromoClickWithoutSession(t *testing.T) {
	t.Parallel()
	const id = "promoanon000001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/{id} without a session goes to login and counts nothing",
		Method:         http.MethodGet,
		URL:            "/promo/" + id,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, "")
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "/login?next=%2Fpromo%2F"+id, res.Header.Get("Location"))
		views, clicks := promoCounters(tb, app, id)
		assert.Equal(tb, 0, clicks, "clicks")
		assert.Equal(tb, 0, views, "views")
	}
	s.Test(t)
}

func TestPromoCardCountsOneView(t *testing.T) {
	t.Parallel()
	const id = "promocard000001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/card renders the chosen offer and counts one view for it only",
		Method:         http.MethodGet,
		URL:            "/promo/card",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"data-promo-card",
			"Patrocinado",
			"Decathlon",
			"Descripción de Decathlon",
			`href="/promo/` + id + `"`,
			`data-promo-link="` + id + `"`,
			`data-promo-dismiss`,
			`aria-label="descartar promoción"`,
		},
		NotExpectedContent: []string{"Apagada", "Vencida", "Descripción de Apagada", "Descripción de Vencida"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, promoFuture)
		makePromotionTB(tb, app, "promooff0000001", "Apagada", "https://example.com/off", false, promoFuture)
		makePromotionTB(tb, app, "promoold0000001", "Vencida", "https://example.com/old", true, promoPast)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		views, clicks := promoCounters(tb, app, id)
		assert.Equal(tb, 1, views, "views of the chosen offer")
		assert.Equal(tb, 0, clicks, "clicks")
		for _, other := range []string{"promooff0000001", "promoold0000001"} {
			v, c := promoCounters(tb, app, other)
			assert.Equal(tb, 0, v, other+" views")
			assert.Equal(tb, 0, c, other+" clicks")
		}
	}
	s.Test(t)
}

func TestPromoCardNoContent(t *testing.T) {
	t.Parallel()
	const id = "promonocontent1"
	cases := []struct {
		name    string
		url     string
		headers func(tb testing.TB, app *tests.TestApp) map[string]string
	}{
		{
			name: "every current offer excluded",
			url:  "/promo/card?exclude=" + id + ",promounknown0001",
			headers: func(tb testing.TB, app *tests.TestApp) map[string]string {
				return handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
			},
		},
		{
			name: "admin view",
			url:  "/promo/card",
			headers: func(tb testing.TB, app *tests.TestApp) map[string]string {
				return handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory: handlers.TestAppFactory,
				Name:           "GET /promo/card: " + tc.name,
				Method:         http.MethodGet,
				URL:            tc.url,
				ExpectedStatus: 204,
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, "")
				s.Headers = tc.headers(tb, app)
			}
			s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				assert.Empty(tb, res.Header.Get("HX-Redirect"), "an empty slot must not redirect")
				views, clicks := promoCounters(tb, app, id)
				assert.Equal(tb, 0, views, "views")
				assert.Equal(tb, 0, clicks, "clicks")
			}
			s.Test(t)
		})
	}
}

func TestPromotionsListShowsCurrentOnly(t *testing.T) {
	t.Parallel()
	const id = "promolist000001"
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promotions lists current offers only and counts no view",
		Method:         http.MethodGet,
		URL:            "/promotions",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			"Promociones",
			"Decathlon",
			`href="/promo/` + id + `"`,
		},
		NotExpectedContent: []string{
			"Apagada",
			"Vencida",
			"No hay promociones disponibles",
			"data-promo-dismiss",
		},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, "")
		makePromotionTB(tb, app, "promooff0000001", "Apagada", "https://example.com/off", false, promoFuture)
		makePromotionTB(tb, app, "promoold0000001", "Vencida", "https://example.com/old", true, promoPast)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		for _, pid := range []string{id, "promooff0000001", "promoold0000001"} {
			v, c := promoCounters(tb, app, pid)
			assert.Equal(tb, 0, v, pid+" views")
			assert.Equal(tb, 0, c, pid+" clicks")
		}
	}
	s.Test(t)
}

func TestPromotionsListEmptyState(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /promotions with no current offer shows the empty state",
		Method:          http.MethodGet,
		URL:             "/promotions",
		ExpectedStatus:  200,
		ExpectedContent: []string{"No hay promociones disponibles"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, "promooff0000001", "Apagada", "https://example.com/off", false, "")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
	}
	s.Test(t)
}

func TestPromotionsNavbarEntries(t *testing.T) {
	t.Parallel()
	const phoneIcon = `aria-label="promociones"`
	const desktopLink = `<li><a href="/promotions"`
	cases := []struct {
		name    string
		offers  bool
		admin   bool
		visible bool
	}{
		{"player, current offer: entries shown", true, false, true},
		{"player, no current offer: entries hidden", false, false, false},
		{"admin view never shows the player entries", true, true, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &tests.ApiScenario{
				TestAppFactory: handlers.TestAppFactory,
				Name:           tt.name,
				Method:         http.MethodGet,
				URL:            "/promotions",
				ExpectedStatus: 200,
			}
			if tt.visible {
				s.ExpectedContent = []string{phoneIcon, desktopLink}
			} else {
				s.NotExpectedContent = []string{phoneIcon, desktopLink}
			}
			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupProductionRoutes(tb, app, e)
				makePromotionTB(tb, app, "promooff0000001", "Apagada", "https://example.com/off", false, "")
				if tt.offers {
					makePromotionTB(tb, app, "promolive000001", "Decathlon", "https://www.decathlon.es", true, "")
				}
				if tt.admin {
					s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
					return
				}
				s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
			}
			s.Test(t)
		})
	}
}
