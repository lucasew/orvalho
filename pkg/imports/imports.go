// Package imports is a specifier resolve chain. Handlers decide whether
// they own a specifier and may call next. Nothing is loaded until Resolve.
package imports

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	// ErrNotFound means no handler claimed the specifier.
	ErrNotFound = errors.New("imports: not found")
	// ErrSpecifier means the name is empty, absolute, or a broken scheme.
	ErrSpecifier = errors.New("imports: invalid specifier")
)

// Resolver looks up one specifier.
type Resolver[T any] func(specifier string) (T, error)

// Handler is one step in a resolve chain. Call next to pass the specifier
// on (or a rewritten one). First handler that does not call next wins.
type Handler[T any] interface {
	Resolve(specifier string, next Resolver[T]) (T, error)
}

// Func adapts a function to a Handler.
type Func[T any] func(specifier string, next Resolver[T]) (T, error)

func (f Func[T]) Resolve(specifier string, next Resolver[T]) (T, error) {
	if f == nil {
		return next(specifier)
	}
	return f(specifier, next)
}

// Map claims exact specifiers. A missing key calls next.
type Map[T any] map[string]T

func (m Map[T]) Resolve(specifier string, next Resolver[T]) (T, error) {
	v, ok := m[specifier]
	if !ok {
		return next(specifier)
	}
	return v, nil
}

// Scheme claims specifiers whose scheme matches Name (node, orvalho, …).
// Load is called only for those specifiers. ErrNotFound continues the chain.
type Scheme[T any] struct {
	Name string
	Load func(specifier string) (T, error)
}

func (s Scheme[T]) Resolve(specifier string, next Resolver[T]) (T, error) {
	if s.Load == nil || !strings.HasPrefix(specifier, s.Name+":") {
		return next(specifier)
	}
	v, err := s.Load(specifier)
	if err != nil && errors.Is(err, ErrNotFound) {
		return next(specifier)
	}
	return v, err
}

// Alias rewrites From to To and continues the chain.
type Alias[T any] struct {
	From, To string
}

func (a Alias[T]) Resolve(specifier string, next Resolver[T]) (T, error) {
	if specifier == a.From {
		specifier = a.To
	}
	return next(specifier)
}

// Resolve validates spec and runs handlers in order. Invalid specifiers
// never reach a handler, so a chain cannot become a filesystem walk.
func Resolve[T any](spec string, handlers ...Handler[T]) (T, error) {
	var zero T
	if !Valid(spec) {
		return zero, fmt.Errorf("%w: %q", ErrSpecifier, spec)
	}
	return Chain(handlers...)(spec)
}

// Chain composes handlers. The last next is always ErrNotFound.
func Chain[T any](handlers ...Handler[T]) Resolver[T] {
	next := func(spec string) (T, error) {
		var zero T
		return zero, fmt.Errorf("%w: %q", ErrNotFound, spec)
	}
	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		if h == nil {
			continue
		}
		n := next
		next = func(spec string) (T, error) {
			return h.Resolve(spec, n)
		}
	}
	return next
}

// Valid reports whether spec may reach a handler. Scheme names, bare
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

func isSchemeSpec(spec string) bool {
	colon := strings.IndexByte(spec, ':')
	return colon >= 1 && validScheme(spec[:colon])
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
