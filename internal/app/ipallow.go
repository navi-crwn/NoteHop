package app

import (
	"net"
	"net/http"
	"strings"
)

// IP allowlist: only these addresses or networks may read or change the
// note (an office network, a VPN exit). The admin link is never blocked, so
// the owner can't lock themselves out. Addresses aren't logged.
var ipAllow = feature{
	name: "ip-allow",
	beforeRead: func(c hookCtx) *hookErr {
		if !allowedIP(c.cfg.IPAllow, c.ip) {
			return &hookErr{http.StatusForbidden, "err.ip_blocked", nil}
		}
		return nil
	},
	beforeWrite: func(c hookCtx) *hookErr {
		if !allowedIP(c.cfg.IPAllow, c.ip) {
			return &hookErr{http.StatusForbidden, "err.ip_blocked", nil}
		}
		return nil
	},
	meta: func(c hookCtx, m *metaResp) { m.IPLimited = len(c.cfg.IPAllow) > 0 },
}

func allowedIP(list []string, ip string) bool {
	if len(list) == 0 {
		return true
	}
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, s := range list {
		if _, n, err := net.ParseCIDR(s); err == nil && n.Contains(addr) {
			return true
		}
		if one := net.ParseIP(s); one != nil && one.Equal(addr) {
			return true
		}
	}
	return false
}

// cleanIPList accepts IPs and CIDRs, one per entry, at most 50.
func cleanIPList(in []string) ([]string, bool) {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(s); err != nil && net.ParseIP(s) == nil {
			return nil, false
		}
		out = append(out, s)
	}
	return out, len(out) <= 50
}
