package js

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// buildExecutionContextLocked builds the Workers executionContext (third arg to fetch).
// Astro/CF adapters call context.waitUntil.bind(context).
func (iso *Isolate) buildExecutionContextLocked() (*goja.Object, error) {
	ctx := iso.vm.NewObject()
	// waitUntil(promise) — fire-and-forget; we do not track for v1 serve.
	waitUntil := func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}
	if err := ctx.Set("waitUntil", waitUntil); err != nil {
		return nil, err
	}
	// passThroughOnException is a no-op on our host.
	if err := ctx.Set("passThroughOnException", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}); err != nil {
		return nil, err
	}
	return ctx, nil
}

// buildEnvLocked constructs the guest env object for default.fetch.
// String keys come from Options.Env; object bindings from Options.Bindings.
// Name clash between a string and a binding is an error (never-allocate at serve).
func (iso *Isolate) buildEnvLocked() (*goja.Object, error) {
	env := iso.vm.NewObject()
	seen := map[string]string{} // name -> "string" | "binding"

	for k, v := range iso.opts.Env {
		if k == "" {
			return nil, fmt.Errorf("js: empty env string key")
		}
		if err := env.Set(k, v); err != nil {
			return nil, err
		}
		seen[k] = "string"
	}

	for name, b := range iso.opts.Bindings {
		if name == "" {
			return nil, fmt.Errorf("js: empty binding name")
		}
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("js: env name %q clashes (%s vs binding)", name, prev)
		}
		obj, err := b.Materialize(iso)
		if err != nil {
			return nil, fmt.Errorf("js: binding %q: %w", name, err)
		}
		if err := env.Set(name, obj); err != nil {
			return nil, err
		}
		seen[name] = "binding"
	}
	// cloudflare:workers stub reads globalThis.__orvalhoCFEnv (see stubs/cloudflare_workers.js).
	if err := iso.vm.Set("__orvalhoCFEnv", env); err != nil {
		return nil, err
	}
	return env, nil
}

// Binding is a host-side env object factory (driver registry materialize result).
type Binding interface {
	Materialize(iso *Isolate) (*goja.Object, error)
}

// AssetBinding is a CF-like static assets binding (env.NAME.fetch → Response).
type AssetBinding struct {
	// Root is the package-relative directory prefix (e.g. "assets").
	Root string
	// Paths, if non-empty, is an allowlist of package-relative paths.
	Paths []string
	// Read returns file bytes for a package-relative path.
	Read func(path string) ([]byte, bool)
}

func (a *AssetBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if a == nil || a.Read == nil {
		return nil, fmt.Errorf("assets: missing Read")
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
	fetchFn := func(call goja.FunctionCall) goja.Value {
		return iso.assetsFetch(call, root, allow, a.Read)
	}
	if err := obj.Set("fetch", fetchFn); err != nil {
		return nil, err
	}
	return obj, nil
}

func (iso *Isolate) assetsFetch(call goja.FunctionCall, root string, allow map[string]struct{}, read func(string) ([]byte, bool)) goja.Value {
	if len(call.Arguments) < 1 {
		panic(iso.vm.NewTypeError("ASSETS.fetch requires a Request, URL, or string"))
	}
	arg := call.Argument(0)
	method := "GET"
	path := ""

	if obj, ok := arg.(*goja.Object); ok {
		if r, ok := iso.requestReg[obj]; ok {
			method = r.method
			path = urlPathOnly(r.url)
		} else {
			// URL object or plain object with href/pathname — treat as string.
			path = urlPathOnly(arg.String())
		}
	} else {
		path = urlPathOnly(arg.String())
	}

	method = strings.ToUpper(method)
	if method != "GET" && method != "HEAD" {
		return iso.assetsResponse(405, "Method Not Allowed", "text/plain; charset=utf-8", "Method Not Allowed")
	}

	filePath, ok := assetsResolvePath(root, path, allow)
	if !ok {
		return iso.assetsResponse(404, "Not Found", "text/plain; charset=utf-8", "Not Found")
	}
	data, found := read(filePath)
	if !found {
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
	// Strip scheme://host if present.
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
		// Directory index not implemented; 404.
		return "", false
	}
	// Reject .. and absolute escapes.
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == ".." || seg == "" {
			return "", false
		}
	}
	var filePath string
	if root == "" {
		filePath = urlPath
	} else {
		filePath = root + "/" + urlPath
	}
	filePath = strings.TrimPrefix(filePath, "/")
	if len(allow) > 0 {
		if _, ok := allow[filePath]; !ok {
			return "", false
		}
	}
	return filePath, true
}

func contentTypeForPath(path string) string {
	lower := strings.ToLower(path)
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
