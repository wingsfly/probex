package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPAllowlist returns a middleware that rejects requests from IPs not in
// the given CIDR list. An empty list means allow all (no filtering).
// Relies on chi middleware.RealIP being applied first so that
// r.RemoteAddr reflects the true client IP behind proxies.
func IPAllowlist(cidrs []string) (func(http.Handler) http.Handler, error) {
	if len(cidrs) == 0 {
		// No restriction — pass through
		return func(next http.Handler) http.Handler { return next }, nil
	}

	var nets []*net.IPNet
	hasIPv4Loopback := false
	for _, c := range cidrs {
		// Allow bare IPs like "127.0.0.1" without mask
		if !strings.Contains(c, "/") {
			if net.ParseIP(c) != nil {
				if strings.Contains(c, ":") {
					c += "/128"
				} else {
					c += "/32"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		nets = append(nets, ipNet)
		// Check if this covers IPv4 loopback (127.x.x.x)
		if ipNet.Contains(net.IPv4(127, 0, 0, 1)) {
			hasIPv4Loopback = true
		}
	}
	// Auto-add IPv6 loopback (::1) when IPv4 loopback is allowed,
	// since macOS/Linux often connect via ::1 for localhost
	if hasIPv4Loopback {
		_, ipv6Lo, _ := net.ParseCIDR("::1/128")
		nets = append(nets, ipv6Lo)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r.RemoteAddr)
			if ip == nil || !matchAny(ip, nets) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func extractIP(remoteAddr string) net.IP {
	// remoteAddr is "ip:port" or just "ip" (unix socket etc.)
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}

func matchAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
