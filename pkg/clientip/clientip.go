package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Resolve trusts X-Forwarded-For only when the direct peer is a trusted proxy,
// and walks the chain from the right so a client cannot spoof its own address.
func Resolve(r *http.Request, trusted []netip.Prefix) string {
	peer := peerAddr(r.RemoteAddr)
	if !peer.IsValid() || !contains(trusted, peer) {
		return peer.String()
	}

	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		if !contains(trusted, addr) {
			return addr.String()
		}
	}
	return peer.String()
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func contains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
