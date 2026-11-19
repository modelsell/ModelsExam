package server

import (
	"net"
	"net/http"
	"strings"
)

// cloudflareNets are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/). A request whose proxy-reported address is
// in them came through Cloudflare, which sets CF-Connecting-IP.
var cloudflareNets = parseNets([]string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
	"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17",
	"162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
	"2a06:98c0::/29", "2c0f:f248::/32",
})

func (s *Server) cloudflareOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isCloudflareOrigin(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isCloudflareOrigin(r *http.Request) bool {
	peer := remoteIP(r)
	if !peer.IsLoopback() || !inNets(peer, s.trusted) {
		return false
	}
	forwarded := r.Header.Values("X-Forwarded-For")
	if len(forwarded) == 0 {
		// The local service probe connects to the loopback listener directly.
		host, _, _ := net.SplitHostPort(r.Host)
		return net.ParseIP(host).IsLoopback() && r.Method == http.MethodGet && r.URL.Path == "/api/status"
	}
	if len(forwarded) != 1 {
		return false
	}
	// Caddy folds X-Forwarded-For into one header and appends its socket peer
	// last. Earlier hops and CF-Connecting-IP may describe the visitor.
	chain := forwarded[0]
	last := strings.TrimSpace(chain[strings.LastIndexByte(chain, ',')+1:])
	return inNets(net.ParseIP(last), cloudflareNets)
}
