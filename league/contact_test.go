package league

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserContactInfo_WithPhoneAndEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "Contactable", "contactable@test.local")
	user.Set("phone", "+34612345678")
	require.NoError(t, app.Save(user))

	info := UserContactInfo(user)
	assert.Equal(t, ContactInfo{Phone: "+34612345678", Email: "contactable@test.local"}, info)
}

func TestUserContactInfo_NoPhone(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	user := makeUser(t, app, "NoPhone", "")

	info := UserContactInfo(user)
	assert.Empty(t, info.Phone)
}

func TestWhatsAppURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://wa.me/34612345678", WhatsAppURL("+34612345678"))
}

func TestMailtoURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "mailto:a@b.es", MailtoURL("a@b.es"))
}
