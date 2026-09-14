package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func TestESMDefaultProcess(t *testing.T) {
	iso := New("", Options{
		Imports:       NodeScriptImports(),
		PrepareSource: bundle.TransformCJS,
	})
	err := iso.ScriptMain(t.Context(), `
		import process from "node:process";
		if (process.platform !== "wasi") throw new Error("platform " + process.platform);
		import os from "node:os";
		if (typeof os.platform !== "function") throw new Error("os");
		import crypto from "node:crypto";
		if (typeof crypto.createHash !== "function") throw new Error("crypto");
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGojaNamedGroup(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var m = new RegExp("(?<id>x)").exec("x");
		if (!m) throw new Error("no match");
		if (!m.groups) throw new Error("no groups");
		if (m.groups.id !== "x") throw new Error("id " + m.groups.id);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransformCJSUtilPromisify(t *testing.T) {
	iso := New("", Options{
		Imports:       NodeScriptImports(),
		PrepareSource: bundle.TransformCJS,
	})
	err := iso.ScriptMain(t.Context(), `
		import { promisify } from "node:util";
		if (typeof promisify !== "function") throw new Error("promisify");
		function cb(done) { done(null, 7); }
		var n = 0;
		promisify(cb)().then(function (v) { n = v; });
		setTimeout(function () {
			if (n !== 7) throw new Error("val " + n);
		}, 0);
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}
