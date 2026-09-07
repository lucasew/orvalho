package workers

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestScriptMainNoDefaultFetch(t *testing.T) {
	iso := New("", Options{})
	if err := iso.ScriptMain(t.Context(), `var x = 1;`, "main.js"); err != nil {
		t.Fatalf("ScriptMain: %v", err)
	}
}

func TestScriptMainRequireDependency(t *testing.T) {
	fsys := fstest.MapFS{
		"main.js":                           {Data: []byte(`var p = require("leftpad"); if (p.pad("1") !== "01") throw new Error("bad");`)},
		"node_modules/leftpad/package.json": {Data: []byte(`{"main":"index.js"}`)},
		"node_modules/leftpad/index.js":     {Data: []byte(`exports.pad = function (s) { return "0" + s; };`)},
	}
	src, err := fs.ReadFile(fsys, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{
		Imports: []imports.Handler[any]{imports.NodeModules{FS: fsys, From: "main.js"}},
	})
	if err := iso.ScriptMain(t.Context(), string(src), "main.js"); err != nil {
		t.Fatalf("ScriptMain: %v", err)
	}
}

func TestScriptMainThrow(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `throw new Error("nope");`, "main.js")
	if !errors.Is(err, ErrScriptThrow) {
		t.Fatalf("got %v want ErrScriptThrow", err)
	}
}

func TestScriptMainSetTimeoutZero(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		globalThis.ran = false;
		setTimeout(function () { globalThis.ran = true; }, 0);
	`, "main.js")
	if err != nil {
		t.Fatalf("ScriptMain: %v", err)
	}
	if !iso.vm.Get("ran").ToBoolean() {
		t.Fatal("setTimeout(0) callback did not run")
	}
}

func TestScriptMainShebang(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), "#!/usr/bin/env node\nglobalThis.ran = true;\n", "bin.js")
	if err != nil {
		t.Fatalf("ScriptMain: %v", err)
	}
	if !iso.vm.Get("ran").ToBoolean() {
		t.Fatal("shebang script did not run")
	}
}

func TestScriptMainImportCall(t *testing.T) {
	fsys := fstest.MapFS{
		"main.js": {Data: []byte(`
			globalThis.got = 0;
			import("./lib.js").then(function (m) { globalThis.got = m.n; });
		`)},
		"lib.js": {Data: []byte(`exports.n = 7;`)},
	}
	src, err := fs.ReadFile(fsys, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	iso := New("", Options{
		Imports: []imports.Handler[any]{imports.NodeModules{FS: fsys, From: "main.js"}},
	})
	if err := iso.ScriptMain(t.Context(), string(src), "main.js"); err != nil {
		t.Fatalf("ScriptMain: %v", err)
	}
	if n := iso.vm.Get("got").ToInteger(); n != 7 {
		t.Fatalf("got=%d want 7", n)
	}
}

func TestScriptMainProcessExitZero(t *testing.T) {
	iso := New("", Options{Argv: []string{"node", "main.js", "dev"}})
	err := iso.ScriptMain(t.Context(), `
		if (process.argv[2] !== "dev") throw new Error("argv");
		process.exit(0);
	`, "main.js")
	if err != nil {
		t.Fatalf("exit 0: %v", err)
	}
}

func TestScriptMainUnhandledRejection(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `Promise.reject(new Error("nope"));`, "main.js")
	if !errors.Is(err, ErrScriptThrow) {
		t.Fatalf("got %v want ErrScriptThrow", err)
	}
}

func TestScriptMainProcessExitOne(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `process.exit(1);`, "main.js")
	var ex *ScriptExitError
	if !errors.As(err, &ex) || ex.Code != 1 {
		t.Fatalf("got %v want ScriptExitError 1", err)
	}
}

func TestScriptMainTimerThrow(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		setTimeout(function () { throw new Error("later"); }, 0);
	`, "main.js")
	if !errors.Is(err, ErrScriptThrow) {
		t.Fatalf("got %v want ErrScriptThrow", err)
	}
}
