package league

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ValidPromotionURL reports whether raw is an absolute http or https URL with a host.
func ValidPromotionURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}

// PromotionExpired reports whether the offer's expiry date is before today in loc.
// The date is stored at 12:00 UTC, so its UTC date is the one the admin picked.
func PromotionExpired(rec *core.Record, now time.Time, loc *time.Location) bool {
	expires := rec.GetDateTime("expires_on")
	if expires.IsZero() {
		return false
	}
	return expires.Time().UTC().Format(time.DateOnly) < now.In(loc).Format(time.DateOnly)
}

// PromotionCurrent reports whether the offer is active and not expired.
func PromotionCurrent(rec *core.Record, now time.Time, loc *time.Location) bool {
	return rec.GetBool("active") && !PromotionExpired(rec, now, loc)
}

// CurrentPromotions returns the current offers, newest first.
func CurrentPromotions(app core.App, now time.Time) ([]*core.Record, error) {
	recs, err := app.FindRecordsByFilter("promotions", "active = true", "-created", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("find promotions: %w", err)
	}
	loc := Timezone(app)
	current := recs[:0]
	for _, r := range recs {
		if PromotionCurrent(r, now, loc) {
			current = append(current, r)
		}
	}
	return current, nil
}

// PickPromotion chooses one offer whose ID is not excluded, or nil if none remain.
// intn returns a number in [0, n).
func PickPromotion(recs []*core.Record, excluded map[string]struct{}, intn func(int) int) *core.Record {
	var left []*core.Record
	for _, r := range recs {
		if _, skip := excluded[r.Id]; !skip {
			left = append(left, r)
		}
	}
	if len(left) == 0 {
		return nil
	}
	return left[intn(len(left))]
}

// AddPromotionView adds one view with a single atomic UPDATE.
func AddPromotionView(app core.App, id string) error {
	return bumpPromotion(app, "views", id)
}

// AddPromotionClick adds one click with a single atomic UPDATE.
func AddPromotionClick(app core.App, id string) error {
	return bumpPromotion(app, "clicks", id)
}

// bumpPromotion increments a counter column; column is always a package constant.
func bumpPromotion(app core.App, column, id string) error {
	_, err := app.NonconcurrentDB().
		NewQuery("UPDATE promotions SET " + column + " = " + column + " + 1 WHERE id = {:id}").
		Bind(dbx.Params{"id": id}).
		Execute()
	if err != nil {
		return fmt.Errorf("bump promotion %s: %w", column, err)
	}
	return nil
}
