package workers

import "testing"

func TestNodeQuerystring(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var qs = require("querystring");
		if (require("node:querystring") !== qs) throw new Error("identity");
		var o = qs.parse("a=1&b=2&b=3");
		if (o.a !== "1") throw new Error("a");
		if (o.b[0] !== "2" || o.b[1] !== "3") throw new Error("b");
		if (qs.stringify({ a: 1, b: ["x", "y"] }) !== "a=1&b=x&b=y") throw new Error("stringify");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
