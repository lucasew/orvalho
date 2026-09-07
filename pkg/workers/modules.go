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
		iso.noteScriptCause(err)
		panic(iso.vm.NewGoError(err))
	}
	return v
}

func (iso *Isolate) noteScriptCause(err error) {
	if err == nil || iso.scriptCause != nil {
		return
	}
	iso.scriptCause = err
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

type fromSetter interface {
	WithFrom(string) imports.Handler[any]
}

func withImportFrom(handlers []imports.Handler[any], from string) []imports.Handler[any] {
	if from == "" || len(handlers) == 0 {
		return handlers
	}
	out := make([]imports.Handler[any], len(handlers))
	for i, h := range handlers {
		if fs, ok := h.(fromSetter); ok {
			out[i] = fs.WithFrom(from)
			continue
		}
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

	source = stripShebang(source)
	if iso.opts.PrepareSource != nil {
		var err error
		source, err = iso.opts.PrepareSource(source, file)
		if err != nil {
			delete(iso.moduleCache, key)
			return nil, err
		}
	}
	source = rewriteImportToRequire(source)

	wrapped := "(function (require, module, exports) {\n" +
		"function __import(s){return Promise.resolve(require(s));}\n" +
		source + "\n})"
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

func stripShebang(src string) string {
	if !strings.HasPrefix(src, "#!") {
		return src
	}
	if i := strings.IndexByte(src, '\n'); i >= 0 {
		return src[i+1:]
	}
	return ""
}

// rewriteImportToRequire turns dynamic import( into __import( (Promise.resolve(require)).
func rewriteImportToRequire(src string) string {
	var b strings.Builder
	b.Grow(len(src) + 32)
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			j := i + 2
			for j < len(src) && src[j] != '\n' {
				j++
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 < len(src) {
				j += 2
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		if src[i] == '\'' || src[i] == '"' || src[i] == '`' {
			q := src[i]
			j := i + 1
			for j < len(src) {
				if src[j] == '\\' && j+1 < len(src) {
					j += 2
					continue
				}
				if src[j] == q {
					j++
					break
				}
				j++
			}
			b.WriteString(src[i:j])
			i = j
			continue
		}
		if n := importCallLen(src, i); n > 0 {
			b.WriteString("__import(")
			i += n
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func importCallLen(src string, i int) int {
	const kw = "import"
	if i+len(kw) > len(src) || src[i:i+len(kw)] != kw {
		return 0
	}
	if i > 0 {
		c := src[i-1]
		if c == '_' || c == '$' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return 0
		}
	}
	j := i + len(kw)
	for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
		j++
	}
	if j >= len(src) || src[j] != '(' {
		return 0
	}
	return j + 1 - i
}
