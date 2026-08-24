package notify

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pushServer stands in for the browser push service. It counts the requests
// webpush actually delivers and replies with the given status.
func pushServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func vapidKeys(t *testing.T) (private, public string) {
	t.Helper()
	private, public, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	return private, public
}

func makeSubscription(t *testing.T, app core.App, userID, endpoint string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("push_subscriptions")
	require.NoError(t, err)
	sub := core.NewRecord(col)
	sub.Set("user", userID)
	sub.Set("endpoint", endpoint)
	// Valid-shaped keys so webpush can encrypt the payload.
	sub.Set("p256dh", "BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM")
	sub.Set("auth", "tBHItJI5svbpez7KI4CCXg")
	require.NoError(t, app.Save(sub))
	return sub
}

func TestNewNotifier_HTTPClientTimeout(t *testing.T) {
	n := NewNotifier(nil, "pub", "priv")
	assert.Equal(t, 10*time.Second, n.httpClient.Timeout,
		"a missing timeout would let a hung push service block the sender")
}

func TestSendPush_NoVAPIDKeysSendsNothing(t *testing.T) {
	app := newTestApp(t)
	srv, hits := pushServer(t, http.StatusCreated)
	user := makeUser(t, app, "player")
	makeSubscription(t, app, user.Id, srv.URL)

	for name, keys := range map[string][2]string{
		"both missing":    {"", ""},
		"public missing":  {"", "priv"},
		"private missing": {"pub", ""},
	} {
		t.Run(name, func(t *testing.T) {
			NewNotifier(app, keys[0], keys[1]).sendPush(user.Id, "T", "B", "")
			assert.Equal(t, int32(0), hits.Load())
		})
	}
}

func TestSendPush_NoSubscriptionsSendsNothing(t *testing.T) {
	app := newTestApp(t)
	srv, hits := pushServer(t, http.StatusCreated)
	priv, pub := vapidKeys(t)
	user := makeUser(t, app, "player")
	// Subscription belongs to a different user.
	other := makeUser(t, app, "player")
	makeSubscription(t, app, other.Id, srv.URL)

	NewNotifier(app, pub, priv).sendPush(user.Id, "T", "B", "")

	assert.Equal(t, int32(0), hits.Load())
}

func TestSendPush_DeliversToSubscription(t *testing.T) {
	app := newTestApp(t)
	srv, hits := pushServer(t, http.StatusCreated)
	priv, pub := vapidKeys(t)
	user := makeUser(t, app, "player")
	sub := makeSubscription(t, app, user.Id, srv.URL)

	NewNotifier(app, pub, priv).sendPush(user.Id, "Titulo", "Cuerpo", "match123")

	require.Equal(t, int32(1), hits.Load())
	_, err := app.FindRecordById("push_subscriptions", sub.Id)
	assert.NoError(t, err, "a delivered push must not remove the subscription")
}

// A push service reports a dead subscription with 410 Gone or 404 Not Found;
// both must prune the record so we stop sending to it.
func TestSendPush_PrunesDeadSubscriptions(t *testing.T) {
	for name, status := range map[string]int{
		"410 Gone":      http.StatusGone,
		"404 Not Found": http.StatusNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			app := newTestApp(t)
			srv, hits := pushServer(t, status)
			priv, pub := vapidKeys(t)
			user := makeUser(t, app, "player")
			sub := makeSubscription(t, app, user.Id, srv.URL)

			NewNotifier(app, pub, priv).sendPush(user.Id, "T", "B", "")

			require.Equal(t, int32(1), hits.Load())
			_, err := app.FindRecordById("push_subscriptions", sub.Id)
			assert.Error(t, err, "dead subscription must be deleted")
		})
	}
}

func TestSendPush_KeepsSubscriptionOnOtherErrors(t *testing.T) {
	app := newTestApp(t)
	srv, hits := pushServer(t, http.StatusInternalServerError)
	priv, pub := vapidKeys(t)
	user := makeUser(t, app, "player")
	sub := makeSubscription(t, app, user.Id, srv.URL)

	NewNotifier(app, pub, priv).sendPush(user.Id, "T", "B", "")

	require.Equal(t, int32(1), hits.Load())
	_, err := app.FindRecordById("push_subscriptions", sub.Id)
	assert.NoError(t, err, "a transient 500 must not drop the subscription")
}

func TestSendPush_UsesConfiguredSenderAsSubscriber(t *testing.T) {
	app := newTestApp(t)
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	priv, pub := vapidKeys(t)
	user := makeUser(t, app, "player")
	makeSubscription(t, app, user.Id, srv.URL)

	app.Settings().Meta.SenderAddress = "liga@padel.test"
	require.NoError(t, app.Save(app.Settings()))

	NewNotifier(app, pub, priv).sendPush(user.Id, "T", "B", "")

	auth, _ := gotAuth.Load().(string)
	require.NotEmpty(t, auth, "expected a VAPID Authorization header")
	assert.Contains(t, decodeJWTClaims(t, auth), "mailto:liga@padel.test")
}

// decodeJWTClaims returns the raw claims JSON of the VAPID JWT carried in an
// "Authorization: vapid t=<jwt>, k=<key>" header.
func decodeJWTClaims(t *testing.T, authHeader string) string {
	t.Helper()
	_, rest, ok := strings.Cut(authHeader, "t=")
	require.True(t, ok, "no t= token in %q", authHeader)
	token, _, _ := strings.Cut(rest, ",")
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "expected a three-part JWT")
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	return string(claims)
}
