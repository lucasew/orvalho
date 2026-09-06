package workers

import (
	"errors"
	"fmt"

	"github.com/lucasew/orvalho/pkg/imports"
)

// Import is a resolve step that yields a Binding. The types live in
// [imports]; these aliases pin T to Binding so Options.Imports stays
// []Import. workers.Handler is the HTTP adapter; this is not that.
type Import = imports.Handler[Binding]

// Map, Scheme, and Alias are Binding-pinned copies of the imports types.
type (
	Map    = imports.Map[Binding]
	Scheme = imports.Scheme[Binding]
	Alias  = imports.Alias[Binding]
)

func resolveImport(spec string, handlers []Import) (Binding, error) {
	b, err := imports.Resolve(spec, handlers...)
	if err == nil {
		return b, nil
	}
	if errors.Is(err, imports.ErrSpecifier) {
		return nil, fmt.Errorf("%w: %q", ErrModuleSpecifier, spec)
	}
	if errors.Is(err, imports.ErrNotFound) {
		return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
	}
	return nil, err
}
