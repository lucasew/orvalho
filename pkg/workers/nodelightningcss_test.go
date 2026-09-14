package workers

import "testing"

func TestNodeLightningCSSTransform(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var native = require("lightningcss-linux-x64-gnu");
		if (require("lightningcss-linux-x64-musl") !== native) throw new Error("identity");
		if (typeof native.transform !== "function") throw new Error("transform");
		var r = native.transform({filename: "x.css", code: Buffer.from("a { color: red }")});
		if (!r || r.code == null) throw new Error("code");
		if (!Array.isArray(r.warnings)) throw new Error("warnings");
		var s = String(r.code);
		if (s.indexOf("color") < 0) throw new Error("css " + s);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
