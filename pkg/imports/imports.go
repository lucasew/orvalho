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
	// ErrSpecifier means the name is not a scheme:rest specifier.
	ErrSpecifier = errors.New("imports: invalid specifier")
)

// Resolver looks up one specifier.
type Resolver[T any] func(specifier string) (T, error)

// Handler is one step in a resolve chain. Call next to pass the specifier
// on (or a rewritten one). First handler that does not call next wins.
type Handler[T any] func(specifier string, next Resolver[T]) (T, error)

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
			return h(spec, n)
		}
	}
	return next
}

// Map claims exact specifiers. A missing key calls next.
func Map[T any](m map[string]T) Handler[T] {
	return func(spec string, next Resolver[T]) (T, error) {
		v, ok := m[spec]
		if !ok {
			return next(spec)
		}
		return v, nil
	}
}

// Scheme claims specifiers whose scheme matches (node, orvalho, …).
// load is called only for those specifiers, with the full name.
// ErrNotFound from load continues the chain.
func Scheme[T any](scheme string, load func(specifier string) (T, error)) Handler[T] {
	prefix := scheme + ":"
	return func(spec string, next Resolver[T]) (T, error) {
		if load == nil || !strings.HasPrefix(spec, prefix) {
			return next(spec)
		}
		v, err := load(spec)
		if err != nil && errors.Is(err, ErrNotFound) {
			return next(spec)
		}
		return v, err
	}
}

// Alias rewrites from to to and continues the chain.
func Alias[T any](from, to string) Handler[T] {
	return func(spec string, next Resolver[T]) (T, error) {
		if spec == from {
			spec = to
		}
		return next(spec)
	}
}

// Valid reports whether spec is a scheme:rest name. Relative paths,
// absolute paths, and bare names are rejected.
func Valid(spec string) bool {
	colon := strings.IndexByte(spec, ':')
	if colon < 1 || colon == len(spec)-1 {
		return false
	}
	if !validScheme(spec[:colon]) {
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
