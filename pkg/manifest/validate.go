package manifest

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// actorIDPattern: DNS-label style, 1–63 chars, starts with a letter.
var actorIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// bindingNamePattern: env-style identifier.
var bindingNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Error is a single manifest field or document error.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return "manifest: <nil>"
	}
	if e.Field != "" {
		return fmt.Sprintf("manifest: %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("manifest: %s", e.Message)
}

// ValidationError collects one or more field errors.
type ValidationError struct {
	Errors []*Error
}

func (v *ValidationError) Error() string {
	if v == nil || len(v.Errors) == 0 {
		return "manifest: validation failed"
	}
	if len(v.Errors) == 1 {
		return v.Errors[0].Error()
	}
	parts := make([]string, len(v.Errors))
	for i, e := range v.Errors {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

// Unwrap returns the underlying errors for errors.Is / errors.As.
func (v *ValidationError) Unwrap() []error {
	out := make([]error, len(v.Errors))
	for i, e := range v.Errors {
		out[i] = e
	}
	return out
}

type validator struct {
	errs []*Error
}

func (v *validator) add(field, msg string) {
	v.errs = append(v.errs, &Error{Field: field, Message: msg})
}

func (v *validator) result() error {
	if len(v.errs) == 0 {
		return nil
	}
	return &ValidationError{Errors: v.errs}
}

// Validate checks m against the orvalho.json schema rules.
// On success, string fields are normalized (trimmed).
func (m *Manifest) Validate() error {
	if m == nil {
		return &Error{Message: "nil manifest"}
	}
	m.normalize()

	v := &validator{}

	if m.SchemaVersion == 0 {
		v.add("schema_version", "required (expected 1)")
	} else if m.SchemaVersion != SchemaVersionCurrent {
		v.add("schema_version", fmt.Sprintf("unsupported version %d (expected %d)", m.SchemaVersion, SchemaVersionCurrent))
	}

	if m.ID == "" {
		v.add("id", "required")
	} else if !actorIDPattern.MatchString(m.ID) {
		v.add("id", "must be a DNS-label style identifier: lowercase letter, then [a-z0-9-], length 1–63")
	} else if strings.HasSuffix(m.ID, "-") || strings.Contains(m.ID, "--") {
		// Pattern allows trailing/double hyphens; reject for hygiene.
		v.add("id", "must not end with '-' or contain '--'")
	}

	if m.Name != "" {
		if !isPrintableDisplayName(m.Name) {
			v.add("name", "must be printable non-control text")
		}
		if len(m.Name) > 128 {
			v.add("name", "must be at most 128 characters")
		}
	}

	if m.Entry == "" {
		v.add("entry", "required")
	} else if err := validatePackagePath(m.Entry); err != "" {
		v.add("entry", err)
	}

	if m.Runtime == "" {
		v.add("runtime", "required")
	} else if m.Runtime != RuntimeJS {
		v.add("runtime", fmt.Sprintf("unsupported runtime %q (expected %q)", m.Runtime, RuntimeJS))
	}

	if m.Bindings != nil {
		v.validateBindings(m.Bindings)
	}

	for i, pattern := range m.Egress {
		field := fmt.Sprintf("egress[%d]", i)
		if pattern == "" {
			v.add(field, "must not be empty")
			continue
		}
		if err := validateEgressPattern(pattern); err != "" {
			v.add(field, err)
		}
	}

	if m.Port != 0 {
		if m.Port < 1 || m.Port > 65535 {
			v.add("port", "must be between 1 and 65535")
		}
	}

	if m.Publish != nil {
		if m.Publish.Port != 0 {
			if m.Publish.Port < 1 || m.Publish.Port > 65535 {
				v.add("publish.port", "must be between 1 and 65535")
			}
		}
		if m.Publish.Protocol != "" && m.Publish.Protocol != ProtocolHTTP {
			v.add("publish.protocol", fmt.Sprintf("unsupported protocol %q (expected %q)", m.Publish.Protocol, ProtocolHTTP))
		}
	}

	return v.result()
}

func (v *validator) validateBindings(b *Bindings) {
	if b.Assets != nil {
		a := b.Assets
		if a.Root == "" && len(a.Paths) == 0 {
			v.add("bindings.assets", "must set root and/or paths")
		}
		if a.Root != "" {
			if err := validatePackagePath(a.Root); err != "" {
				v.add("bindings.assets.root", err)
			}
		}
		for i, p := range a.Paths {
			field := fmt.Sprintf("bindings.assets.paths[%d]", i)
			if p == "" {
				v.add(field, "must not be empty")
				continue
			}
			if err := validatePackagePath(p); err != "" {
				v.add(field, err)
			}
		}
	}

	seenSecrets := make(map[string]struct{})
	for i, s := range b.Secrets {
		field := fmt.Sprintf("bindings.secrets[%d]", i)
		v.validateNameBinding(field, s, seenSecrets)
	}

	seenConfig := make(map[string]struct{})
	for i, c := range b.Config {
		field := fmt.Sprintf("bindings.config[%d]", i)
		v.validateNameBinding(field, c, seenConfig)
	}

	// Secret and config names share the env namespace; reject cross-duplicates.
	for name := range seenSecrets {
		if _, ok := seenConfig[name]; ok {
			v.add("bindings", fmt.Sprintf("name %q declared in both secrets and config", name))
		}
	}
}

func (v *validator) validateNameBinding(field string, b NameBinding, seen map[string]struct{}) {
	if b.Name == "" {
		v.add(field+".name", "required")
		return
	}
	if !bindingNamePattern.MatchString(b.Name) {
		v.add(field+".name", "must match [A-Za-z_][A-Za-z0-9_]*")
		return
	}
	if _, dup := seen[b.Name]; dup {
		v.add(field+".name", fmt.Sprintf("duplicate name %q", b.Name))
		return
	}
	seen[b.Name] = struct{}{}
}

// validatePackagePath ensures a package-relative path uses forward slashes,
// is non-empty, not absolute, and has no ".." segments.
func validatePackagePath(p string) string {
	if p == "" {
		return "must not be empty"
	}
	if strings.Contains(p, "\\") {
		return "must use forward slashes, not backslashes"
	}
	if strings.HasPrefix(p, "/") {
		return "must be package-relative (not absolute)"
	}
	if strings.Contains(p, "://") {
		return "must be a package path, not a URL"
	}
	// Reject empty segments and "." / ".." traversal.
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return "must not contain empty path segments"
		}
		if seg == "." || seg == ".." {
			return `must not contain "." or ".." path segments`
		}
	}
	return ""
}

// validateEgressPattern checks a single egress allowlist entry.
//
// Accepted forms:
//   - host:            example.com
//   - wildcard host:   *.example.com
//   - origin:          https://example.com
//   - wildcard origin: https://*.example.com
//
// Bare "*" is rejected (would make the actor an open proxy). Paths, queries,
// and userinfo are rejected. Only http/https schemes are allowed when present.
func validateEgressPattern(pattern string) string {
	if pattern == "*" {
		return `bare "*" is not allowed (open proxy); list specific hosts or patterns`
	}
	if strings.ContainsAny(pattern, " \t\r\n") {
		return "must not contain whitespace"
	}

	hostPart := pattern
	if strings.Contains(pattern, "://") {
		u, err := url.Parse(pattern)
		if err != nil {
			return fmt.Sprintf("invalid origin pattern: %v", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Sprintf("unsupported scheme %q (expected http or https)", u.Scheme)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return "must not include query or fragment"
		}
		if u.Path != "" && u.Path != "/" {
			return "must not include a path (host/origin only)"
		}
		if u.User != nil {
			return "must not include userinfo"
		}
		if u.Host == "" {
			return "origin pattern missing host"
		}
		hostPart = u.Host
	} else if strings.Contains(pattern, "/") {
		return "must not include a path (host/origin only)"
	}

	// Strip optional :port from host for hostname validation.
	host := hostPart
	if h, port, err := net.SplitHostPort(hostPart); err == nil {
		host = h
		if port == "" {
			return "invalid empty port"
		}
		// port already numeric from SplitHostPort when using bracket IPv6;
		// for names, SplitHostPort still works with host:port.
	} else if strings.Count(hostPart, ":") == 1 && !strings.HasPrefix(hostPart, "[") {
		// hostname:port without brackets — SplitHostPort needs host:port and works for this.
		// If it failed, treat whole string as hostname (no port).
	}

	// Bracketed IPv6 literals are not useful allowlist entries for v1 fetch policy;
	// require hostname form (optionally wildcard).
	if strings.HasPrefix(host, "[") {
		return "IP literals are not allowed; use hostnames"
	}
	if ip := net.ParseIP(host); ip != nil {
		return "IP literals are not allowed; use hostnames"
	}

	if host == "*" {
		return `bare "*" host is not allowed`
	}

	wildcard := false
	if strings.HasPrefix(host, "*.") {
		wildcard = true
		host = host[2:]
		if host == "" {
			return `wildcard pattern missing domain after "*."`
		}
		if strings.HasPrefix(host, "*.") || strings.Contains(host, "*") {
			return `only a single leading "*." wildcard is allowed`
		}
	} else if strings.Contains(host, "*") {
		return `wildcard "*" only allowed as a leading "*." label`
	}

	if err := validateHostname(host); err != "" {
		if wildcard {
			return "wildcard base: " + err
		}
		return err
	}
	return ""
}

func validateHostname(host string) string {
	if host == "" {
		return "hostname is empty"
	}
	if len(host) > 253 {
		return "hostname too long"
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "hostname must not start or end with '.'"
	}
	labels := strings.Split(host, ".")
	if len(labels) < 1 {
		return "invalid hostname"
	}
	for _, label := range labels {
		if label == "" {
			return "hostname has empty label"
		}
		if len(label) > 63 {
			return "hostname label too long"
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "hostname label must not start or end with '-'"
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return "hostname contains invalid character"
		}
	}
	return ""
}

func isPrintableDisplayName(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			return false
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
