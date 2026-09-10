package workers

import (
	"testing"
	"testing/fstest"
)

func TestNodeOxideScan(t *testing.T) {
	iso := New("", Options{
		Imports: NodeScriptImports(),
		FS: fstest.MapFS{
			"src/btn.js": {Data: []byte(`el.className = "flex items-center p-4 hover:bg-red-500"`)},
		},
		Cwd: "/",
	})
	err := iso.ScriptMain(t.Context(), `
		var oxide = require("@tailwindcss/oxide");
		if (typeof oxide.Scanner !== "function") throw new Error("Scanner");
		var s = new oxide.Scanner({sources: [{base: ".", pattern: "**/*.{js,html}", negated: false}]});
		var got = s.scan();
		function has(x) {
			for (var i = 0; i < got.length; i++) if (got[i] === x) return true;
			return false;
		}
		if (!has("flex")) throw new Error("flex " + got);
		if (!has("p-4")) throw new Error("p-4 " + got);
		if (!has("hover:bg-red-500")) throw new Error("hover " + got);
		if (!Array.isArray(s.files)) throw new Error("files");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
