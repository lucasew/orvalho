// Package imports validates specifiers and looks up files in an
// injected tree. It does not know about Isolates or Bindings.
package imports

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	// ErrNotFound means the tree has no file for the specifier.
	ErrNotFound = errors.New("imports: not found")
	// ErrSpecifier means the name is empty, absolute, or a broken scheme.
	ErrSpecifier = errors.New("imports: invalid specifier")
)

// Valid reports whether spec may be resolved. Scheme names, bare
// package names, and relative paths are allowed. Empty names and
// absolute host paths (/…) are rejected so require is not a host walk.
func Valid(spec string) bool {
	if spec == "" || strings.ContainsRune(spec, 0) {
		return false
	}
	if spec[0] == '/' {
		return false
	}
	if isRelative(spec) {
		return true
	}
	if colon := strings.IndexByte(spec, ':'); colon >= 1 {
		if colon == len(spec)-1 || !validScheme(spec[:colon]) {
			return false
		}
		rest := spec[colon+1:]
		if rest[0] == '/' || rest[0] == '.' {
			return false
		}
		for _, seg := range strings.Split(rest, "/") {
			if seg == "" || seg == "." || seg == ".." {
				return false
			}
		}
		return true
	}
	return validBare(spec)
}

func isRelative(spec string) bool {
	return spec == "." || spec == ".." ||
		strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
}

func validBare(spec string) bool {
	if spec[0] == '@' {
		rest := spec[1:]
		slash := strings.IndexByte(rest, '/')
		if slash < 1 || slash == len(rest)-1 {
			return false
		}
		return validSegments(rest)
	}
	if spec[0] == '.' {
		return false
	}
	return validSegments(spec)
}

func validSegments(s string) bool {
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func validScheme(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	if r < 'a' || r > 'z' {
		return false
	}
	for _, r := range s[size:] {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}
