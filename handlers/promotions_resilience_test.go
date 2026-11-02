package handlers_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"padelleague/handlers"
	"padelleague/league"
)

const raceClicks = 20

// startClicksDuringSave fires raceClicks click counts from inside the admin
// write (the hook runs while the handler's save is executing), so every click
// overlaps the edit. wait blocks until all of them have finished.
func startClicksDuringSave(app *tests.TestApp, id string) (wait func() []error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	var once sync.Once
	app.OnRecordUpdateExecute("promotions").BindFunc(func(e *core.RecordEvent) error {
		once.Do(func() {
			for range raceClicks {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := league.AddPromotionClick(app, id); err != nil {
						mu.Lock()
						errs = append(errs, err)
						mu.Unlock()
					}
				}()
			}
		})
		return e.Next()
	})
	return func() []error {
		wg.Wait()
		return errs
	}
}

func TestPromotionClicksSurviveConcurrentEdit(t *testing.T) {
	t.Parallel()
	const id = "promorace000001"
	var wait func() []error
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id} while 20 clicks arrive keeps every click",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id,
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Antes", "https://a.example", true, promoFuture)
		promoPost(tb, s, app, promoForm{fields: map[string]string{
			"title": "Después", "description": "d", "url": "https://a.example", "expires_on": "2999-12-31",
		}})
		wait = startClicksDuringSave(app, id)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Empty(tb, wait(), "no click failed")
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.Equal(tb, "Después", r.GetString("title"), "edit applied")
		assert.Equal(tb, raceClicks, r.GetInt("clicks"), "every click kept")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionClicksSurviveConcurrentToggle(t *testing.T) {
	t.Parallel()
	const id = "promorace000002"
	var wait func() []error
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "POST /admin/promotions/{id}/toggle while 20 clicks arrive keeps every click",
		Method:         http.MethodPost,
		URL:            "/admin/promotions/" + id + "/toggle",
		ExpectedStatus: 204,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Oferta", "https://a.example", true, promoFuture)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
		wait = startClicksDuringSave(app, id)
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		assert.Empty(tb, wait(), "no click failed")
		r, err := app.FindRecordById("promotions", id)
		require.NoError(tb, err)
		assert.False(tb, r.GetBool("active"), "toggle applied")
		assert.Equal(tb, raceClicks, r.GetInt("clicks"), "every click kept")
	}
	handlers.ExpectRedirect(s, redirectTo("/admin/promotions"))
	s.Test(t)
}

func TestPromotionAdminDeleteFailure(t *testing.T) {
	t.Parallel()
	const id = "promodelfail001"
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "POST /admin/promotions/{id}/delete when the delete fails",
		Method:             http.MethodPost,
		URL:                "/admin/promotions/" + id + "/delete",
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Error al eliminar la promoción"},
		NotExpectedContent: []string{"Promoción eliminada"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Oferta", "https://a.example", true, "")
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeAdminUserTB(tb, app))
		app.OnRecordDeleteExecute("promotions").BindFunc(func(*core.RecordEvent) error {
			return errors.New("disk full")
		})
	}
	s.AfterTestFunc = func(tb testing.TB, app *tests.TestApp, _ *http.Response) {
		_, err := app.FindRecordById("promotions", id)
		assert.NoError(tb, err, "offer still stored")
	}
	s.Test(t)
}

// failCounters makes every UPDATE of a promotion fail at the database.
func failCounters(tb testing.TB, app core.App) {
	tb.Helper()
	_, err := app.DB().NewQuery(`CREATE TRIGGER promotions_fail BEFORE UPDATE ON promotions
		BEGIN SELECT RAISE(ABORT, 'counter down'); END`).Execute()
	require.NoError(tb, err)
}

// captureLog routes slog to a buffer for the test. Serial tests only: it swaps
// the process-wide default logger.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestPromoCardCounterFailureStillShowsCard(t *testing.T) {
	const id = "promocardfail01"
	logs := captureLog(t)
	s := &tests.ApiScenario{
		TestAppFactory:  handlers.TestAppFactory,
		Name:            "GET /promo/card when the view count fails",
		Method:          http.MethodGet,
		URL:             "/promo/card",
		ExpectedStatus:  200,
		ExpectedContent: []string{"data-promo-card", "Decathlon", `href="/promo/` + id + `"`},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es", true, promoFuture)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
		failCounters(tb, app)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, _ *http.Response) {
		assert.Contains(tb, logs.String(), "Promotions: count view")
		assert.Contains(tb, logs.String(), "counter down")
	}
	s.Test(t)
}

func TestPromoClickCounterFailureStillRedirects(t *testing.T) {
	const id = "promoclickfail1"
	logs := captureLog(t)
	s := &tests.ApiScenario{
		TestAppFactory: handlers.TestAppFactory,
		Name:           "GET /promo/{id} when the click count fails",
		Method:         http.MethodGet,
		URL:            "/promo/" + id,
		ExpectedStatus: 302,
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		setupProductionRoutes(tb, app, e)
		makePromotionTB(tb, app, id, "Decathlon", "https://www.decathlon.es/palas", true, promoFuture)
		s.Headers = handlers.AuthHeaders(tb, handlers.MakeUserTB(tb, app, "Jugador", ""))
		failCounters(tb, app)
	}
	s.AfterTestFunc = func(tb testing.TB, _ *tests.TestApp, res *http.Response) {
		assert.Equal(tb, "https://www.decathlon.es/palas", res.Header.Get("Location"))
		assert.Contains(tb, logs.String(), "Promotions: count click")
		assert.Contains(tb, logs.String(), "counter down")
	}
	s.Test(t)
}

// competitionWithDoc seeds a two-pair league with one attached document and
// opens its page as a player of the first pair.
func competitionWithDoc(tb testing.TB, s *tests.ApiScenario, app *tests.TestApp, e *core.ServeEvent, mandatory bool) {
	tb.Helper()
	setupProductionRoutes(tb, app, e)
	p1 := handlers.MakePairTB(tb, app, "PromoGateA")
	p2 := handlers.MakePairTB(tb, app, "PromoGateB")
	comp := handlers.MakeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
	doc := handlers.MakeDocumentTB(tb, app, "Reglamento", mandatory, "https://example.com/regla")
	comp.Set("documents", []string{doc.Id})
	require.NoError(tb, app.Save(comp))
	s.URL = "/competition/" + comp.Id
	user, err := app.FindRecordById("users", p1.GetString("player1"))
	require.NoError(tb, err)
	s.Headers = handlers.AuthHeaders(tb, user)
}

func TestCompetitionDocsGateShowsNoPromotionCard(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "a player held at the mandatory documents gate gets no promotion slot",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{"Documentos obligatorios"},
		NotExpectedContent: []string{`id="promo-slot"`, "/promo/card"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		competitionWithDoc(tb, s, app, e, true)
	}
	s.Test(t)
}

func TestCompetitionWithoutGateShowsPromotionSlot(t *testing.T) {
	t.Parallel()
	s := &tests.ApiScenario{
		TestAppFactory:     handlers.TestAppFactory,
		Name:               "the same player without a pending mandatory document gets the promotion slot",
		Method:             http.MethodGet,
		ExpectedStatus:     200,
		ExpectedContent:    []string{`id="promo-slot"`, `hx-get="/promo/card"`},
		NotExpectedContent: []string{"Documentos obligatorios"},
	}
	s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		competitionWithDoc(tb, s, app, e, false)
	}
	s.Test(t)
}
