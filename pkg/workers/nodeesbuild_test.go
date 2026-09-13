package workers

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestEsbuildLeavesSvelteToHost(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.svelte"), []byte("<script>export let n = 1</script>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{FS: os.DirFS(dir), Cwd: dir, Imports: NodeScriptImports()})
	out := filepath.Join(dir, "out")
	src := `
		var esbuild = require("esbuild");
		esbuild.build({
			entryPoints: ["x.svelte"],
			write: true,
			bundle: true,
			outdir: ` + "`" + out + "`" + `,
			absWorkingDir: ` + "`" + dir + "`" + `
		});
	`
	err := iso.ScriptMain(t.Context(), src, "t.js")
	if err == nil {
		t.Fatal("expected esbuild to reject raw .svelte (host path, no loader)")
	}
	if strings.Contains(err.Error(), "svelte stubbed") {
		t.Fatal(err)
	}
}

func TestEsbuildMetafileObject(t *testing.T) {
	fsys := fstest.MapFS{"in.js": {Data: []byte("export const n = 4;\n")}}
	iso := New("", Options{FS: fsys, Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var r = null;
		esbuild.build({ entryPoints: ["in.js"], write: false, format: "esm", metafile: true, bundle: true }).then(function (out) {
			r = out;
		});
		setTimeout(function () {
			if (!r) throw new Error("no result");
			if (!r.metafile || typeof r.metafile !== "object") throw new Error("metafile " + typeof r.metafile);
			if (!r.metafile.outputs) throw new Error("outputs");
		}, 200);
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
		}, 200);
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
		}, 200);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildResolvesDirImportToIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "aliases"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aliases", "index.js"), []byte("export const n = 4;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte("export * from './aliases';\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{FS: os.DirFS(dir), Cwd: dir, Imports: NodeScriptImports()})
	src := `
		var esbuild = require("esbuild");
		var r = null;
		esbuild.build({
			entryPoints: ["main.js"],
			bundle: true,
			write: false,
			format: "esm",
			absWorkingDir: ` + "`" + dir + "`" + `
		}).then(function (out) { r = out; });
		setTimeout(function () {
			if (!r) throw new Error("no result");
			if (r.errors && r.errors.length) throw new Error(JSON.stringify(r.errors));
			var text = r.outputFiles && r.outputFiles[0] && r.outputFiles[0].text || "";
			if (text.indexOf("4") < 0) throw new Error(text);
		}, 200);
	`
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildResolveKind(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var kind = "";
		esbuild.build({
			stdin: { contents: 'export { n } from "virt:x";', loader: "js" },
			bundle: true,
			write: false,
			format: "esm",
			plugins: [{
				name: "virt",
				setup: function (b) {
					b.onResolve({ filter: /^virt:/ }, function (args) {
						kind = args.kind;
						return { path: args.path, namespace: "virt" };
					});
					b.onLoad({ filter: /.*/, namespace: "virt" }, function () {
						return { contents: "export const n = 1;", loader: "js" };
					});
				}
			}]
		}).then(function () {});
		setTimeout(function () {
			if (kind !== "import-statement") throw new Error("kind " + kind);
		}, 200);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEsbuildJSPluginOnLoad(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var esbuild = require("esbuild");
		var r = null;
		var hit = 0;
		esbuild.build({
			stdin: { contents: 'export { n } from "virt:x";', loader: "js" },
			bundle: true,
			write: false,
			format: "esm",
			plugins: [{
				name: "virt",
				setup: function (b) {
					b.onResolve({ filter: /^virt:/ }, function (args) {
						hit++;
						return { path: args.path, namespace: "virt" };
					});
					b.onLoad({ filter: /.*/, namespace: "virt" }, function () {
						hit += 10;
						return { contents: "export const n = 9;", loader: "js" };
					});
				}
			}]
		}).then(function (out) { r = out; });
		setTimeout(function () {
			if (!r) throw new Error("no result hit=" + hit);
			if (r.errors && r.errors.length) throw new Error("errors " + JSON.stringify(r.errors) + " hit=" + hit);
			var files = r.outputFiles || [];
			var text = files[0] && files[0].text || "";
			if (text.indexOf("9") < 0) throw new Error("plugin hit=" + hit + " files=" + files.length + " " + text);
		}, 200);
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
		}, 200);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
