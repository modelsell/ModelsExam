package ssrf

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeResolver map[string][]net.IP

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, ip := range f[host] {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

func TestBlocksPrivateTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()

	c := New(false)
	if err := c.ValidateURL(context.Background(), srv.URL); err == nil {
		t.Fatal("loopback IP literal must be rejected by preflight")
	}
	if _, err := c.Get(srv.URL); err == nil {
		t.Fatal("loopback IP literal must be rejected at dial time")
	}
	// A hostname that resolves to loopback is rejected both by preflight and by the dialer.
	c.resolver = fakeResolver{"evil.example": {net.ParseIP("127.0.0.1")}}
	if err := c.ValidateURL(context.Background(), "http://evil.example/"); err == nil {
		t.Fatal("DNS answer in a private range must be rejected")
	}
	if _, err := c.Get("http://evil.example:" + strings.Split(srv.URL, ":")[2]); err == nil {
		t.Fatal("DNS rebinding to loopback must be rejected at dial time")
	}
}

func TestAllowPrivateForDevelopment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	c := New(true)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestNoRedirectFollow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()
	resp, err := New(true).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status %d, want the redirect returned unfollowed", resp.StatusCode)
	}
}
