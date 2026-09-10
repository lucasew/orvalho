package workers

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func (iso *Isolate) installEvalHook() {
	mustRuntimeSet(iso.vm, "__orvalhoPrepareEval", iso.jsPrepareEval)
	mustRuntimeSet(iso.vm, "__orvalhoDownlevel", iso.jsDownlevel)
	mustRuntimeSet(iso.vm, "__orvalhoNeedsEvalTransform", iso.jsNeedsEvalTransform)
	if _, err := iso.vm.RunString(evalHookScript); err != nil {
		panic(err)
	}
}

func (iso *Isolate) jsPrepareEval(call goja.FunctionCall) goja.Value {
	src := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		src = call.Argument(0).String()
	}
	asFn := false
	if len(call.Arguments) > 1 {
		asFn = call.Argument(1).ToBoolean()
	}
	return iso.vm.ToValue(iso.prepareEvalSource(src, asFn))
}

func (iso *Isolate) jsDownlevel(call goja.FunctionCall) goja.Value {
	src := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		src = call.Argument(0).String()
	}
	return iso.vm.ToValue(iso.downlevelEvalWrapped(src))
}

func (iso *Isolate) jsNeedsEvalTransform(call goja.FunctionCall) goja.Value {
	src := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		src = call.Argument(0).String()
	}
	if looksLikeESM(src) {
		return iso.vm.ToValue(true)
	}
	isAsync := false
	if len(call.Arguments) > 1 {
		isAsync = call.Argument(1).ToBoolean()
	}
	if isAsync {
		// goja already runs await, #fields, ?., ??, rest/spread, and
		// optional catch. Rewriting those to ES2015 (__async +
		// __privateAdd) is what threw "same private member more than
		// once" on Svelte's Renderer during Vite SSR.
		return iso.vm.ToValue(needsExoticSyntax(src))
	}
	return iso.vm.ToValue(needsDownlevel(src))
}

const orvalhoEvalPrefix = "async function __orvalhoEval("

func (iso *Isolate) downlevelEvalWrapped(src string) string {
	body, args, ok := splitOrvalhoEval(src)
	if !ok {
		if looksLikeESM(src) {
			return iso.prepareEvalSource(src, false)
		}
		if needsDownlevel(src) {
			return downlevelSyntax(src)
		}
		return src
	}
	if looksLikeESM(body) {
		prepared := iso.prepareEvalSource(body, true)
		return "function __orvalhoEval(" + args + ") {\n" + prepared + "\n}"
	}
	if needsDownlevel(body) {
		return downlevelSyntax(src)
	}
	return src
}

func splitOrvalhoEval(src string) (body, args string, ok bool) {
	for _, prefix := range []string{orvalhoEvalPrefix, "function __orvalhoEval("} {
		if !strings.HasPrefix(src, prefix) {
			continue
		}
		rest := src[len(prefix):]
		idx := strings.Index(rest, ") {\n")
		if idx < 0 {
			continue
		}
		args = rest[:idx]
		body = strings.TrimSuffix(rest[idx+len(") {\n"):], "\n}")
		return body, args, true
	}
	return "", "", false
}

func (iso *Isolate) prepareEvalSource(src string, asFunction bool) string {
	if !looksLikeESM(src) {
		if asFunction && needsDownlevel(src) {
			out, err := downlevelAwaitBody(src)
			if err != nil {
				panic(iso.vm.NewGoError(err))
			}
			return out
		}
		return src
	}
	out, err := bundle.TransformEvalCJS(src, evalSourceName)
	if err != nil {
		panic(iso.vm.NewGoError(err))
	}
	return wrapEvalCJS(out, asFunction)
}

func downlevelAwaitBody(src string) (string, error) {
	out, err := downlevelEval("async function __orvalhoEval() {\n" + src + "\n}")
	if err != nil {
		return "", err
	}
	out = strings.Replace(out, "function __orvalhoEval() {", "", 1)
	if i := strings.LastIndex(out, "}"); i >= 0 {
		out = out[:i]
	}
	return out, nil
}

func downlevelEval(src string) (string, error) {
	r := api.Transform(src, api.TransformOptions{
		Loader:  api.LoaderJS,
		Target:  api.ES2015,
		Format:  api.FormatDefault,
		Charset: api.CharsetASCII,
	})
	if len(r.Errors) > 0 {
		return "", fmt.Errorf("downlevel: %s", r.Errors[0].Text)
	}
	return string(r.Code), nil
}

func downlevelSyntax(src string) string {
	r := api.Transform(src, api.TransformOptions{
		Loader:  api.LoaderJS,
		Target:  api.ES2015,
		Format:  api.FormatDefault,
		Charset: api.CharsetASCII,
	})
	if len(r.Errors) == 0 {
		return string(r.Code)
	}
	if strings.HasPrefix(src, "function __orvalhoEval(") {
		return downlevelSyntax("async " + src)
	}
	return src
}

func needsDownlevel(src string) bool {
	if strings.Contains(src, "await ") || strings.Contains(src, "await\t") ||
		strings.Contains(src, "await\n") || strings.Contains(src, "await(") {
		return true
	}
	return needsExoticSyntax(src)
}

func needsExoticSyntax(src string) bool {
	if strings.Contains(src, " using ") || strings.Contains(src, "\tusing ") ||
		strings.Contains(src, "\nusing ") || strings.HasPrefix(src, "using ") {
		return true
	}
	if strings.Contains(src, " with {") || strings.Contains(src, " with{") {
		return true
	}
	return false
}

const evalSourceName = "eval.mjs"

func wrapEvalCJS(source string, asFunction bool) string {
	async := strings.Contains(source, "await")
	var b strings.Builder
	if asFunction {
		b.WriteString("return ")
	}
	if async {
		b.WriteString("(async function () {\n")
	} else {
		b.WriteString("(function () {\n")
	}
	b.WriteString("var module = { exports: {} };\n")
	b.WriteString("var exports = module.exports;\n")
	b.WriteString("var __file = (typeof __vite_ssr_import_meta__ === \"object\" && __vite_ssr_import_meta__ && __vite_ssr_import_meta__.url) ? String(__vite_ssr_import_meta__.url) : (typeof __filename !== \"undefined\" ? __filename : \"\");\n")
	b.WriteString("var __dir = \".\";\n")
	b.WriteString("if (__file) {\n")
	b.WriteString("  var __p = __file;\n")
	b.WriteString("  if (__p.indexOf(\"file:\") === 0) {\n")
	b.WriteString("    __p = __p.slice(7);\n")
	b.WriteString("    if (__p.charAt(0) !== \"/\") __p = \"/\" + __p;\n")
	b.WriteString("  }\n")
	b.WriteString("  var __slash = __p.lastIndexOf(\"/\");\n")
	b.WriteString("  __dir = __slash >= 0 ? __p.slice(0, __slash) : \".\";\n")
	b.WriteString("}\n")
	b.WriteString("var __req = typeof require === \"function\" ? require : function (s) { throw new Error(\"require \" + s); };\n")
	if async {
		b.WriteString("await ")
		b.WriteString(wrapCJSFn(source, true))
	} else {
		b.WriteString(wrapCJS(source))
	}
	b.WriteString("(__req, module, exports, __file, __dir);\n")
	b.WriteString("if (typeof __vite_ssr_exportName__ === \"function\") {\n")
	b.WriteString("  var __e = module.exports;\n")
	b.WriteString("  if (__e && (typeof __e === \"object\" || typeof __e === \"function\")) {\n")
	b.WriteString("    var __names = Object.getOwnPropertyNames(__e);\n")
	b.WriteString("    for (var __i = 0; __i < __names.length; __i++) {\n")
	b.WriteString("      (function (k) { __vite_ssr_exportName__(k, function () { return __e[k]; }); })(__names[__i]);\n")
	b.WriteString("    }\n")
	b.WriteString("  } else {\n")
	b.WriteString("    __vite_ssr_exportName__(\"default\", function () { return __e; });\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	b.WriteString("return module.exports;\n")
	b.WriteString("})()")
	return b.String()
}

func looksLikeESM(src string) bool {
	i := 0
	for i < len(src) {
		if n := evalCommentLen(src, i); n > 0 {
			i += n
			continue
		}
		if n := evalStringLen(src, i); n > 0 {
			i += n
			continue
		}
		if isEvalIdentStart(src[i]) {
			j := i + 1
			for j < len(src) && isEvalIdentPart(src[j]) {
				j++
			}
			w := src[i:j]
			if (w == "import" || w == "export") && !evalIdentBefore(src, i) {
				return true
			}
			i = j
			continue
		}
		i++
	}
	return false
}

func evalIdentBefore(src string, i int) bool {
	if i == 0 {
		return false
	}
	c := src[i-1]
	return isEvalIdentPart(c) || c == '.' || c == '?'
}

func isEvalIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isEvalIdentPart(c byte) bool {
	return isEvalIdentStart(c) || (c >= '0' && c <= '9')
}

func evalCommentLen(src string, i int) int {
	if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
		j := i + 2
		for j < len(src) && src[j] != '\n' {
			j++
		}
		return j - i
	}
	if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
		j := i + 2
		for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
			j++
		}
		if j+1 < len(src) {
			j += 2
		}
		return j - i
	}
	return 0
}

func evalStringLen(src string, i int) int {
	if src[i] != '\'' && src[i] != '"' && src[i] != '`' {
		return 0
	}
	q := src[i]
	j := i + 1
	for j < len(src) {
		if src[j] == '\\' && j+1 < len(src) {
			j += 2
			continue
		}
		if src[j] == q {
			return j + 1 - i
		}
		j++
	}
	return j - i
}

// evalHookScript wraps eval / Function / AsyncFunction so ESM and
// newer syntax are compiled before goja parses them.
const evalHookScript = `
(function () {
  var prepHost = globalThis.__orvalhoPrepareEval;
  var downHost = globalThis.__orvalhoDownlevel;
  var needHost = globalThis.__orvalhoNeedsEvalTransform;
  var origEval = eval;
  var origFunction = Function;
  var AsyncFunction = (async function () {}).constructor;
  function wrapCtor(Orig, isAsync) {
    function Wrapped() {
      var args = [];
      for (var i = 0; i < arguments.length; i++) args[i] = arguments[i];
      if (!args.length) return Orig.apply(this, args);
      var body = String(args[args.length - 1]);
      if (needHost(body, isAsync)) {
        var names = [];
        for (var j = 0; j < args.length - 1; j++) names.push(String(args[j]));
        var src = "async function __orvalhoEval(" + names.join(",") + ") {\n" + body + "\n}";
        var out = downHost(src);
        return origEval("(function () {\n" + out + "\nreturn __orvalhoEval;\n})()");
      }
      return Orig.apply(this, args);
    }
    try { Object.defineProperty(Wrapped, "name", { value: Orig.name, configurable: true }); } catch (e) {}
    Wrapped.prototype = Orig.prototype;
    return Wrapped;
  }
  function wrapEval(code) { return origEval(prepHost(String(code), false)); }
  var WrappedFunction = wrapCtor(origFunction, false);
  var WrappedAsync = wrapCtor(AsyncFunction, true);
  globalThis.eval = wrapEval;
  globalThis.Function = WrappedFunction;
  try {
    Object.defineProperty(origFunction.prototype, "constructor", { value: WrappedFunction, writable: true, configurable: true });
  } catch (e) { origFunction.prototype.constructor = WrappedFunction; }
  try {
    Object.defineProperty(AsyncFunction.prototype, "constructor", { value: WrappedAsync, writable: true, configurable: true });
  } catch (e) { AsyncFunction.prototype.constructor = WrappedAsync; }
})();
`
