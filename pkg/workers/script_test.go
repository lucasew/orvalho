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

func TestScriptMainTimerThrow(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		setTimeout(function () { throw new Error("later"); }, 0);
	`, "main.js")
	if !errors.Is(err, ErrScriptThrow) {
		t.Fatalf("got %v want ErrScriptThrow", err)
	}
}
