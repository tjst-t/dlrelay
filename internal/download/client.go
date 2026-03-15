package download

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// httpClient is a shared HTTP client with timeouts and SSRF protection.
var httpClient = &http.Client{
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DialContext:            ssrfSafeDialer,
	},
}

// AllowPrivateIPs disables SSRF protection for testing purposes.
// Must only be called from tests.
var AllowPrivateIPs bool

// ssrfSafeDialer wraps the default dialer to reject connections to private/loopback IPs.
func ssrfSafeDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}

	if !AllowPrivateIPs {
		for _, ip := range ips {
			if isPrivateIP(ip.IP) {
				return nil, fmt.Errorf("connections to private/loopback addresses are not allowed: %s", ip.IP)
			}
		}
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// isPrivateIP returns true for loopback, private, and link-local addresses.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// ValidateDownloadURL checks that the URL is a valid HTTP/HTTPS URL.
func ValidateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q: only http and https are allowed", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}
