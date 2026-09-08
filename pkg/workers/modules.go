package workers

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dop251/goja"
	"github.com/lucasew/orvalho/pkg/imports"
)

var cjsIdentDecl = regexp.MustCompile(`(?m)\b(?:var|let|const|function)\s+(__filename|__dirname)\b`)

func (iso *Isolate) installModules() {
	iso.moduleCache = make(map[string]goja.Value)
	iso.loading = make(map[string]*goja.Object)
	mustRuntimeSet(iso.vm, "require", iso.newRequire(""))
	mustRuntimeSet(iso.vm, "getBuiltinModule", iso.jsRequire)
}

// newRequire is require bound to from, with resolve (Node createRequire).
func (iso *Isolate) newRequire(from string) *goja.Object {
	fn := func(call goja.FunctionCall) goja.Value {
		prev := iso.importFrom
		if from != "" {
			iso.importFrom = from
		}
		defer func() { iso.importFrom = prev }()
		return iso.jsRequire(call)
	}
	obj := iso.vm.ToValue(fn).ToObject(iso.vm)
	mustSet(obj, "resolve", func(call goja.FunctionCall) goja.Value {
		return iso.jsRequireResolve(from, call)
	})
	main := iso.vm.NewObject()
	filename := from
	if filename == "" && len(iso.opts.Argv) > 1 {
		filename = iso.opts.Argv[1]
	}
	mustSet(main, "filename", filename)
	mustSet(obj, "main", main)
	return obj
}

func (iso *Isolate) jsRequireResolve(from string, call goja.FunctionCall) goja.Value {
	spec := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		spec = call.Argument(0).String()
	}
	resolveFrom := from
	if resolveFrom == "" {
		resolveFrom = iso.importFrom
	}
	if len(call.Arguments) > 1 {
		if p := firstResolvePath(call.Argument(1)); p != "" {
			resolveFrom = iso.guestResolveFrom(p)
		}
	}
	prev := iso.importFrom
	if resolveFrom != "" {
		iso.importFrom = resolveFrom
	}
	defer func() { iso.importFrom = prev }()

	lookup := iso.rewriteRelative(spec)
	v, err := imports.Resolve(lookup, withImportFrom(iso.opts.Imports, iso.importFrom)...)
	if err != nil {
		e := iso.vm.NewGoError(fmt.Errorf("%w: %q", ErrModuleNotFound, spec))
		_ = e.Set("code", "MODULE_NOT_FOUND")
		panic(e)
	}
	switch x := v.(type) {
	case imports.Script:
		if x.File != "" {
			return iso.vm.ToValue(x.File)
		}
	}
	return iso.vm.ToValue(spec)
}

func firstResolvePath(v goja.Value) string {
	obj, ok := v.(*goja.Object)
	if !ok {
		return ""
	}
	paths := obj.Get("paths")
	if paths == nil || goja.IsUndefined(paths) || goja.IsNull(paths) {
		return ""
	}
	o, ok := paths.(*goja.Object)
	if !ok {
		return ""
	}
	if n := o.Get("length"); n != nil && n.ToInteger() > 0 {
		item := o.Get("0")
		if item != nil && !goja.IsUndefined(item) && !goja.IsNull(item) {
			return item.String()
		}
	}
	return ""
}

func (iso *Isolate) guestResolveFrom(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if isFileHref(p) {
		p = fileURLToGuest(p)
	}
	p = strings.TrimLeft(p, "/")
	cwd := strings.ReplaceAll(iso.opts.Cwd, "\\", "/")
	cwd = strings.TrimLeft(cwd, "/")
	if cwd != "" {
		if p == cwd {
			return "."
		}
		if rest, ok := strings.CutPrefix(p, cwd+"/"); ok {
			if rest == "" {
				return "."
			}
			return rest
		}
	}
	if p == "" {
		return "."
	}
	return p
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

func (iso *Isolate) cachedExports(key string) (goja.Value, bool) {
	if mod, ok := iso.loading[key]; ok {
		return mod.Get("exports"), true
	}
	v, ok := iso.moduleCache[key]
	return v, ok
}

func (iso *Isolate) loadModule(spec string) (goja.Value, error) {
	spec = iso.rewriteRelative(spec)
	if v, ok := iso.cachedExports(spec); ok {
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
		// Same File is one module identity (path and path/posix).
		key := spec
		if x.File != "" {
			key = "file:" + x.File
		}
		if cached, ok := iso.cachedExports(key); ok {
			if _, loading := iso.loading[key]; !loading {
				iso.moduleCache[spec] = cached
			}
			return cached, nil
		}
		got, err := iso.loadScript(key, x.Source, x.File)
		if err != nil {
			return nil, err
		}
		iso.moduleCache[spec] = got
		return got, nil
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

// requireFrom is require bound to from, so a later callback still
// resolves relative specs against that file, not the caller.
func (iso *Isolate) requireFrom(from string) goja.Value {
	return iso.newRequire(from)
}

func (iso *Isolate) rewriteRelative(spec string) string {
	spec = stripSpecifierQuery(spec)
	if mapped, ok := iso.fileURLSpec(spec); ok {
		return mapped
	}
	if iso.importFrom == "" {
		return spec
	}
	if spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return path.Clean(path.Join(path.Dir(iso.importFrom), spec))
	}
	return spec
}

func (iso *Isolate) fileURLSpec(spec string) (string, bool) {
	if !isFileHref(spec) {
		return "", false
	}
	p := fileURLToGuest(spec)
	cwd := ""
	if iso != nil {
		cwd = iso.cwd
	}
	p = stripCwdPrefix(p, cwd)
	p = strings.TrimLeft(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return ".", true
	}
	return p, true
}

func (iso *Isolate) loadScript(key, source, file string) (goja.Value, error) {
	exports := iso.vm.NewObject()
	module := iso.vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	iso.moduleCache[key] = exports
	iso.loading[key] = module

	prev := iso.importFrom
	if file != "" {
		iso.importFrom = file
	}
	defer func() {
		iso.importFrom = prev
		delete(iso.loading, key)
	}()

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

	dir := "."
	if file != "" {
		dir = path.Dir(file)
		if dir == "" {
			dir = "."
		}
	}

	wrapped := wrapCJS(source)
	name := file
	if name == "" {
		name = "script.js"
	}
	v, err := runNamedScript(iso.vm, name, wrapped)
	if err != nil {
		delete(iso.moduleCache, key)
		return nil, err
	}
	fn, ok := goja.AssertFunction(v)
	if !ok {
		delete(iso.moduleCache, key)
		return nil, fmt.Errorf("workers: script %q is not a function", key)
	}
	_, err = fn(goja.Undefined(), iso.requireFrom(file), module, exports, iso.vm.ToValue(file), iso.vm.ToValue(dir))
	if err != nil {
		delete(iso.moduleCache, key)
		return nil, err
	}
	final := module.Get("exports")
	iso.moduleCache[key] = final
	return final, nil
}

func wrapCJS(source string) string {
	var b strings.Builder
	b.WriteString("(function (require, module, exports, __orvalhoFilename, __orvalhoDirname) {\n")
	b.WriteString("function __import(s){return Promise.resolve(require(s));}\n")
	b.WriteString("function __orvalhoFileURL(f){\n")
	b.WriteString("  if(!f) f = __orvalhoFilename;\n")
	b.WriteString("  if(!f) return 'file:///script.js';\n")
	b.WriteString("  f = String(f);\n")
	b.WriteString("  if(f.indexOf('file:')===0) return f;\n")
	b.WriteString("  f = f.replace(/\\\\/g,'/');\n")
	b.WriteString("  if(f.charAt(0)!=='/') f = '/'+f;\n")
	b.WriteString("  return 'file://'+f;\n")
	b.WriteString("}\n")
	found := map[string]bool{}
	for _, m := range cjsIdentDecl.FindAllStringSubmatch(source, -1) {
		if len(m) > 1 {
			found[m[1]] = true
		}
	}
	if !found["__filename"] {
		b.WriteString("var __filename = __orvalhoFilename;\n")
	}
	if !found["__dirname"] {
		b.WriteString("var __dirname = __orvalhoDirname;\n")
	}
	b.WriteString(source)
	b.WriteString("\n})")
	return b.String()
}

func runNamedScript(vm *goja.Runtime, name, src string) (v goja.Value, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("workers: parse %s: %v", name, rec)
		}
	}()
	return vm.RunScript(name, src)
}

// stripSpecifierQuery drops a URL query (Vite `?t=` cache bust) so
// svelte.config.js?t=1 looks up svelte.config.js. Hash imports (#foo) stay.
func stripSpecifierQuery(spec string) string {
	if i := strings.IndexByte(spec, '?'); i >= 0 {
		return spec[:i]
	}
	return spec
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
		if c == '.' || c == '?' || c == '_' || c == '$' ||
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
