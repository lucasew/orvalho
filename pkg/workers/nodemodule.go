package workers

import (
	"path"
	"strings"

	"github.com/dop251/goja"
)

// nodeModuleBinding materializes guest require("module") / require("node:module").
type nodeModuleBinding struct{}

var _ Binding = nodeModuleBinding{}

func (nodeModuleBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"module", "node:module"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeModule(iso), nil
}

type nodeModule struct {
	iso *Isolate
}

func newNodeModule(iso *Isolate) *goja.Object {
	n := &nodeModule{iso: iso}
	obj := iso.vm.ToValue(n.constructor).ToObject(iso.vm)

	mustSet(obj, "Module", obj)
	mustSet(obj, "createRequire", n.jsCreateRequire)
	mustSet(obj, "isBuiltin", n.jsIsBuiltin)
	mustSet(obj, "builtinModules", append([]string(nil), nodeBuiltinModules...))
	return obj
}

func (n *nodeModule) constructor(call goja.ConstructorCall) *goja.Object {
	id := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		id = call.Argument(0).String()
	}
	mustSet(call.This, "id", id)
	mustSet(call.This, "exports", n.iso.vm.NewObject())
	mustSet(call.This, "filename", goja.Null())
	mustSet(call.This, "loaded", false)
	mustSet(call.This, "children", n.iso.vm.NewArray())
	dir := ""
	if id != "" {
		if i := strings.LastIndex(id, "/"); i >= 0 {
			dir = id[:i]
		}
	}
	mustSet(call.This, "path", dir)
	return nil
}

func (n *nodeModule) jsIsBuiltin(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return n.iso.vm.ToValue(false)
	}
	return n.iso.vm.ToValue(nodeIsBuiltin(call.Argument(0)))
}

func (n *nodeModule) jsCreateRequire(call goja.FunctionCall) goja.Value {
	filename, ok := createRequireFilename(call.Argument(0))
	if !ok {
		n.throwInvalidFilename(call.Argument(0))
	}
	if strings.HasSuffix(filename, "/") {
		filename += "noop.js"
	}
	return n.iso.newRequire(filename)
}

func (n *nodeModule) throwInvalidFilename(v goja.Value) {
	msg := "The argument 'filename' must be a file URL object, file URL string, or absolute path string. Received " + inspectCreateRequireArg(v)
	e := n.iso.vm.NewTypeError(msg)
	_ = e.Set("code", "ERR_INVALID_ARG_VALUE")
	panic(e)
}

func createRequireFilename(v goja.Value) (string, bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", false
	}
	if s, ok := exportString(v); ok {
		if isFileHref(s) {
			p := fileURLToGuest(s)
			if p == "" {
				return "", false
			}
			return p, true
		}
		if isAbsoluteFilename(s) {
			return absFilenameToGuest(s), true
		}
		// require.resolve returns guest-tree paths (no leading /).
		// Vite tsconfck then createRequire(that). Node wants absolute;
		// those strings are our module identity.
		if strings.Contains(s, "://") {
			return "", false
		}
		s = path.Clean(strings.ReplaceAll(s, "\\", "/"))
		if s == "" || s == "." || strings.HasPrefix(s, "..") {
			return "", false
		}
		return s, true
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return "", false
	}
	href := obj.Get("href")
	if href == nil || goja.IsUndefined(href) || goja.IsNull(href) {
		return "", false
	}
	h := href.String()
	if !isFileHref(h) {
		return "", false
	}
	p := fileURLToGuest(h)
	if p == "" {
		return "", false
	}
	return p, true
}

func exportString(v goja.Value) (string, bool) {
	switch x := v.Export().(type) {
	case string:
		return x, true
	}
	return "", false
}

func isAbsoluteFilename(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '/' || s[0] == '\\' {
		return true
	}
	if len(s) >= 3 && isDriveLetter(s[0]) && s[1] == ':' && (s[2] == '/' || s[2] == '\\') {
		return true
	}
	return false
}

func isDriveLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func absFilenameToGuest(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimLeft(s, "/")
	if s == "" {
		return "."
	}
	return s
}

func inspectCreateRequireArg(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	if s, ok := exportString(v); ok {
		return "'" + s + "'"
	}
	if obj, ok := v.(*goja.Object); ok && obj.ClassName() == "Object" && len(obj.Keys()) == 0 {
		return "{}"
	}
	return v.String()
}

func nodeIsBuiltin(v goja.Value) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	s, ok := exportString(v)
	if !ok {
		return false
	}
	if strings.HasPrefix(s, "node:") {
		return nodeBuiltinName(s[len("node:"):])
	}
	return nodeUnprefixedBuiltin[s]
}

func nodeBuiltinName(name string) bool {
	return nodeUnprefixedBuiltin[name] || nodePrefixOnlyBuiltin[name]
}

// nodeUnprefixedBuiltin is the Node public set that isBuiltin accepts
// with or without a "node:" prefix. http/sys stay listed even when unimplemented.
var nodeUnprefixedBuiltin = func() map[string]bool {
	m := make(map[string]bool, len(nodeBuiltinUnprefixed))
	for _, id := range nodeBuiltinUnprefixed {
		m[id] = true
	}
	return m
}()

// nodePrefixOnlyBuiltin is requireable only as node:NAME (isBuiltin("test") is false).
var nodePrefixOnlyBuiltin = map[string]bool{
	"sea":            true,
	"test":           true,
	"test/reporters": true,
}

// nodeBuiltinUnprefixed matches Node 24 lib/ public ids (no internal/).
var nodeBuiltinUnprefixed = []string{
	"_http_agent", "_http_client", "_http_common", "_http_incoming",
	"_http_outgoing", "_http_server",
	"_stream_duplex", "_stream_passthrough", "_stream_readable",
	"_stream_transform", "_stream_wrap", "_stream_writable",
	"_tls_common", "_tls_wrap",
	"assert", "assert/strict", "async_hooks", "buffer",
	"child_process", "cluster", "console", "constants", "crypto",
	"dgram", "diagnostics_channel", "dns", "dns/promises", "domain",
	"events", "fs", "fs/promises", "http", "http2", "https",
	"inspector", "inspector/promises", "module", "net", "os",
	"path", "path/posix", "path/win32", "perf_hooks", "process",
	"punycode", "querystring", "readline", "readline/promises",
	"repl", "stream", "stream/consumers", "stream/promises", "stream/web",
	"string_decoder", "sys", "timers", "timers/promises", "tls",
	"trace_events", "tty", "url", "util", "util/types", "v8",
	"vm", "wasi", "worker_threads", "zlib",
}

// nodeBuiltinModules is Module.builtinModules: unprefixed public names plus
// node: scheme-only ids. No internal/ names.
var nodeBuiltinModules = append(append([]string(nil), nodeBuiltinUnprefixed...),
	"node:sea", "node:test", "node:test/reporters")
