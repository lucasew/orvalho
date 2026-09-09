package workers

import (
	"testing"
	"testing/fstest"
)

func TestEsbuildTransform(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		if (require("esbuild/lib/main.js") !== esbuild) throw new Error("identity");
		if (typeof esbuild.transform !== "function") throw new Error("transform");
		if (typeof esbuild.version !== "string") throw new Error("version");
		var got = "";
		esbuild.transform("export const x = 1;", { loader: "js", format: "cjs" }).then(function (r) {
			got = r.code;
			if (got.indexOf("x") < 0) throw new Error("code " + got);
		});
		setTimeout(function () {
			if (!got) throw new Error("no result");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildTransformDefine(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var r = esbuild.transformSync("export const x = foo;", { loader: "js", format: "cjs", define: { foo: "1" } });
		if (r.code.indexOf("1") < 0) throw new Error("define " + JSON.stringify(r.code));
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildFormatMessages(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var n = 0;
		esbuild.formatMessages([{ text: "boom", location: { file: "a.js", line: 1, column: 0 } }], { kind: "error" }).then(function (lines) {
			if (!lines || !lines.length) throw new Error("empty");
			if (String(lines[0]).indexOf("boom") < 0) throw new Error(lines[0]);
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("no format");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildBuildGuestFS(t *testing.T) {
	fsys := fstest.MapFS{
		"in.js": {Data: []byte(`export const n = 2;`)},
	}
	iso := New("", Options{FS: fsys, Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var got = "";
		esbuild.build({ entryPoints: ["in.js"], write: false, format: "cjs" }).then(function (r) {
			got = r.outputFiles[0].text;
			if (got.indexOf("2") < 0) throw new Error("build " + got);
		});
		setTimeout(function () {
			if (!got) throw new Error("no build");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildContextRebuild(t *testing.T) {
	fsys := fstest.MapFS{
		"in.js": {Data: []byte(`export const n = 3;`)},
	}
	iso := New("", Options{FS: fsys, Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		if (typeof esbuild.context !== "function") throw new Error("context");
		var got = "";
		esbuild.context({ entryPoints: ["in.js"], write: false, format: "cjs" }).then(function (ctx) {
			if (typeof ctx.rebuild !== "function") throw new Error("rebuild");
			if (typeof ctx.dispose !== "function") throw new Error("dispose");
			if (typeof ctx.cancel !== "function") throw new Error("cancel");
			return ctx.rebuild().then(function (r) {
				got = r.outputFiles[0].text;
				if (got.indexOf("3") < 0) throw new Error("rebuild " + got);
				return ctx.dispose();
			});
		});
		setTimeout(function () {
			if (!got) throw new Error("no context rebuild");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildContextStdin(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var got = "";
		esbuild.context({
			stdin: { contents: "export const n = 4;", loader: "js" },
			write: false,
			format: "cjs"
		}).then(function (ctx) {
			return ctx.rebuild().then(function (r) {
				got = r.outputFiles[0].text;
				if (got.indexOf("4") < 0) throw new Error("stdin " + got);
				return ctx.dispose();
			});
		});
		setTimeout(function () {
			if (!got) throw new Error("no stdin rebuild");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
