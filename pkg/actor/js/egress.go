package js

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// EgressList is a package allowlist for outbound fetch destinations.
// Empty means deny all (no ambient open proxy).
//
// Entries match the CUE #Egress shape:
//   - hostname: exact match on URL hostname (case-insensitive), e.g. "catfact.ninja"
//   - *.hostname: suffix match, e.g. "*.example.com" matches "a.example.com"
//   - http(s)://origin: scheme + host [+ port] must match, e.g. "https://catfact.ninja"
//   - "*": allow any http(s) host (explicit open egress; must be declared in the package)
type EgressList []string

// Allows reports whether rawURL is permitted by the allowlist.
func (e EgressList) Allows(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return e.AllowsURL(u)
}

// AllowsURL is [EgressList.Allows] for a parsed URL.
func (e EgressList) AllowsURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	for _, rule := range e {
		if ruleAllows(rule, u, host) {
			return true
		}
	}
	return false
}

func ruleAllows(rule string, u *url.URL, host string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}
	// Explicit package open egress (CUE allows bare "*"). Still only http(s).
	if rule == "*" {
		return true
	}
	if strings.HasPrefix(rule, "http://") || strings.HasPrefix(rule, "https://") {
		ru, err := url.Parse(rule)
		if err != nil {
			return false
		}
		if !strings.EqualFold(ru.Scheme, u.Scheme) {
			return false
		}
		// Origin match: host + explicit port when present on the rule.
		if !strings.EqualFold(ru.Hostname(), host) {
			return false
		}
		if ru.Port() != "" && ru.Port() != u.Port() {
			// Rule fixed a port; request must use the same.
			// url.Port() is empty when default; compare effective ports.
			if effectivePort(ru) != effectivePort(u) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(rule, "*.") {
		// *.example.com → suffix .example.com, and not the bare parent only
		// unless host equals example.com (common allowlist convenience).
		base := rule[2:]
		if base == "" {
			return false
		}
		if strings.EqualFold(host, base) {
			return true
		}
		suffix := "." + base
		if len(host) > len(suffix) && strings.EqualFold(host[len(host)-len(suffix):], suffix) {
			return true
		}
		return false
	}
	// Exact hostname, or host:port (port must match when present on the rule).
	if h, p, err := net.SplitHostPort(rule); err == nil {
		if !strings.EqualFold(host, h) {
			return false
		}
		if p == "" {
			return true
		}
		return p == effectivePort(u)
	}
	return strings.EqualFold(host, rule)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// CheckURL returns nil if allowed, or a descriptive error if denied.
func (e EgressList) CheckURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("egress denied: scheme %q not allowed", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("egress denied: missing host")
	}
	if !e.AllowsURL(u) {
		return fmt.Errorf("egress denied: host %q not in allowlist", u.Hostname())
	}
	return nil
}
