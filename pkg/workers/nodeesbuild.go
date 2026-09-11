package workers

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/lucasew/orvalho/pkg/imports"
)

// nodeEsbuildBinding is require("esbuild") backed by the linked Go API.
// The npm package wants a native @esbuild/<platform> binary; we never spawn it.
type nodeEsbuildBinding struct{}

var _ Binding = nodeEsbuildBinding{}

func (nodeEsbuildBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v, ok := iso.moduleCache["esbuild"]; ok {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	return newNodeEsbuild(iso), nil
}

type nodeEsbuild struct {
	iso *Isolate
}

func newNodeEsbuild(iso *Isolate) *goja.Object {
	n := &nodeEsbuild{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "version", apiVersion())
	mustSet(obj, "transform", n.jsTransform)
	mustSet(obj, "transformSync", n.jsTransformSync)
	mustSet(obj, "formatMessages", n.jsFormatMessages)
	mustSet(obj, "formatMessagesSync", n.jsFormatMessagesSync)
	mustSet(obj, "build", n.jsBuild)
	mustSet(obj, "buildSync", n.jsBuildSync)
	mustSet(obj, "context", n.jsContext)
	mustSet(obj, "default", obj)
	return obj
}

func apiVersion() string {
	// Match the module we compile against (go.mod).
	return "0.28.2"
}

func (n *nodeEsbuild) jsTransform(call goja.FunctionCall) goja.Value {
	return n.promiseTransform(call)
}

func (n *nodeEsbuild) jsTransformSync(call goja.FunctionCall) goja.Value {
	out, err := n.doTransform(call)
	if err != nil {
		panic(err)
	}
	return out
}

func (n *nodeEsbuild) promiseTransform(call goja.FunctionCall) goja.Value {
	p, resolve, reject := n.iso.vm.NewPromise()
	out, err := n.doTransform(call)
	if err != nil {
		reject(err)
	} else {
		resolve(out)
	}
	return n.iso.vm.ToValue(p)
}

func (n *nodeEsbuild) doTransform(call goja.FunctionCall) (goja.Value, goja.Value) {
	code := jsToString(call.Argument(0))
	opts := transformOpts(call.Argument(1))
	n.iso.trace("esbuild transform %dB", len(code))
	t0 := time.Now()
	res := api.Transform(code, opts)
	if len(res.Errors) > 0 {
		n.iso.trace("esbuild transform fail %s", time.Since(t0).Round(time.Millisecond))
		return nil, n.fail(res.Errors, res.Warnings)
	}
	n.iso.trace("esbuild transform ok %dB %s", len(res.Code), time.Since(t0).Round(time.Millisecond))
	return n.transformOK(res), nil
}

func (n *nodeEsbuild) transformOK(res api.TransformResult) goja.Value {
	o := n.iso.vm.NewObject()
	mustSet(o, "code", string(res.Code))
	mustSet(o, "map", string(res.Map))
	mustSet(o, "warnings", n.msgsValue(res.Warnings))
	return o
}

func (n *nodeEsbuild) jsFormatMessages(call goja.FunctionCall) goja.Value {
	p, resolve, reject := n.iso.vm.NewPromise()
	out, err := n.doFormat(call)
	if err != nil {
		reject(err)
	} else {
		resolve(out)
	}
	return n.iso.vm.ToValue(p)
}

func (n *nodeEsbuild) jsFormatMessagesSync(call goja.FunctionCall) goja.Value {
	out, err := n.doFormat(call)
	if err != nil {
		panic(err)
	}
	return out
}

func (n *nodeEsbuild) doFormat(call goja.FunctionCall) (goja.Value, goja.Value) {
	msgs := jsMessages(call.Argument(0))
	kind := api.WarningMessage
	if o, ok := call.Argument(1).(*goja.Object); ok {
		if jsToString(o.Get("kind")) == "error" {
			kind = api.ErrorMessage
		}
	}
	lines := api.FormatMessages(msgs, api.FormatMessagesOptions{Kind: kind})
	return n.iso.vm.ToValue(lines), nil
}

func (n *nodeEsbuild) jsBuild(call goja.FunctionCall) goja.Value {
	p, resolve, reject := n.iso.vm.NewPromise()
	out, err := n.doBuild(call)
	if err != nil {
		reject(err)
	} else {
		resolve(out)
	}
	return n.iso.vm.ToValue(p)
}

func (n *nodeEsbuild) jsBuildSync(call goja.FunctionCall) goja.Value {
	out, err := n.doBuild(call)
	if err != nil {
		panic(err)
	}
	return out
}

func (n *nodeEsbuild) doBuild(call goja.FunctionCall) (goja.Value, goja.Value) {
	opts := n.prepareBuild(call.Argument(0))
	n.iso.trace("esbuild build %s", strings.Join(opts.EntryPoints, " "))
	t0 := time.Now()
	res := api.Build(opts)
	if len(res.Errors) > 0 {
		n.iso.trace("esbuild build fail %s", time.Since(t0).Round(time.Millisecond))
		return nil, n.fail(res.Errors, res.Warnings)
	}
	n.iso.trace("esbuild build ok %s", time.Since(t0).Round(time.Millisecond))
	return n.buildOK(res), nil
}

func (n *nodeEsbuild) jsContext(call goja.FunctionCall) goja.Value {
	p, resolve, reject := n.iso.vm.NewPromise()
	opts := n.prepareBuild(call.Argument(0))
	n.iso.trace("esbuild context %s", strings.Join(opts.EntryPoints, " "))
	ctx, cerr := api.Context(opts)
	if cerr != nil {
		reject(n.fail(cerr.Errors, nil))
		return n.iso.vm.ToValue(p)
	}
	resolve(n.wrapContext(ctx))
	return n.iso.vm.ToValue(p)
}

func (n *nodeEsbuild) wrapContext(ctx api.BuildContext) goja.Value {
	o := n.iso.vm.NewObject()
	mustSet(o, "rebuild", func(goja.FunctionCall) goja.Value {
		p, resolve, reject := n.iso.vm.NewPromise()
		n.iso.trace("esbuild context rebuild")
		t0 := time.Now()
		res := ctx.Rebuild()
		if len(res.Errors) > 0 {
			msg := ""
			if len(res.Errors) > 0 {
				msg = res.Errors[0].Text
			}
			n.iso.trace("esbuild context rebuild fail %s %s", time.Since(t0).Round(time.Millisecond), msg)
			reject(n.fail(res.Errors, res.Warnings))
		} else {
			n.iso.trace("esbuild context rebuild ok %s files=%d", time.Since(t0).Round(time.Millisecond), len(res.OutputFiles))
			resolve(n.buildOK(res))
		}
		return n.iso.vm.ToValue(p)
	})
	mustSet(o, "cancel", func(goja.FunctionCall) goja.Value {
		p, resolve, _ := n.iso.vm.NewPromise()
		ctx.Cancel()
		resolve(goja.Undefined())
		return n.iso.vm.ToValue(p)
	})
	mustSet(o, "dispose", func(goja.FunctionCall) goja.Value {
		p, resolve, _ := n.iso.vm.NewPromise()
		ctx.Dispose()
		resolve(goja.Undefined())
		return n.iso.vm.ToValue(p)
	})
	return o
}

func (n *nodeEsbuild) prepareBuild(v goja.Value) api.BuildOptions {
	opts, plugins := n.buildOpts(v)
	// Guest JS plugins first so Vite can claim .svelte / flattened ids.
	opts.Plugins = append(opts.Plugins, plugins...)
	opts.Plugins = append(opts.Plugins, n.guestFSPlugin())
	n.iso.trace("esbuild prepare write=%v outdir=%s abs=%s entries=%d", opts.Write, opts.Outdir, opts.AbsWorkingDir, len(opts.EntryPoints))
	return opts
}

func (n *nodeEsbuild) buildOK(res api.BuildResult) goja.Value {
	o := n.iso.vm.NewObject()
	files := make([]any, 0, len(res.OutputFiles))
	for _, f := range res.OutputFiles {
		fo := n.iso.vm.NewObject()
		mustSet(fo, "path", f.Path)
		mustSet(fo, "text", string(f.Contents))
		mustSet(fo, "contents", f.Contents)
		files = append(files, fo)
	}
	mustSet(o, "outputFiles", n.iso.vm.ToValue(files))
	mustSet(o, "warnings", n.msgsValue(res.Warnings))
	mustSet(o, "errors", n.msgsValue(res.Errors))
	if res.Metafile != "" {
		var parsed any
		if err := json.Unmarshal([]byte(res.Metafile), &parsed); err == nil {
			mustSet(o, "metafile", parsed)
		} else {
			mustSet(o, "metafile", res.Metafile)
		}
	}
	return o
}

func (n *nodeEsbuild) guestFSPlugin() api.Plugin {
	return api.Plugin{
		Name: "orvalho-fs",
		Setup: func(b api.PluginBuild) {
			// Vite dep-scan must keep htmlLike stubs. The optimizeDeps
			// bundle (write+outdir) must leave .svelte on the host path
			// so vite-plugin-svelte:optimize can compile them.
			scanLike := !b.InitialOptions.Write || b.InitialOptions.Outdir == ""
			for _, p := range b.InitialOptions.Plugins {
				if p.Name == "vite:dep-scan" {
					scanLike = true
					break
				}
			}
			b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				p := args.Path
				if isNodeBuiltinSpec(p) || strings.HasPrefix(p, "node:") {
					return api.OnResolveResult{Path: p, External: true}, nil
				}
				if strings.Contains(p, "://") && !strings.HasPrefix(p, "file:") {
					return api.OnResolveResult{Path: p, External: true}, nil
				}
				rel := n.toGuest(p)
				if args.Importer != "" && (p == "." || p == ".." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../")) {
					rel = path.Join(path.Dir(n.toGuest(args.Importer)), p)
				}
				rel = path.Clean(rel)
				if n.guestFile(rel) != "" {
					if !scanLike {
						if hp := n.hostSveltePath(rel); hp != "" {
							return api.OnResolveResult{Path: hp}, nil
						}
					}
					return api.OnResolveResult{Path: rel, Namespace: "orvalho"}, nil
				}
				if !strings.HasPrefix(p, ".") && !path.IsAbs(p) {
					if resolved := n.resolveBare(p); resolved != "" {
						return api.OnResolveResult{Path: resolved, Namespace: "orvalho"}, nil
					}
				}
				if args.Kind == api.ResolveEntryPoint {
					// Flattened Vite ids are not files; stub them instead of marking external.
					return api.OnResolveResult{Path: p, Namespace: "orvalho"}, nil
				}
				return api.OnResolveResult{Path: p, External: true}, nil
			})
			b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "orvalho"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				file := n.guestFile(args.Path)
				if file == "" {
					empty := "export default function () {}\n"
					return api.OnLoadResult{Contents: &empty, Loader: api.LoaderJS}, nil
				}
				data, err := fs.ReadFile(n.iso.opts.FS, file)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				if js, ok := htmlLikeScanJS(string(data), file); ok {
					return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
				}
				s := string(data)
				return api.OnLoadResult{Contents: &s, Loader: loaderForPath(file)}, nil
			})
		},
	}
}

func svelteLike(p string) bool {
	p = strings.ToLower(p)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".svelte") || strings.HasSuffix(p, ".svelte.js") || strings.HasSuffix(p, ".svelte.ts")
}

// hostSveltePath is the host file for a guest .svelte id so Vite's
// vite-plugin-svelte:optimize OnLoad (readFileSync + svelte.compile) can run.
func (n *nodeEsbuild) hostSveltePath(guest string) string {
	if n == nil || n.iso == nil || !svelteLike(guest) {
		return ""
	}
	root := n.iso.opts.Cwd
	if root == "" {
		root = n.iso.cwd
	}
	if root == "" {
		return ""
	}
	full := filepath.Join(root, filepath.FromSlash(guest))
	if _, err := os.Stat(full); err != nil {
		return ""
	}
	return full
}

func (n *nodeEsbuild) toGuest(raw string) string {
	raw = strings.ReplaceAll(raw, "\\", "/")
	if strings.HasPrefix(raw, "file:") {
		raw = fileURLToGuest(raw)
	}
	cwd := ""
	mount := ""
	if n.iso != nil {
		cwd = n.iso.cwd
		mount = n.iso.opts.Cwd
	}
	p := stripCwdPrefix(raw, cwd)
	if mount != "" && mount != cwd {
		if alt := stripCwdPrefix(raw, mount); alt != raw && (p == raw || len(alt) < len(p)) {
			p = alt
		}
	}
	return strings.TrimPrefix(path.Clean(p), "/")
}

func (n *nodeEsbuild) guestFile(rel string) string {
	if n.iso == nil || n.iso.opts.FS == nil || rel == "" || rel == "." {
		return ""
	}
	rel = path.Clean(rel)
	for _, cand := range []string{rel, rel + ".js", rel + ".mjs", rel + ".cjs", rel + ".ts", path.Join(rel, "index.js")} {
		if st, err := fs.Stat(n.iso.opts.FS, cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

func (n *nodeEsbuild) fail(errs, warns []api.Message) goja.Value {
	e := n.iso.vm.NewGoError(ErrEsbuild)
	_ = e.Set("errors", n.msgsValue(errs))
	_ = e.Set("warnings", n.msgsValue(warns))
	if len(errs) > 0 {
		_ = e.Set("message", errs[0].Text)
	}
	return e
}

func (n *nodeEsbuild) msgsValue(msgs []api.Message) goja.Value {
	arr := make([]any, 0, len(msgs))
	for _, m := range msgs {
		arr = append(arr, n.msgObj(m))
	}
	return n.iso.vm.ToValue(arr)
}

func (n *nodeEsbuild) msgObj(m api.Message) *goja.Object {
	o := n.iso.vm.NewObject()
	mustSet(o, "text", m.Text)
	mustSet(o, "id", m.ID)
	mustSet(o, "pluginName", m.PluginName)
	if m.Location != nil {
		loc := n.iso.vm.NewObject()
		mustSet(loc, "file", m.Location.File)
		mustSet(loc, "line", m.Location.Line)
		mustSet(loc, "column", m.Location.Column)
		mustSet(loc, "lineText", m.Location.LineText)
		mustSet(o, "location", loc)
	} else {
		mustSet(o, "location", goja.Null())
	}
	return o
}

func transformOpts(v goja.Value) api.TransformOptions {
	opts := api.TransformOptions{LogLevel: api.LogLevelSilent}
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return opts
	}
	opts.Sourcefile = jsToString(o.Get("sourcefile"))
	opts.Loader = parseLoader(jsToString(o.Get("loader")))
	opts.Sourcemap = parseSourceMap(o.Get("sourcemap"))
	opts.Platform = parsePlatform(jsToString(o.Get("platform")))
	opts.Format = parseFormat(jsToString(o.Get("format")))
	opts.Charset = parseCharset(jsToString(o.Get("charset")))
	opts.Define = jsStringMap(o.Get("define"))
	opts.Target = parseTarget(o.Get("target"))
	if b, ok := jsBool(o.Get("minify")); ok && b {
		opts.MinifyWhitespace = true
		opts.MinifyIdentifiers = true
		opts.MinifySyntax = true
	}
	if b, ok := jsBool(o.Get("minifyWhitespace")); ok {
		opts.MinifyWhitespace = b
	}
	if b, ok := jsBool(o.Get("minifyIdentifiers")); ok {
		opts.MinifyIdentifiers = b
	}
	if b, ok := jsBool(o.Get("minifySyntax")); ok {
		opts.MinifySyntax = b
	}
	if raw := o.Get("tsconfigRaw"); raw != nil && !goja.IsUndefined(raw) && !goja.IsNull(raw) {
		switch t := raw.Export().(type) {
		case string:
			opts.TsconfigRaw = t
		default:
			if b, err := json.Marshal(t); err == nil {
				opts.TsconfigRaw = string(b)
			}
		}
	}
	return opts
}

func (n *nodeEsbuild) buildOpts(v goja.Value) (api.BuildOptions, []api.Plugin) {
	opts := api.BuildOptions{LogLevel: api.LogLevelSilent, Write: true}
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return opts, nil
	}
	opts.Sourcemap = parseSourceMap(o.Get("sourcemap"))
	opts.Platform = parsePlatform(jsToString(o.Get("platform")))
	opts.Format = parseFormat(jsToString(o.Get("format")))
	opts.Charset = parseCharset(jsToString(o.Get("charset")))
	opts.Define = jsStringMap(o.Get("define"))
	opts.Target = parseTarget(o.Get("target"))
	if b, ok := jsBool(o.Get("bundle")); ok {
		opts.Bundle = b
	}
	if b, ok := jsBool(o.Get("metafile")); ok {
		opts.Metafile = b
	}
	if b, ok := jsBool(o.Get("minify")); ok && b {
		opts.MinifyWhitespace = true
		opts.MinifyIdentifiers = true
		opts.MinifySyntax = true
	}
	opts.EntryPoints = jsStringSlice(o.Get("entryPoints"))
	opts.External = jsStringSlice(o.Get("external"))
	if s := jsToString(o.Get("absWorkingDir")); s != "" {
		opts.AbsWorkingDir = s
	}
	if s := jsToString(o.Get("sourceRoot")); s != "" {
		opts.SourceRoot = s
	}
	if s := jsToString(o.Get("outdir")); s != "" {
		opts.Outdir = s
	}
	if s := jsToString(o.Get("outfile")); s != "" {
		opts.Outfile = s
	}
	if b, ok := jsBool(o.Get("write")); ok {
		opts.Write = b
	}
	if b, ok := jsBool(o.Get("splitting")); ok {
		opts.Splitting = b
	}
	if b, ok := jsBool(o.Get("ignoreAnnotations")); ok {
		opts.IgnoreAnnotations = b
	}
	if b, ok := jsBool(o.Get("jsxDev")); ok {
		opts.JSXDev = b
	}
	if banner := jsStringMap(o.Get("banner")); len(banner) > 0 {
		opts.Banner = banner
	}
	opts.LegalComments = parseLegalComments(jsToString(o.Get("legalComments")))
	opts.LogLevel = parseLogLevel(jsToString(o.Get("logLevel")))
	if raw := o.Get("tsconfigRaw"); raw != nil && !goja.IsUndefined(raw) && !goja.IsNull(raw) {
		switch t := raw.Export().(type) {
		case string:
			opts.TsconfigRaw = t
		default:
			if b, err := json.Marshal(t); err == nil {
				opts.TsconfigRaw = string(b)
			}
		}
	}
	if stdin, ok := o.Get("stdin").(*goja.Object); ok && stdin != nil {
		opts.Stdin = &api.StdinOptions{
			Contents:   jsToString(stdin.Get("contents")),
			ResolveDir: jsToString(stdin.Get("resolveDir")),
			Sourcefile: jsToString(stdin.Get("sourcefile")),
			Loader:     parseLoader(jsToString(stdin.Get("loader"))),
		}
	}
	opts.MainFields = jsStringSlice(o.Get("mainFields"))
	if loaders := jsStringMap(o.Get("loader")); len(loaders) > 0 {
		opts.Loader = make(map[string]api.Loader, len(loaders))
		for ext, name := range loaders {
			opts.Loader[ext] = parseLoader(name)
		}
	}
	return opts, n.jsPlugins(o.Get("plugins"))
}

func (n *nodeEsbuild) jsPlugins(v goja.Value) []api.Plugin {
	arr, ok := v.(*goja.Object)
	if !ok || arr == nil {
		return nil
	}
	nlen := int(arr.Get("length").ToInteger())
	if nlen <= 0 {
		return nil
	}
	var out []api.Plugin
	for i := 0; i < nlen; i++ {
		item := arr.Get(strconv.Itoa(i))
		po, ok := item.(*goja.Object)
		if !ok {
			continue
		}
		setup, ok := goja.AssertFunction(po.Get("setup"))
		if !ok {
			continue
		}
		name := jsToString(po.Get("name"))
		stub := n.iso.vm.NewObject()
		mustSet(stub, "onResolve", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		mustSet(stub, "onLoad", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		mustSet(stub, "onStart", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		mustSet(stub, "onEnd", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		mustSet(stub, "initialOptions", n.iso.vm.NewObject())
		if _, err := setup(po, stub); err != nil {
			continue
		}
		// setup() runs on the isolate thread. Do not register JS onResolve/onLoad:
		// esbuild invokes those on worker goroutines and goja is not safe there.
		out = append(out, api.Plugin{Name: name, Setup: func(api.PluginBuild) {}})
	}
	return out
}

var htmlImportRE = regexp.MustCompile(`(?m)(?:^|[;\s])import\s+(?:type\s+)?(?:[\w*{}\s,]+from\s+)?["']([^"']+)["']`)

func htmlLikeScanJS(src, file string) (string, bool) {
	switch strings.ToLower(path.Ext(file)) {
	case ".svelte", ".vue", ".astro", ".html", ".imba":
	default:
		return "", false
	}
	var b strings.Builder
	for _, m := range htmlImportRE.FindAllStringSubmatch(src, -1) {
		if len(m) > 1 && m[1] != "" {
			b.WriteString("import ")
			b.WriteByte('"')
			b.WriteString(m[1])
			b.WriteString("\"\n")
		}
	}
	b.WriteString("export default function () {}\n")
	return b.String(), true
}

func (n *nodeEsbuild) resolveBare(spec string) string {
	if n.iso == nil || spec == "" {
		return ""
	}
	v, err := imports.Resolve(spec, withImportFrom(n.iso.opts.Imports, n.iso.importFrom)...)
	if err != nil {
		return ""
	}
	s, ok := v.(imports.Script)
	if !ok || s.File == "" {
		return ""
	}
	rel := n.toGuest(s.File)
	if n.guestFile(rel) != "" {
		return rel
	}
	return ""
}

func jsMessages(v goja.Value) []api.Message {
	arr, ok := v.(*goja.Object)
	if !ok || arr == nil {
		return nil
	}
	nlen := int(arr.Get("length").ToInteger())
	var msgs []api.Message
	for i := 0; i < nlen; i++ {
		o, ok := arr.Get(strconv.Itoa(i)).(*goja.Object)
		if !ok {
			continue
		}
		m := api.Message{Text: jsToString(o.Get("text"))}
		if loc, ok := o.Get("location").(*goja.Object); ok && loc != nil {
			line, col := 0, 0
			if v := loc.Get("line"); v != nil && !goja.IsUndefined(v) {
				line = int(v.ToInteger())
			}
			if v := loc.Get("column"); v != nil && !goja.IsUndefined(v) {
				col = int(v.ToInteger())
			}
			m.Location = &api.Location{
				File:     jsToString(loc.Get("file")),
				Line:     line,
				Column:   col,
				LineText: jsToString(loc.Get("lineText")),
			}
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func jsRegexpSource(v goja.Value) string {
	if o, ok := v.(*goja.Object); ok && o != nil {
		if s := jsToString(o.Get("source")); s != "" {
			return s
		}
	}
	return jsToString(v)
}

func jsStringMap(v goja.Value) map[string]string {
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range o.Keys() {
		out[k] = jsToString(o.Get(k))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func jsStringSlice(v goja.Value) []string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	if s := jsToString(v); s != "" && s != "[object Object]" && s != "[object Array]" {
		if _, isObj := v.(*goja.Object); !isObj || v.Export() == s {
			return []string{s}
		}
	}
	o, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	if ln := o.Get("length"); ln != nil && !goja.IsUndefined(ln) {
		nlen := int(ln.ToInteger())
		out := make([]string, 0, nlen)
		for i := 0; i < nlen; i++ {
			out = append(out, jsToString(o.Get(strconv.Itoa(i))))
		}
		return out
	}
	return nil
}

func jsBool(v goja.Value) (bool, bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false, false
	}
	if b, ok := v.Export().(bool); ok {
		return b, true
	}
	return false, false
}

func parseLoader(s string) api.Loader {
	switch strings.ToLower(s) {
	case "js":
		return api.LoaderJS
	case "jsx":
		return api.LoaderJSX
	case "ts":
		return api.LoaderTS
	case "tsx":
		return api.LoaderTSX
	case "css":
		return api.LoaderCSS
	case "json":
		return api.LoaderJSON
	case "text":
		return api.LoaderText
	case "file":
		return api.LoaderFile
	case "dataurl":
		return api.LoaderDataURL
	default:
		return api.LoaderDefault
	}
}

func parseSourceMap(v goja.Value) api.SourceMap {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return api.SourceMapNone
	}
	if b, ok := v.Export().(bool); ok {
		if b {
			return api.SourceMapExternal
		}
		return api.SourceMapNone
	}
	switch jsToString(v) {
	case "inline":
		return api.SourceMapInline
	case "both":
		return api.SourceMapInlineAndExternal
	case "external":
		return api.SourceMapExternal
	case "linked":
		return api.SourceMapLinked
	default:
		return api.SourceMapNone
	}
}

func parsePlatform(s string) api.Platform {
	switch s {
	case "node":
		return api.PlatformNode
	case "browser":
		return api.PlatformBrowser
	case "neutral":
		return api.PlatformNeutral
	default:
		return api.PlatformDefault
	}
}

func parseFormat(s string) api.Format {
	switch s {
	case "esm", "esmodule":
		return api.FormatESModule
	case "cjs", "commonjs":
		return api.FormatCommonJS
	case "iife":
		return api.FormatIIFE
	default:
		return api.FormatDefault
	}
}

func parseLegalComments(s string) api.LegalComments {
	switch s {
	case "none":
		return api.LegalCommentsNone
	case "inline":
		return api.LegalCommentsInline
	case "eof":
		return api.LegalCommentsEndOfFile
	case "linked":
		return api.LegalCommentsLinked
	case "external":
		return api.LegalCommentsExternal
	default:
		return api.LegalCommentsDefault
	}
}

func parseLogLevel(s string) api.LogLevel {
	switch s {
	case "verbose":
		return api.LogLevelVerbose
	case "debug":
		return api.LogLevelDebug
	case "info":
		return api.LogLevelInfo
	case "warning":
		return api.LogLevelWarning
	case "error":
		return api.LogLevelError
	case "silent":
		return api.LogLevelSilent
	default:
		return api.LogLevelSilent
	}
}

func parseCharset(s string) api.Charset {
	switch strings.ToLower(s) {
	case "ascii":
		return api.CharsetASCII
	case "utf8", "utf-8":
		return api.CharsetUTF8
	default:
		return api.CharsetDefault
	}
}

func parseTarget(v goja.Value) api.Target {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return api.DefaultTarget
	}
	s := jsToString(v)
	if sl := jsStringSlice(v); len(sl) > 0 {
		s = sl[0]
	}
	s = strings.ToLower(s)
	switch {
	case s == "esnext":
		return api.ESNext
	case s == "es5":
		return api.ES5
	case strings.HasPrefix(s, "es2025"):
		return api.ES2025
	case strings.HasPrefix(s, "es2024"):
		return api.ES2024
	case strings.HasPrefix(s, "es2023"):
		return api.ES2023
	case strings.HasPrefix(s, "es2022"):
		return api.ES2022
	case strings.HasPrefix(s, "es2021"):
		return api.ES2021
	case strings.HasPrefix(s, "es2020"):
		return api.ES2020
	case strings.HasPrefix(s, "es2019"):
		return api.ES2019
	case strings.HasPrefix(s, "es2018"):
		return api.ES2018
	case strings.HasPrefix(s, "es2017"):
		return api.ES2017
	case strings.HasPrefix(s, "es2016"):
		return api.ES2016
	case strings.HasPrefix(s, "es2015"), s == "es6":
		return api.ES2015
	case strings.HasPrefix(s, "node"):
		return api.ES2020
	default:
		return api.DefaultTarget
	}
}

func loaderForPath(p string) api.Loader {
	switch strings.ToLower(path.Ext(p)) {
	case ".ts":
		return api.LoaderTS
	case ".tsx":
		return api.LoaderTSX
	case ".jsx":
		return api.LoaderJSX
	case ".css":
		return api.LoaderCSS
	case ".json":
		return api.LoaderJSON
	case ".mjs", ".cjs", ".js":
		return api.LoaderJS
	default:
		return api.LoaderJS
	}
}

func isNodeBuiltinSpec(p string) bool {
	p = strings.TrimPrefix(p, "node:")
	for _, b := range nodeBuiltinUnprefixed {
		if p == b {
			return true
		}
	}
	return false
}
