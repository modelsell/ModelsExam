package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Client is an outbound HTTP client whose every dial is validated against the
// SSRF policy after DNS resolution, so a name that resolves to a private
// address (including DNS rebinding) can never be connected to. It does not use
// proxies and never follows redirects.
type Client struct {
	*http.Client
	protection *SSRFProtection
	resolver   IPResolver
	transport  *http.Transport
}

// New returns a client. allowPrivate permits loopback/private targets and is
// meant for local development only.
func New(allowPrivate bool) *Client {
	p := &SSRFProtection{
		AllowPrivateIp:         allowPrivate,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		ApplyIPFilterForDomain: true,
	}
	c := &Client{protection: p, resolver: net.DefaultResolver}
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	c.transport = &http.Transport{
		Proxy:                 nil,
		DialContext:           c.dial(dialer),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 90 * time.Second,
	}
	c.Client = &http.Client{
		Transport:     c.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c
}

func (c *Client) dial(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, portText, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address %s: %w", addr, err)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", portText)
		}
		if err := c.protection.ValidateNetworkTarget(host, port); err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); ip != nil {
			return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
		}
		resolved, err := c.resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed for %s: %v", host, err)
		}
		var last error
		tried := false
		for _, a := range resolved {
			if a.IP == nil {
				continue
			}
			if err := c.protection.ValidateResolvedIP(host, a.IP); err != nil {
				return nil, err
			}
			tried = true
			// Dial the verified IP itself so a second lookup cannot differ.
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), portText))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		if !tried {
			return nil, fmt.Errorf("DNS resolution for %s returned no usable IP addresses", host)
		}
		return nil, last
	}
}

// ValidateURL performs the preflight check before any request is sent.
func (c *Client) ValidateURL(ctx context.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.protection.ValidateURLWithResolver(ctx, rawURL, c.resolver)
}

func (c *Client) CloseIdleConnections() { c.transport.CloseIdleConnections() }
