package notify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildNotificationEmail_Basic(t *testing.T) {
	html := BuildNotificationEmail("Juan", "Tu partido ha sido confirmado", "")
	assert.Contains(t, html, "Juan")
	assert.Contains(t, html, "Tu partido ha sido confirmado")
	assert.Contains(t, html, "PadelLeague")
	assert.NotContains(t, html, "Ver partido")
}

func TestBuildNotificationEmail_WithLink(t *testing.T) {
	html := BuildNotificationEmail("Ana", "Resultado enviado", "https://example.com/match/123")
	assert.Contains(t, html, "Ana")
	assert.Contains(t, html, "Ver partido")
	assert.Contains(t, html, "https://example.com/match/123")
}

func TestBuildNotificationEmail_EscapesHTML(t *testing.T) {
	html := BuildNotificationEmail("<script>alert(1)</script>", "Body", "")
	assert.NotContains(t, html, "<script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

func TestEmailNotifyPlayers_NoSMTP(t *testing.T) {
	app := newTestApp(t)
	user := makeUser(t, app, "player")
	EmailNotifyPlayers(app, []string{user.Id}, "Test", "Body", "")
}

func TestEmailNotifyPlayers_InvalidUser(t *testing.T) {
	app := newTestApp(t)
	EmailNotifyPlayers(app, []string{"nonexistent"}, "Test", "Body", "")
}
