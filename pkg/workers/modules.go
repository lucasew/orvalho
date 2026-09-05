package workers

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/dop251/goja"
	"github.com/lucasew/orvalho/pkg/imports"
)

// NewScriptBinding returns a Binding that evaluates source as CommonJS
// (require, module, exports) on first require. Source MUST NOT call host
// I/O; it may only require() specifiers the Imports chain resolves.
func NewScriptBinding(source string) Binding {
	return scriptBinding{source: source}
}

// NodeModules resolves bare specifiers against FS. Relative require()
// calls are rewritten to a path inside FS using the current script file.
type NodeModules struct {
	FS fs.FS
}

func (n NodeModules) Resolve(spec string, next imports.Resolver[Binding]) (Binding, error) {
	file, ok := (imports.NodeModules{FS: n.FS}).Lookup(spec)
	if !ok {
		return next(spec)
	}
	data, err := fs.ReadFile(n.FS, file)
	if err != nil {
		return nil, err
	}
	return scriptBinding{source: string(data), file: file}, nil
}

type scriptBinding struct {
	source string
	file   string
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
	spec = iso.rewriteRelative(spec)
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
		return iso.loadScript(spec, sb.source, sb.file)
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
		return nil, fmt.Errorf("workers: script binding %q is not a function", key)
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
