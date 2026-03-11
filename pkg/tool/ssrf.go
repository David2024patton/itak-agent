package tool

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IsPrivateIP reports whether ip is in a private, loopback, or link-local range.
// Covers RFC1918 (10/8, 172.16/12, 192.168/16), loopback (127/8, ::1),
// and link-local (169.254/16, fe80::/10).
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() { // 0.0.0.0 or ::
		return true
	}

	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"fc00::/7"}, // unique local addresses
	}

	for _, r := range privateRanges {
		_, cidr, err := net.ParseCIDR(r.network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateURL parses a raw URL string and checks whether it resolves to a
// private-network address. If DNS resolution returns any private IP, the URL
// is rejected. This prevents SSRF via DNS rebinding or A/AAAA record tricks.
func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("ssrf: invalid URL: %w", err)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("ssrf: empty hostname in URL %q", rawURL)
	}

	// Direct IP check.
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateIP(ip) {
			return fmt.Errorf("ssrf: blocked request to private IP %s", ip)
		}
		return nil
	}

	// DNS resolution check - all returned IPs must be public.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("ssrf: DNS resolution failed for %q: %w", host, err)
	}

	for _, addr := range addrs {
		if IsPrivateIP(addr.IP) {
			log.Printf("[SSRF] DNS rebind blocked: %q resolved to private IP %s", host, addr.IP)
			return fmt.Errorf("ssrf: blocked request to %q - resolved to private IP %s", host, addr.IP)
		}
	}

	return nil
}

// ValidateRedirectChain walks the redirect history of an HTTP response and
// blocks any hop that targets a private-network address. Returns an error
// if any intermediate or final URL resolves to a private IP.
// The response must have been obtained with a CheckRedirect that recorded the
// chain, or use NewSSRFSafeTransport which blocks at the dial level.
func ValidateRedirectChain(resp *http.Response) error {
	if resp == nil || resp.Request == nil {
		return nil
	}

	// Walk the via chain if present (recorded by the client's CheckRedirect).
	req := resp.Request
	for req != nil {
		if err := ValidateURL(req.URL.String()); err != nil {
			return fmt.Errorf("ssrf: redirect chain blocked: %w", err)
		}
		req = req.Response.Request
		// Guard against nil - final request has no prior response.
		if req != nil && req.Response == nil {
			break
		}
		if req != nil && req.Response != nil {
			req = req.Response.Request
		} else {
			break
		}
	}

	return nil
}

// NewSSRFSafeTransport returns an http.Transport whose DialContext blocks all
// connections to private-network IPs. This is the primary defense: even if a
// URL looks safe at parse time, the actual TCP connection is checked.
func NewSSRFSafeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("ssrf: invalid address %q: %w", addr, err)
			}

			// Resolve the host and check every IP.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("ssrf: DNS lookup failed for %q: %w", host, err)
			}

			for _, ip := range ips {
				if IsPrivateIP(ip.IP) {
					log.Printf("[SSRF] Dial blocked: %s -> private IP %s", host, ip.IP)
					return nil, fmt.Errorf("ssrf: blocked connection to private IP %s (host: %s)", ip.IP, host)
				}
			}

			// All IPs are public, proceed with the dial.
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// NewSSRFSafeClient returns an HTTP client that blocks private-network
// connections and validates redirect chains. Use this instead of
// http.DefaultClient for any outbound HTTP requests from tools.
func NewSSRFSafeClient() *http.Client {
	return &http.Client{
		Transport: NewSSRFSafeTransport(),
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("ssrf: too many redirects")
			}

			// Validate each redirect target.
			target := req.URL.String()
			if err := ValidateURL(target); err != nil {
				return err
			}

			// Block HTTP -> HTTPS -> HTTP downgrade chains that could
			// smuggle requests to internal services.
			if len(via) > 0 {
				prev := via[len(via)-1].URL
				if prev.Scheme == "https" && req.URL.Scheme == "http" {
					// Allow downgrade only for public IPs.
					host := req.URL.Hostname()
					if ip := net.ParseIP(host); ip != nil && IsPrivateIP(ip) {
						log.Printf("[SSRF] HTTPS->HTTP downgrade blocked: %s -> private IP %s", host, ip)
						return fmt.Errorf("ssrf: blocked HTTPS->HTTP downgrade to private IP %s", ip)
					}
				}
			}

			return nil
		},
	}
}

// ssrfDangerousSchemes are URL schemes that should never be allowed in
// user-controlled URLs (file://, gopher://, etc.).
var ssrfDangerousSchemes = []string{
	"file", "gopher", "dict", "ftp", "ldap", "tftp",
}

// ValidateScheme checks that a URL uses only http or https.
func ValidateScheme(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("ssrf: invalid URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "http" || scheme == "https" {
		return nil
	}

	for _, blocked := range ssrfDangerousSchemes {
		if scheme == blocked {
			return fmt.Errorf("ssrf: blocked dangerous scheme %q", scheme)
		}
	}

	return fmt.Errorf("ssrf: unsupported scheme %q", scheme)
}
