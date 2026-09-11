package workers

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestEsbuildBuildWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.js"), []byte("export const n = 3;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{FS: os.DirFS(dir), Cwd: dir, Imports: NodeScriptImports()})
	src := `
		var esbuild = require("esbuild");
		var dir = ` + "`" + dir + "`" + `;
		esbuild.build({
			entryPoints: ["in.js"],
			outfile: "out.js",
			write: true,
			format: "cjs",
			absWorkingDir: dir
		});
	`
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("3")) {
		t.Fatalf("out %s", got)
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

func TestEsbuildContextSvelteScan(t *testing.T) {
	fsys := fstest.MapFS{
		"App.svelte": {Data: []byte(`<script lang="ts">
  import Foo from "./Foo.svelte";
  export let title: string;
</script>
<div>{title}</div>
`)},
		"Foo.svelte": {Data: []byte(`<script lang="ts">
  const n: number = 1;
</script>
<span>{n}</span>
`)},
	}
	iso := New("", Options{FS: fsys, Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var ok = false;
		esbuild.context({
			stdin: { contents: 'import "./App.svelte";', loader: "js" },
			bundle: true,
			write: false,
			format: "esm"
		}).then(function (ctx) {
			return ctx.rebuild().then(function (r) {
				if (r.errors && r.errors.length) throw new Error("errors " + JSON.stringify(r.errors));
				ok = true;
				return ctx.dispose();
			});
		});
		setTimeout(function () {
			if (!ok) throw new Error("svelte scan failed");
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildEntryPointStub(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var ok = false;
		esbuild.context({
			entryPoints: ["svelte_internal"],
			write: false,
			format: "esm"
		}).then(function (ctx) {
			return ctx.rebuild().then(function (r) {
				if (r.errors && r.errors.length) throw new Error("errors " + JSON.stringify(r.errors));
				ok = true;
				return ctx.dispose();
			});
		});
		setTimeout(function () {
			if (!ok) throw new Error("entry stub failed");
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
