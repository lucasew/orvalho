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
		try { Buffer.alloc(-1); throw new Error("alloc neg"); } catch (e) {
			if (e.code !== "ERR_OUT_OF_RANGE") throw new Error("neg " + e.code);
		}
		var u8 = new Uint8Array([1, 2]);
		var copied = Buffer.from(u8);
		u8[0] = 9;
		if (copied[0] !== 1) throw new Error("from view must copy");
		var ab = new ArrayBuffer(2);
		var shared = Buffer.from(ab);
		new Uint8Array(ab)[0] = 7;
		if (shared[0] !== 7) throw new Error("from ArrayBuffer must share");
		var sl = copied.slice(0, 1);
		copied[0] = 3;
		if (sl[0] !== 3) throw new Error("slice must overlap");
	`)
}

func TestNodeBufferToStringEncodings(t *testing.T) {
	runNodeBuffer(t, `
		var b = Buffer.from([0x68, 0xc3, 0xa9]);
		if (b.toString() !== "hé") throw new Error("utf8 " + b.toString());
		if (b.toString("utf8") !== "hé") throw new Error("utf8 named");
		if (Buffer.from("dead", "hex").toString("hex") !== "dead") throw new Error("hex");
		if (Buffer.from("hi").toString("base64") !== "aGk=") throw new Error("b64 " + Buffer.from("hi").toString("base64"));
		if (Buffer.from("hi").toString("base64url") !== "aGk") throw new Error("b64url");
		var hi = Buffer.from([0xff, 0x00, 0x41]);
		if (hi.toString("latin1").length !== 3 || hi.toString("latin1").charCodeAt(0) !== 255) throw new Error("latin1");
		if (hi.toString("ascii").charCodeAt(0) !== 127) throw new Error("ascii " + hi.toString("ascii").charCodeAt(0));
		var u = Buffer.from("é", "utf16le");
		if (u.toString("utf16le") !== "é") throw new Error("utf16le " + u.toString("utf16le"));
		if (Buffer.from("hello").toString("utf8", 1, 4) !== "ell") throw new Error("slice " + Buffer.from("hello").toString("utf8", 1, 4));
		if (btoa("hi") !== "aGk=") throw new Error("btoa " + btoa("hi"));
		if (atob("aGk=") !== "hi") throw new Error("atob " + atob("aGk="));
		var raw = String.fromCharCode(255, 0, 65);
		if (atob(btoa(raw)).charCodeAt(0) !== 255) throw new Error("atob latin1");
	`)
}

func TestNodeBufferToStringLarge(t *testing.T) {
	// 1MiB latin1. The old JS path did s += fromCharCode per byte
	// (~500GiB of goja Concat). Native decode must stay O(n).
	runNodeBuffer(t, `
		var n = 1024 * 1024;
		var b = Buffer.alloc(n);
		for (var i = 0; i < n; i++) b[i] = 0xc0;
		var s = b.toString("latin1");
		if (s.length !== n) throw new Error("len " + s.length);
		if (s.charCodeAt(0) !== 0xc0 || s.charCodeAt(n - 1) !== 0xc0) throw new Error("bytes");
		if (btoa(Buffer.from("x").toString("latin1")) !== "eA==") throw new Error("btoa");
	`)
}
