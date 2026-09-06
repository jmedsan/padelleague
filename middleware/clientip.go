package middleware

import (
	"net/netip"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// fastlyPrefixes is Fastly's published edge address list
// (https://api.fastly.com/public-ip-list, fetched 2026-09-06). Northflank fronts
// custom domains with Fastly, so a request whose last proxy hop is one of these
// came through the CDN.
var fastlyPrefixes = []string{
	"23.235.32.0/20", "43.249.72.0/22", "103.244.50.0/24", "103.245.222.0/23",
	"103.245.224.0/24", "104.156.80.0/20", "140.248.64.0/18", "140.248.128.0/17",
	"146.75.0.0/17", "151.101.0.0/16", "157.52.64.0/18", "167.82.0.0/17",
	"167.82.128.0/20", "167.82.160.0/20", "167.82.224.0/20", "172.111.64.0/18",
	"185.31.16.0/22", "199.27.72.0/21", "199.232.0.0/16",
	"2a04:4e40::/32", "2a04:4e42::/32",
}

var cdnPrefixes = sync.OnceValue(func() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(fastlyPrefixes))
	for _, s := range fastlyPrefixes {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
})

// ClientIP resolves the client address behind the Northflank proxy chain
// (optional Fastly CDN, then istio-envoy) from X-Forwarded-For, read right to
// left: the rightmost entry was appended by envoy and is the peer it accepted
// the connection from. When that peer is a Fastly edge the client is the entry
// before it, appended by Fastly. Anything further left is client-supplied and
// never trusted. Falls back to RemoteIP when no usable entry exists.
func ClientIP(e *core.RequestEvent) string {
	entries := splitForwardedFor(e.Request.Header.Get("X-Forwarded-For"))
	if len(entries) == 0 {
		return e.RemoteIP()
	}
	last := entries[len(entries)-1]
	if len(entries) >= 2 && isCDN(last) {
		return entries[len(entries)-2].String()
	}
	return last.String()
}

func isCDN(ip netip.Addr) bool {
	for _, p := range cdnPrefixes() {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// splitForwardedFor parses a comma-separated X-Forwarded-For value, dropping
// entries that are not valid IPs so they cannot shift the right-to-left count.
func splitForwardedFor(header string) []netip.Addr {
	var out []netip.Addr
	for _, part := range strings.Split(header, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		if ap, err := netip.ParseAddrPort(s); err == nil {
			s = ap.Addr().String()
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		out = append(out, addr.Unmap())
	}
	return out
}
