package workers

import "testing"

func runNodeZlib(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeZlibIdentity(t *testing.T) {
	runNodeZlib(t, `
		var zlib = require("zlib");
		if (require("node:zlib") !== zlib) throw new Error("identity");
		if (typeof zlib.gzip !== "function") throw new Error("gzip");
		if (typeof zlib.gzipSync !== "function") throw new Error("gzipSync");
		if (zlib.constants.Z_OK !== 0) throw new Error("Z_OK");
		if (zlib.codes.Z_OK !== 0) throw new Error("codes");
		if (zlib.default !== zlib) throw new Error("default");
	`)
}

func TestNodeZlibGzipRoundtrip(t *testing.T) {
	runNodeZlib(t, `
		var zlib = require("zlib");
		var raw = "hello zlib";
		if (zlib.gunzipSync(zlib.gzipSync(raw)).toString() !== raw) throw new Error("sync");
		zlib.gzip(raw, function (err, buf) {
			if (err) throw err;
			zlib.gunzip(buf, function (err2, dec) {
				if (err2) throw err2;
				if (dec.toString() !== raw) throw new Error("roundtrip " + dec.toString());
			});
		});
	`)
}
