package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudflareOrigin(t *testing.T) {
	const localProxy = "127.0.0.1:1234"
	s := &Server{trusted: parseNets([]string{"127.0.0.1/32"})}
	handler := s.cloudflareOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tests := []struct {
		name      string
		remote    string
		target    string
		forwarded []string
		want      int
	}{
		{"Cloudflare IPv4", localProxy, "/", []string{"203.0.113.1, 173.245.48.10"}, 200},
		{"Cloudflare IPv6", localProxy, "/", []string{"2606:4700::100"}, 200},
		{"non-Cloudflare peer", localProxy, "/", []string{"203.0.113.1"}, 403},
		{"earlier hop is not the peer", localProxy, "/", []string{"173.245.48.10, 203.0.113.1"}, 403},
		{"duplicate forwarded headers", localProxy, "/", []string{"203.0.113.1", "173.245.48.10"}, 403},
		{"untrusted proxy", "192.0.2.10:1234", "/", []string{"173.245.48.10"}, 403},
		{"unlisted loopback proxy", "127.0.0.2:1234", "/", []string{"173.245.48.10"}, 403},
		{"site request missing forwarding", localProxy, "/api/status", nil, 403},
		{"local health probe", localProxy, "http://127.0.0.1:8088/api/status", nil, 200},
		{"local homepage", localProxy, "http://127.0.0.1:8088/", nil, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.target, nil)
			r.RemoteAddr = tt.remote
			for _, value := range tt.forwarded {
				r.Header.Add("X-Forwarded-For", value)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Errorf("got status %d, want %d", w.Code, tt.want)
			}
		})
	}
}
