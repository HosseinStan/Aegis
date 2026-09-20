package security

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

func ParseCIDRs(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", value, err)
		}
		result = append(result, network)
	}
	return result, nil
}

func ClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	peerIP := net.ParseIP(peer)
	if peerIP == nil || !isTrusted(peerIP, trusted) {
		return peer
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for index := len(chain) - 1; index >= 0; index-- {
		candidate := strings.TrimSpace(chain[index])
		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		if !isTrusted(ip, trusted) {
			return candidate
		}
	}
	return peer
}

func isTrusted(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
