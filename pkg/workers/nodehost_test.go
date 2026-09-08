package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestNodeHostPathIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var path = require("path");
		var posix = require("path/posix");
		var win32 = require("path/win32");
		if (posix !== path.posix) throw new Error("path/posix !== path.posix");
		if (win32 !== path.win32) throw new Error("path/win32 !== path.win32");
		if (path.sep !== "/") throw new Error("path.sep");
		if (win32.sep !== "\\") throw new Error("win32.sep");
	`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostTypesIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (require("util/types") !== require("util").types) throw new Error("types identity");
	`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeUtilPromisify(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var util = require("util");
		if (typeof util.promisify !== "function") throw new Error("promisify");
		var n = 0;
		function orig(cb) { cb(null, 7); }
		util.promisify(orig)().then(function (v) { n = v; });
		setTimeout(function () {
			if (n !== 7) throw new Error("val " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostImportsDoNotNeedTree(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), imports.NodeModules{}),
	})
	err := iso.ScriptMain(t.Context(), `require("assert").strictEqual(1, 1);`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}
