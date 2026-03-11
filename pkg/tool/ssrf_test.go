package tool

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		// Private ranges
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},

		// Loopback
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"::1", true},

		// Link-local
		{"169.254.1.1", true},
		{"fe80::1", true},

		// Unspecified
		{"0.0.0.0", true},
		{"::", true},

		// Public IPs
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"104.26.10.1", false},
		{"2606:4700::1", false},

		// Edge of private ranges (public side)
		{"172.15.255.255", false},
		{"172.32.0.0", false},
		{"11.0.0.0", false},
		{"192.167.255.255", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP: %s", tt.ip)
		}
		got := IsPrivateIP(ip)
		if got != tt.private {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestValidateURL(t *testing.T) {
	// Private IPs should be blocked.
	blocked := []string{
		"http://192.168.1.1",
		"http://10.0.0.1:8080",
		"http://127.0.0.1/admin",
		"http://[::1]:9090",
		"http://0.0.0.0",
		"http://172.16.0.1/api",
	}
	for _, u := range blocked {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) should have blocked, but allowed", u)
		}
	}

	// Invalid URLs should also error.
	invalid := []string{
		"://no-scheme",
		"http://",
	}
	for _, u := range invalid {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) should have errored, but allowed", u)
		}
	}
}

func TestValidateScheme(t *testing.T) {
	allowed := []string{
		"http://example.com",
		"https://example.com",
		"HTTP://EXAMPLE.COM",
		"HTTPS://example.com/path",
	}
	for _, u := range allowed {
		if err := ValidateScheme(u); err != nil {
			t.Errorf("ValidateScheme(%q) blocked: %v", u, err)
		}
	}

	blocked := []string{
		"file:///etc/passwd",
		"gopher://evil.com",
		"ftp://fileserver.internal",
		"dict://attacker.com",
		"ldap://dc.internal",
	}
	for _, u := range blocked {
		if err := ValidateScheme(u); err == nil {
			t.Errorf("ValidateScheme(%q) should have blocked, but allowed", u)
		}
	}
}
