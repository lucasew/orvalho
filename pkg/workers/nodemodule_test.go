package workers

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/lucasew/orvalho/pkg/imports"
	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func runNodeModule(t *testing.T, src string) {
	t.Helper()
	runNodeModuleFS(t, nil, src)
}

func runNodeModuleFS(t *testing.T, fsys fs.FS, src string) {
	t.Helper()
	handlers := NodeScriptImports()
	if fsys != nil {
		handlers = append(handlers, imports.NodeModules{FS: fsys})
	}
	iso := New("", Options{Imports: handlers})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeModuleIdentity(t *testing.T) {
	runNodeModule(t, `
		var mod = require("module");
		if (require("node:module") !== mod) throw new Error("node:module identity");
		if (mod.Module !== mod) throw new Error("Module namespace");
		if (typeof mod.createRequire !== "function") throw new Error("createRequire");
		if (typeof mod.isBuiltin !== "function") throw new Error("isBuiltin");
	`)
}

func TestNodeModuleIsBuiltin(t *testing.T) {
	runNodeModule(t, `
		var isBuiltin = require("module").isBuiltin;
		if (!isBuiltin("http")) throw new Error("http");
		if (!isBuiltin("sys")) throw new Error("sys");
		if (!isBuiltin("node:fs")) throw new Error("node:fs");
		if (!isBuiltin("node:test")) throw new Error("node:test");
		if (isBuiltin("internal/errors")) throw new Error("internal/errors");
		if (isBuiltin("test")) throw new Error("test");
		if (isBuiltin("")) throw new Error("empty");
		if (isBuiltin(undefined)) throw new Error("undefined");
	`)
}

func TestNodeModuleBuiltinModules(t *testing.T) {
	runNodeModule(t, `
		var builtinModules = require("module").builtinModules;
		if (builtinModules.indexOf("http") < 0) throw new Error("http");
		if (builtinModules.indexOf("sys") < 0) throw new Error("sys");
		var internal = builtinModules.filter(function (mod) { return mod.indexOf("internal/") === 0; });
		if (internal.length !== 0) throw new Error("internal " + internal);
	`)
}

func TestNodeModuleCreateRequireInvalid(t *testing.T) {
	runNodeModule(t, `
		var createRequire = require("module").createRequire;
		function expectInvalid(fn) {
			try {
				fn();
				throw new Error("should throw");
			} catch (e) {
				if (e.code !== "ERR_INVALID_ARG_VALUE") throw new Error("code " + e.code);
			}
		}
		expectInvalid(function () { createRequire({}); });
		expectInvalid(function () { createRequire("../"); });
		expectInvalid(function () { createRequire("https://github.com/nodejs/node/pull/27405/"); });
	`)
}

func TestNodeModuleRequireResolve(t *testing.T) {
	fsys := fstest.MapFS{
		"app/pkg/lib.js":       {Data: []byte(`exports.n = 1;`)},
		"app/pkg/package.json": {Data: []byte(`{"name":"pkg"}`)},
	}
	runNodeModuleFS(t, fsys, `
		if (typeof require.resolve !== "function") throw new Error("require.resolve");
		if (require.resolve("fs") !== "fs") throw new Error("builtin " + require.resolve("fs"));
		var createRequire = require("module").createRequire;
		var req = createRequire("/app/pkg/index.js");
		if (typeof req.resolve !== "function") throw new Error("createRequire.resolve");
		var p = req.resolve("./lib.js");
		if (p.indexOf("lib.js") < 0) throw new Error("rel " + p);
	`)
}

func TestNodeModuleCreateRequireRelative(t *testing.T) {
	fsys := fstest.MapFS{
		"app/pkg/lib.js": {Data: []byte(`exports.n = 42;`)},
	}
	runNodeModuleFS(t, fsys, `
		var createRequire = require("module").createRequire;
		var req = createRequire("/app/pkg/index.js");
		var lib = req("./lib.js");
		if (lib.n !== 42) throw new Error("path " + lib.n);
		var reqURL = createRequire("file:///app/pkg/index.js");
		if (reqURL("./lib.js").n !== 42) throw new Error("file url string");
		var reqObj = createRequire({ href: "file:///app/pkg/index.js" });
		if (reqObj("./lib.js").n !== 42) throw new Error("file url object");
	`)
}

func TestNodeModuleCreateRequireImportMeta(t *testing.T) {
	fsys := fstest.MapFS{
		"app/pkg/lib.js": {Data: []byte(`exports.n = 42;`)},
	}
	src := `
		import { createRequire } from "module";
		var req = createRequire(import.meta.url);
		if (req("./lib.js").n !== 42) throw new Error("import.meta.url");
	`
	handlers := append(NodeScriptImports(), imports.NodeModules{FS: fsys, From: "app/pkg/main.mjs"})
	iso := New("", Options{Imports: handlers, PrepareSource: bundle.CompileCJS})
	if err := iso.ScriptMain(t.Context(), src, "app/pkg/main.mjs"); err != nil {
		t.Fatal(err)
	}
}
