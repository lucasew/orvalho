package workers

import "testing"

func runNodeV8(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeV8Identity(t *testing.T) {
	runNodeV8(t, `
		var v8 = require("v8");
		if (require("node:v8") !== v8) throw new Error("identity");
		if (v8.startupSnapshot.isBuildingSnapshot() !== false) throw new Error("snapshot");
		if (typeof v8.getHeapStatistics !== "undefined") throw new Error("heap");
		if (v8.default !== v8) throw new Error("default");
	`)
}
