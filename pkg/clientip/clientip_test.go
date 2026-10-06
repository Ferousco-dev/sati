package clientip

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestResolve(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct client ignores forwarded header", "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"trusted proxy uses forwarded client", "10.0.0.2:80", "198.51.100.7", "198.51.100.7"},
		{"spoofed left entry is skipped", "10.0.0.2:80", "1.1.1.1, 198.51.100.7", "198.51.100.7"},
		{"chain of trusted proxies", "10.0.0.2:80", "198.51.100.7, 10.0.0.9", "198.51.100.7"},
		{"trusted proxy without header falls back to peer", "10.0.0.2:80", "", "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := Resolve(r, trusted); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
