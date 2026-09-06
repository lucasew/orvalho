package workers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lucasew/orvalho/pkg/imports"
)

// Import is one step in the require chain. Resolve returns a Binding
// or calls next. Earlier Imports wrap later ones, so the list is
// tried in order. workers.Handler is the HTTP adapter; this is not that.
type Import interface {
	Resolve(specifier string, next func(string) (Binding, error)) (Binding, error)
}

// Map claims exact specifiers.
type Map map[string]Binding

func (m Map) Resolve(specifier string, next func(string) (Binding, error)) (Binding, error) {
	v, ok := m[specifier]
	if !ok {
		return next(specifier)
	}
	return v, nil
}

// Scheme claims specifiers whose scheme matches Name (node, orvalho, …).
// Load is called only for those specifiers. ErrNotFound continues the chain.
type Scheme struct {
	Name string
	Load func(specifier string) (Binding, error)
}

func (s Scheme) Resolve(specifier string, next func(string) (Binding, error)) (Binding, error) {
	if s.Load == nil || !strings.HasPrefix(specifier, s.Name+":") {
		return next(specifier)
	}
	v, err := s.Load(specifier)
	if err != nil && errors.Is(err, ErrModuleNotFound) {
		return next(specifier)
	}
	return v, err
}

// Alias rewrites From to To and continues the chain.
type Alias struct {
	From, To string
}

func (a Alias) Resolve(specifier string, next func(string) (Binding, error)) (Binding, error) {
	if specifier == a.From {
		specifier = a.To
	}
	return next(specifier)
}

func resolveImport(spec string, handlers []Import) (Binding, error) {
	if !imports.Valid(spec) {
		return nil, fmt.Errorf("%w: %q", ErrModuleSpecifier, spec)
	}
	next := func(spec string) (Binding, error) {
		return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
	}
	// Earlier entries wrap later ones, so the slice is tried in order.
	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		if h == nil {
			continue
		}
		n := next
		next = func(spec string) (Binding, error) {
			return h.Resolve(spec, n)
		}
	}
	return next(spec)
}
