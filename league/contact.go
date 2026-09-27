package league

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

const (
	whatsAppURLFormat = "https://wa.me/%s"
	mailtoURLFormat   = "mailto:%s"
)

// ContactInfo is a set of contact details (the league admin's or a
// player's), each empty when unset. Phone is E.164.
type ContactInfo struct {
	Phone string
	Email string
}

// LoadContactInfo reads the league admin's contact details from the
// app_settings singleton.
func LoadContactInfo(app core.App) ContactInfo {
	settings := leagueSettingsRecord(app)
	if settings == nil {
		return ContactInfo{}
	}
	return ContactInfo{Phone: settings.GetString("contact_whatsapp"), Email: settings.GetString("contact_email")}
}

// UserContactInfo returns a user's own contact details.
func UserContactInfo(user *core.Record) ContactInfo {
	return ContactInfo{Phone: user.GetString("phone"), Email: user.GetString("email")}
}

// WhatsAppURL returns a wa.me chat link for an E.164 number.
func WhatsAppURL(e164 string) string {
	return fmt.Sprintf(whatsAppURLFormat, strings.TrimPrefix(e164, "+"))
}

// WhatsAppURLWithText returns a wa.me chat link with a prefilled message.
func WhatsAppURLWithText(e164, msg string) string {
	return WhatsAppURL(e164) + "?text=" + url.QueryEscape(msg)
}

// MailtoURL returns a mailto: link for an email address.
func MailtoURL(email string) string {
	return fmt.Sprintf(mailtoURLFormat, email)
}
