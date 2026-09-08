package workers

import "testing"

func runNodeBuffer(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeBufferIdentity(t *testing.T) {
	runNodeBuffer(t, `
		var buf = require("buffer");
		if (require("node:buffer") !== buf) throw new Error("identity");
		if (buf.Buffer !== Buffer) throw new Error("global");
		if (typeof Buffer.from !== "function") throw new Error("from");
		if (Buffer.isEncoding("utf8") !== true) throw new Error("utf8");
		if (Buffer.isEncoding("utf9") !== false) throw new Error("utf9");
		if (Buffer.isBuffer(Buffer.from("ab")) !== true) throw new Error("isBuffer");
		if (Buffer.isBuffer(new Uint8Array(2)) !== false) throw new Error("u8");
	`)
}

func TestNodeBufferRoundtrip(t *testing.T) {
	runNodeBuffer(t, `
		var a = Buffer.from("hello");
		if (a.toString() !== "hello") throw new Error("utf8 " + a.toString());
		if (Buffer.byteLength("hé") !== Buffer.from("hé").length) throw new Error("byteLength");
		var hex = Buffer.from("dead", "hex");
		if (hex.toString("hex") !== "dead") throw new Error("hex " + hex.toString("hex"));
		var z = Buffer.alloc(3);
		if (z.length !== 3 || z[0] !== 0) throw new Error("alloc");
		var c = Buffer.concat([Buffer.from("a"), Buffer.from("b")]);
		if (c.toString() !== "ab") throw new Error("concat " + c.toString());
	`)
}
