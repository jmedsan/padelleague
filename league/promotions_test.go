package league

import (
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePromotion(t *testing.T, app core.App, title string, active bool, expiresOn string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("promotions")
	require.NoError(t, err)
	rec := core.NewRecord(col)
	rec.Set("title", title)
	rec.Set("url", "https://shop.example/"+title)
	rec.Set("active", active)
	if expiresOn != "" {
		rec.Set("expires_on", expiresOn+" 12:00:00.000Z")
	}
	require.NoError(t, app.Save(rec))
	return rec
}

func TestValidPromotionURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://a.example", true},
		{"http://a.example/x?y=1", true},
		{"HTTPS://a.example", true},
		{"javascript:alert(1)", false},
		{"ftp://a.example", false},
		{"//a.example", false},
		{"https://", false},
		{"", false},
		{"a.example", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ValidPromotionURL(tt.raw), "ValidPromotionURL(%q)", tt.raw)
	}
}

func TestPromotionExpired(t *testing.T) {
	t.Parallel()
	madrid, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	col := core.NewBaseCollection("promotions")
	col.Fields.Add(&core.DateField{Name: "expires_on"}, &core.BoolField{Name: "active"})
	mk := func(expiresOn string, active bool) *core.Record {
		rec := core.NewRecord(col)
		rec.Set("active", active)
		if expiresOn != "" {
			rec.Set("expires_on", expiresOn+" 12:00:00.000Z")
		}
		return rec
	}
	noon := time.Date(2026, 11, 2, 12, 0, 0, 0, madrid)
	lateUTC := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC) // 00:30 on 3 Nov in Madrid

	tests := []struct {
		name        string
		expiresOn   string
		now         time.Time
		wantExpired bool
	}{
		{"no expiry", "", noon, false},
		{"expires today", "2026-11-02", noon, false},
		{"expired yesterday", "2026-11-01", noon, true},
		{"expires tomorrow", "2026-11-03", noon, false},
		{"23:30 UTC on expiry date is next day in Madrid", "2026-11-02", lateUTC, true},
		{"23:30 UTC the day before expiry", "2026-11-03", lateUTC, false},
	}
	for _, tt := range tests {
		rec := mk(tt.expiresOn, true)
		assert.Equal(t, tt.wantExpired, PromotionExpired(rec, tt.now, madrid), tt.name)
		assert.Equal(t, !tt.wantExpired, PromotionCurrent(rec, tt.now, madrid), "current: "+tt.name)
	}
	assert.False(t, PromotionCurrent(mk("", false), noon, madrid), "inactive is never current")
}

func TestCurrentPromotions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	loc := Timezone(app)
	now := time.Now()
	today := now.In(loc).Format("2006-01-02")
	yesterday := now.In(loc).AddDate(0, 0, -1).Format("2006-01-02")

	first := makePromotion(t, app, "first", true, "")
	makePromotion(t, app, "off", false, "")
	makePromotion(t, app, "old", true, yesterday)
	second := makePromotion(t, app, "second", true, today)

	got, err := CurrentPromotions(app, now)
	require.NoError(t, err)
	var ids []string
	for _, r := range got {
		ids = append(ids, r.Id)
	}
	assert.ElementsMatch(t, []string{first.Id, second.Id}, ids)
	assert.Equal(t, []string{second.Id, first.Id}, ids, "newest first")
}

func TestPickPromotion(t *testing.T) {
	t.Parallel()
	col := core.NewBaseCollection("promotions")
	mk := func(id string) *core.Record {
		rec := core.NewRecord(col)
		rec.Id = id
		return rec
	}
	recs := []*core.Record{mk("a"), mk("b"), mk("c")}
	at := func(i int) func(int) int { return func(n int) int { assert.Equal(t, 3, n); return i } }

	for i, want := range []string{"a", "b", "c"} {
		assert.Equal(t, want, PickPromotion(recs, nil, at(i)).Id)
	}

	ex := map[string]struct{}{"b": {}}
	pick := PickPromotion(recs, ex, func(n int) int { assert.Equal(t, 2, n); return 1 })
	assert.Equal(t, "c", pick.Id, "intn indexes the remaining candidates")

	unknown := map[string]struct{}{"zzz": {}}
	assert.Equal(t, "a", PickPromotion(recs, unknown, at(0)).Id, "unknown excluded ID is ignored")

	all := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	assert.Nil(t, PickPromotion(recs, all, func(int) int { t.Fatal("intn must not run"); return 0 }))
	assert.Nil(t, PickPromotion(nil, nil, func(int) int { t.Fatal("intn must not run"); return 0 }))
}

func TestPromotionCounters(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	rec := makePromotion(t, app, "counted", true, "")

	require.NoError(t, AddPromotionView(app, rec.Id))
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- AddPromotionClick(app, rec.Id)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	got, err := app.FindRecordById("promotions", rec.Id)
	require.NoError(t, err)
	assert.Equal(t, 20, got.GetInt("clicks"))
	assert.Equal(t, 1, got.GetInt("views"))
}

func TestPromotionImageURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/logo/promotion/p1", PromotionImageURL("p1", "x.jpg"))
	assert.Equal(t, "", PromotionImageURL("p1", ""))
}

func TestHasCurrentPromotions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	loc := Timezone(app)
	now := time.Now()
	yesterday := now.In(loc).AddDate(0, 0, -1).Format("2006-01-02")

	assert.False(t, HasCurrentPromotions(app, now), "no offers")
	makePromotion(t, app, "off", false, "")
	makePromotion(t, app, "old", true, yesterday)
	assert.False(t, HasCurrentPromotions(app, now), "inactive and expired offers do not count")
	makePromotion(t, app, "live", true, "")
	assert.True(t, HasCurrentPromotions(app, now))
}
