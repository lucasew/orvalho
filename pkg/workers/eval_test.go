package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestEvalESMExportDefault(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var got = eval("export default 3;");
		if (!got || got.default !== 3) throw new Error("eval " + (got && got.default));
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFunctionPlainBodyUnchanged(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		var f = new Function("a", "return a + 1");
		if (f(6) !== 7) throw new Error("fn " + f(6));
		if (eval("1 + 1") !== 2) throw new Error("eval");
		var src = new Function("/*code*/").toString();
		if (src.indexOf("/*code*/") < 0) throw new Error("toString " + src);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsyncFunctionESMViteShape(t *testing.T) {
	iso := New("", Options{
		Imports: importMap(map[string]any{
			"orvalho:n": imports.Script{Source: `exports.n = 4;`},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var AsyncFunction = async function () {}.constructor;
		var exports = {};
		function exportName(k, get) {
			Object.defineProperty(exports, k, { enumerable: true, configurable: true, get: get });
		}
		var fn = new AsyncFunction(
			"__vite_ssr_exports__",
			"__vite_ssr_import_meta__",
			"__vite_ssr_import__",
			"__vite_ssr_dynamic_import__",
			"__vite_ssr_exportAll__",
			"__vite_ssr_exportName__",
			"\"use strict\";\nimport { n } from \"orvalho:n\";\nexport default n + 1;\n"
		);
		var n = 0;
		Promise.resolve(fn(exports, { url: "file:///t.mjs" }, null, null, null, exportName)).then(function () {
			n = exports.default;
		});
		setTimeout(function () {
			if (n !== 5) throw new Error("default " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDownlevelEvalWrappedESM(t *testing.T) {
	iso := New("", Options{
		Imports: importMap(map[string]any{
			"orvalho:n": imports.Script{Source: `exports.n = 4;`},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var body = '"use strict";\n' + 'import { n } from "orvalho:n";\n' + "export default n + 1;\n";
		var src = "async function __orvalhoEval(__vite_ssr_exports__, __vite_ssr_exportName__) {\n" + body + "}";
		var out = __orvalhoDownlevel(src);
		var fn = (new Function(out + "\nreturn __orvalhoEval;"))();
		var exports = {};
		function exportName(k, get) {
			Object.defineProperty(exports, k, { enumerable: true, configurable: true, get: get });
		}
		fn(exports, exportName);
		if (exports.default !== 5) throw new Error("default " + exports.default);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFunctionAwaitBody(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		var fn = new Function("x", "return await x;");
		var n = 0;
		Promise.resolve(fn(Promise.resolve(8))).then(function (v) { n = v; });
		setTimeout(function () {
			if (n !== 8) throw new Error("await " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsyncFunctionViteSSRBody(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		var AsyncFunction = async function () {}.constructor;
		var exports = {};
		function exportName(k, get) {
			Object.defineProperty(exports, k, { enumerable: true, configurable: true, get: get });
		}
		async function ssrImport() { return { n: 4 }; }
		var fn = new AsyncFunction(
			"__vite_ssr_exports__",
			"__vite_ssr_import_meta__",
			"__vite_ssr_import__",
			"__vite_ssr_exportName__",
			'"use strict";__vite_ssr_exportName__("default", () => { try { return __vite_ssr_export_default__ } catch {} });\n' +
			'const __vite_ssr_import_0__ = await __vite_ssr_import__("orvalho:n");\n' +
			'const extra = { ...{ a: 1 } };\n' +
			'const __vite_ssr_export_default__ = __vite_ssr_import_0__.n + extra.a;\n'
		);
		var n = 0;
		Promise.resolve(fn(exports, { url: "file:///t.mjs" }, ssrImport, exportName)).then(function () {
			n = exports.default;
		});
		setTimeout(function () {
			if (n !== 5) throw new Error("default " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsyncFunctionESMTopLevelAwait(t *testing.T) {
	iso := New("", Options{
		Imports: importMap(map[string]any{
			"orvalho:n": imports.Script{Source: `exports.n = 4;`},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var AsyncFunction = async function () {}.constructor;
		var exports = {};
		function exportName(k, get) {
			Object.defineProperty(exports, k, { enumerable: true, configurable: true, get: get });
		}
		var fn = new AsyncFunction(
			"__vite_ssr_exports__",
			"__vite_ssr_import_meta__",
			"__vite_ssr_import__",
			"__vite_ssr_dynamic_import__",
			"__vite_ssr_exportAll__",
			"__vite_ssr_exportName__",
			"\"use strict\";\nimport { n } from \"orvalho:n\";\nexport default await Promise.resolve(n + 1);\n"
		);
		var got;
		Promise.resolve(fn(exports, { url: "file:///t.mjs" }, null, null, null, exportName)).then(function () {
			got = exports.default;
		});
		setTimeout(function () {
			if (got !== 5) throw new Error("default " + got);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestLooksLikeESM(t *testing.T) {
	if !looksLikeESM("import { x } from \"y\";\nexport default x;\n") {
		t.Fatal("esm")
	}
	if looksLikeESM("/* import x from \"y\" */\nreturn a + 1") {
		t.Fatal("comment")
	}
	if looksLikeESM("var s = \"export default 1\";") {
		t.Fatal("string")
	}
	if looksLikeESM("/*code*/") {
		t.Fatal("padding")
	}
	if looksLikeESM("obj.import(x)") {
		t.Fatal("method")
	}
}
