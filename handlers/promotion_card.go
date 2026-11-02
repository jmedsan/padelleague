package handlers

import (
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

// PromotionView is the data the promotionCard component renders. Every surface
// that shows an offer builds it with NewPromotionView.
type PromotionView struct {
	Mode                                                          Mode
	ID, Title, Description, ImageURL, ClickURL, URL, ExpiresLabel string
	Active, Expired                                               bool
	Views, Clicks                                                 int
}

// NewPromotionView builds the view of one offer. ClickURL is the counted
// redirect players follow; URL is the stored link.
func NewPromotionView(rec *core.Record, mode Mode, now time.Time, loc *time.Location) PromotionView {
	v := PromotionView{
		Mode:        mode,
		ID:          rec.Id,
		Title:       rec.GetString("title"),
		Description: rec.GetString("description"),
		ImageURL:    league.PromotionImageURL(rec.Id, rec.GetString("image")),
		ClickURL:    "/promo/" + rec.Id,
		URL:         rec.GetString("url"),
		Active:      rec.GetBool("active"),
		Expired:     league.PromotionExpired(rec, now, loc),
		Views:       rec.GetInt("views"),
		Clicks:      rec.GetInt("clicks"),
	}
	if exp := rec.GetDateTime("expires_on"); !exp.IsZero() {
		v.ExpiresLabel = "Hasta el " + exp.Time().UTC().Format("02/01/2006")
	}
	return v
}

// ViewsLabel is the view counter with its Spanish singular or plural noun.
func (v PromotionView) ViewsLabel() string { return countPhrase(v.Views, "vista", "vistas") }

// ClicksLabel is the click counter with its Spanish singular or plural noun.
func (v PromotionView) ClicksLabel() string { return countPhrase(v.Clicks, "clic", "clics") }
