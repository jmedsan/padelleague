package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
)

func TestClientIP(t *testing.T) {
	t.Parallel()
	const fastlyEdge = "167.82.130.61"
	tests := []struct {
		name string
		xff  string
		want string
	}{
		{name: "no header falls back to remote addr", xff: "", want: "10.0.0.9"},
		{name: "direct ingress path takes rightmost", xff: "203.0.113.5", want: "203.0.113.5"},
		{name: "direct path ignores client-prepended entry", xff: "1.2.3.4, 203.0.113.5", want: "203.0.113.5"},
		{name: "cdn path takes entry before fastly edge", xff: "2a0c:5a83:110b:a200:a0b1:8f62:e4c:2c95, " + fastlyEdge, want: "2a0c:5a83:110b:a200:a0b1:8f62:e4c:2c95"},
		{name: "cdn path ignores client-prepended entry", xff: "1.2.3.4, 203.0.113.5, " + fastlyEdge, want: "203.0.113.5"},
		{name: "spoofed fastly edge without a preceding entry is used as is", xff: fastlyEdge, want: fastlyEdge},
		{name: "ipv6 fastly edge is recognized", xff: "203.0.113.7, 2a04:4e42::1", want: "203.0.113.7"},
		{name: "malformed entries are dropped", xff: "garbage, 203.0.113.5, not-an-ip", want: "203.0.113.5"},
		{name: "host:port entry is accepted", xff: "203.0.113.5:4433", want: "203.0.113.5"},
		{name: "ipv4-mapped ipv6 is unmapped", xff: "::ffff:203.0.113.5", want: "203.0.113.5"},
		{name: "only malformed entries fall back to remote addr", xff: "garbage", want: "10.0.0.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("POST", "/login", nil)
			req.RemoteAddr = "10.0.0.9:51234"
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			req.Header.Set("Fastly-Client-IP", "9.9.9.9")
			req.Header.Set("X-Real-IP", "9.9.9.9")
			e := &core.RequestEvent{}
			e.Request = req
			assert.Equal(t, tc.want, ClientIP(e))
		})
	}
}
