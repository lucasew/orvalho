package workers

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/dop251/goja"
)

// buildExecutionContextLocked builds the Workers executionContext (third arg to fetch).
// waitUntil is a stub: accepted and ignored (no post-response isolate work).
func (iso *Isolate) buildExecutionContextLocked() (*goja.Object, error) {
	ctx := iso.vm.NewObject()
	waitUntil := func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}
	if err := ctx.Set("waitUntil", waitUntil); err != nil {
		return nil, err
	}
	if err := ctx.Set("passThroughOnException", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}); err != nil {
		return nil, err
	}
	return ctx, nil
}

// buildEnvLocked constructs the guest env object for default.fetch.
// String keys come from Options.Env; object bindings from Options.Bindings.
// Name clash between a string and a binding is an error.
func (iso *Isolate) buildEnvLocked() (*goja.Object, error) {
	env := iso.vm.NewObject()
	seen := map[string]string{} // name -> "string" | "binding"

	for k, v := range iso.opts.Env {
		if k == "" {
			return nil, ErrEmptyEnvKey
		}
		if err := env.Set(k, v); err != nil {
			return nil, err
		}
		seen[k] = "string"
	}

	for name, b := range iso.opts.Bindings {
		if name == "" {
			return nil, ErrEmptyBindingName
		}
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("%w: %q (%s vs binding)", ErrEnvNameClash, name, prev)
		}
		obj, err := b.Materialize(iso)
		if err != nil {
			return nil, fmt.Errorf("workers: binding %q: %w", name, err)
		}
		if err := env.Set(name, obj); err != nil {
			return nil, err
		}
		seen[name] = "binding"
	}
	// cloudflare:workers stub reads globalThis.__orvalhoCFEnv (see bundle stubs).
	if err := iso.vm.Set("__orvalhoCFEnv", env); err != nil {
		return nil, err
	}
	return env, nil
}

// Binding is a factory for one guest JS object. The isolate calls
// Materialize the first time the object is needed (env on Fetch, or
// require of a specifier). Implementors must not touch host I/O except
// through values the embedder already put on the Binding.
type Binding interface {
	Materialize(iso *Isolate) (*goja.Object, error)
}

// AssetBinding is a CF-like static assets binding (env.NAME.fetch → Response)
// over a pluggable [fs.FS].
type AssetBinding struct {
	// FS is the file tree (required).
	FS fs.FS
	// Root is an optional subdirectory within FS (e.g. "assets").
	Root string
	// Paths, if non-empty, is an allowlist of FS paths relative to the FS root
	// (including Root prefix when set), slash-separated.
	Paths []string
}

// NewAssetBinding builds an [AssetBinding] over fsys.
// root is an optional prefix inside the FS; paths is an optional allowlist.
func NewAssetBinding(fsys fs.FS, root string, paths ...string) *AssetBinding {
	return &AssetBinding{FS: fsys, Root: root, Paths: paths}
}

func (a *AssetBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if a == nil || a.FS == nil {
		return nil, ErrAssetsMissingFS
	}
	root := strings.Trim(strings.ReplaceAll(a.Root, "\\", "/"), "/")
	allow := map[string]struct{}{}
	for _, p := range a.Paths {
		p = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "/")
		if p != "" {
			allow[p] = struct{}{}
		}
	}

	obj := iso.vm.NewObject()
	if err := obj.Set("fetch", func(call goja.FunctionCall) goja.Value {
		return iso.assetsFetch(call, a.FS, root, allow)
	}); err != nil {
		return nil, err
	}
	return obj, nil
}

func (iso *Isolate) assetsFetch(call goja.FunctionCall, fsys fs.FS, root string, allow map[string]struct{}) goja.Value {
	if len(call.Arguments) < 1 {
		panic(iso.vm.NewTypeError("ASSETS.fetch requires a Request, URL, or string"))
	}
	arg := call.Argument(0)
	method := "GET"
	urlPath := ""

	if obj, ok := arg.(*goja.Object); ok {
		if r, ok := iso.requestReg[obj]; ok {
			method = r.method
			urlPath = urlPathOnly(r.url)
		} else {
			urlPath = urlPathOnly(arg.String())
		}
	} else {
		urlPath = urlPathOnly(arg.String())
	}

	method = strings.ToUpper(method)
	if method != "GET" && method != "HEAD" {
		return iso.assetsResponse(405, "Method Not Allowed", "text/plain; charset=utf-8", "Method Not Allowed")
	}

	filePath, ok := assetsResolvePath(root, urlPath, allow)
	if !ok {
		return iso.assetsResponse(404, "Not Found", "text/plain; charset=utf-8", "Not Found")
	}
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return iso.assetsResponse(404, "Not Found", "text/plain; charset=utf-8", "Not Found")
	}
	if method == "HEAD" {
		return iso.assetsResponse(200, "OK", contentTypeForPath(filePath), "")
	}
	// Body as UTF-8 string; binary assets may corrupt — v1 OK for text/css etc.
	return iso.assetsResponse(200, "OK", contentTypeForPath(filePath), string(data))
}

func (iso *Isolate) assetsResponse(status int, statusText, contentType, body string) goja.Value {
	h := newHeaderBag()
	h.set("content-type", contentType)
	r := &responseBag{
		status:     status,
		statusText: statusText,
		headers:    h,
		body:       body,
	}
	ctor, ok := goja.AssertConstructor(iso.vm.Get("Response"))
	if !ok {
		panic("Response constructor missing")
	}
	obj, err := ctor(nil, iso.vm.ToValue(body))
	if err != nil {
		panic(err)
	}
	iso.responseReg[obj] = r
	return obj
}

func urlPathOnly(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			raw = rest[j:]
		} else {
			raw = "/"
		}
	}
	if q := strings.IndexAny(raw, "?#"); q >= 0 {
		raw = raw[:q]
	}
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw
}

func assetsResolvePath(root, urlPath string, allow map[string]struct{}) (string, bool) {
	urlPath = strings.TrimPrefix(urlPath, "/")
	if urlPath == "" || strings.HasSuffix(urlPath, "/") {
		return "", false
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == ".." || seg == "" {
			return "", false
		}
	}
	var filePath string
	if root == "" {
		filePath = urlPath
	} else {
		filePath = path.Join(root, urlPath)
	}
	filePath = path.Clean(filePath)
	if strings.HasPrefix(filePath, "..") {
		return "", false
	}
	if len(allow) > 0 {
		if _, ok := allow[filePath]; !ok {
			return "", false
		}
	}
	return filePath, true
}

func contentTypeForPath(p string) string {
	lower := strings.ToLower(p)
	switch {
	case strings.HasSuffix(lower, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(lower, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(lower, ".woff"):
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}
