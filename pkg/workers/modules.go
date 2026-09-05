package workers

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/lucasew/orvalho/pkg/imports"
)

// NewScriptBinding returns a Binding that evaluates source as CommonJS
// (require, module, exports) on first require. Source MUST NOT call host
// I/O; it may only require() specifiers the Imports chain resolves.
func NewScriptBinding(source string) Binding {
	return scriptBinding{source: source}
}

type scriptBinding struct {
	source string
}

func (scriptBinding) Materialize(*Isolate) (*goja.Object, error) {
	// loadModule handles CommonJS evaluation so the cache slot exists
	// before the factory runs (circular require).
	return nil, fmt.Errorf("workers: script binding loads via require")
}

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
	if v, ok := iso.moduleCache[spec]; ok {
		return v, nil
	}
	b, err := imports.Resolve(spec, iso.opts.Imports...)
	if err != nil {
		if errors.Is(err, imports.ErrSpecifier) {
			return nil, fmt.Errorf("%w: %q", ErrModuleSpecifier, spec)
		}
		if errors.Is(err, imports.ErrNotFound) {
			return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
		}
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, spec)
	}
	if sb, isScript := b.(scriptBinding); isScript {
		return iso.loadScript(spec, sb.source)
	}
	obj, err := b.Materialize(iso)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("%w: %q", ErrBindNilObject, spec)
	}
	iso.moduleCache[spec] = obj
	return obj, nil
}

func (iso *Isolate) loadScript(spec, source string) (goja.Value, error) {
	exports := iso.vm.NewObject()
	module := iso.vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	iso.moduleCache[spec] = exports

	wrapped := "(function (require, module, exports) {\n" + source + "\n})"
	v, err := iso.vm.RunString(wrapped)
	if err != nil {
		delete(iso.moduleCache, spec)
		return nil, err
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		delete(iso.moduleCache, spec)
		return nil, fmt.Errorf("workers: script binding %q is not a function", spec)
	}
	_, err = fn(goja.Undefined(), iso.vm.Get("require"), module, exports)
	if err != nil {
		delete(iso.moduleCache, spec)
		return nil, err
	}
	final := module.Get("exports")
	iso.moduleCache[spec] = final
	return final, nil
}
