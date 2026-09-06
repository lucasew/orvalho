package workers

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/dop251/goja"
	"github.com/lucasew/orvalho/pkg/imports"
)

func (iso *Isolate) installModules() {
	iso.moduleCache = make(map[string]goja.Value)
	mustRuntimeSet(iso.vm, "require", iso.jsRequire)
	mustRuntimeSet(iso.vm, "getBuiltinModule", iso.jsRequire)
}

func (iso *Isolate) jsRequire(call goja.FunctionCall) goja.Value {
	spec := ""
	if len(call.Arguments) > 0 {
		spec = call.Argument(0).String()
	}
	v, err := iso.loadModule(spec)
	if err != nil {
		panic(iso.vm.NewGoError(err))
	}
	return v
}

func (iso *Isolate) loadModule(spec string) (goja.Value, error) {
	spec = iso.rewriteRelative(spec)
	if v, ok := iso.moduleCache[spec]; ok {
		return v, nil
	}
	v, err := imports.Resolve(spec, withImportFrom(iso.opts.Imports, iso.importFrom)...)
	if err != nil {
		if errors.Is(err, imports.ErrSpecifier) {
			return nil, fmt.Errorf("%w: %q", ErrModuleSpecifier, spec)
		}
		if errors.Is(err, imports.ErrNotFound) {
			return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
		}
		return nil, err
	}
	switch x := v.(type) {
	case imports.Script:
		return iso.loadScript(spec, x.Source, x.File)
	case Binding:
		if x == nil {
			return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
		}
		obj, err := x.Materialize(iso)
		if err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, fmt.Errorf("%w: %q", ErrBindNilObject, spec)
		}
		iso.moduleCache[spec] = obj
		return obj, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
	}
}

func withImportFrom(handlers []imports.Handler[any], from string) []imports.Handler[any] {
	if from == "" || len(handlers) == 0 {
		return handlers
	}
	out := make([]imports.Handler[any], len(handlers))
	for i, h := range handlers {
		switch n := h.(type) {
		case imports.NodeModules:
			n.From = from
			out[i] = n
		case *imports.NodeModules:
			cp := *n
			cp.From = from
			out[i] = cp
		default:
			out[i] = h
		}
	}
	return out
}

func (iso *Isolate) rewriteRelative(spec string) string {
	if iso.importFrom == "" {
		return spec
	}
	if spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return path.Clean(path.Join(path.Dir(iso.importFrom), spec))
	}
	return spec
}

func (iso *Isolate) loadScript(key, source, file string) (goja.Value, error) {
	exports := iso.vm.NewObject()
	module := iso.vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	iso.moduleCache[key] = exports

	prev := iso.importFrom
	if file != "" {
		iso.importFrom = file
	}
	defer func() { iso.importFrom = prev }()

	wrapped := "(function (require, module, exports) {\n" + source + "\n})"
	v, err := iso.vm.RunString(wrapped)
	if err != nil {
		delete(iso.moduleCache, key)
		return nil, err
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		delete(iso.moduleCache, key)
		return nil, fmt.Errorf("workers: script %q is not a function", key)
	}
	_, err = fn(goja.Undefined(), iso.vm.Get("require"), module, exports)
	if err != nil {
		delete(iso.moduleCache, key)
		return nil, err
	}
	final := module.Get("exports")
	iso.moduleCache[key] = final
	return final, nil
}
