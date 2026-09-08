package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func runNodeCrypto(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeCryptoIdentity(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		if (require("node:crypto") !== crypto) throw new Error("node:crypto identity");
		if (typeof crypto.randomBytes !== "function") throw new Error("randomBytes");
		if (typeof crypto.createHash !== "function") throw new Error("createHash");
	`)
}

func TestNodeCryptoRandomBytes(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var a = crypto.randomBytes(16);
		var b = crypto.randomBytes(16);
		if (a.length !== 16) throw new Error("len a " + a.length);
		if (b.length !== 16) throw new Error("len b " + b.length);
		var same = true;
		for (var i = 0; i < 16; i++) {
			if (a[i] !== b[i]) { same = false; break; }
		}
		if (same) throw new Error("two calls should differ");
	`)
}

func TestNodeCryptoRandomFillSync(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var u = new Uint8Array(8);
		var before = Array.prototype.slice.call(u);
		var ret = crypto.randomFillSync(u);
		if (ret !== u) throw new Error("return identity");
		var changed = false;
		for (var i = 0; i < u.length; i++) {
			if (u[i] !== before[i]) { changed = true; break; }
		}
		if (!changed) throw new Error("not mutated");
	`)
}

func TestNodeCryptoRandomFill(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var u = new Uint8Array(8);
		crypto.randomFill(u, function (err, buf) {
			if (err) throw err;
			if (buf !== u) throw new Error("callback buf");
			var n = 0;
			for (var i = 0; i < u.length; i++) n |= u[i];
			if (n === 0) throw new Error("still zero");
		});
	`)
}

func TestNodeCryptoGetRandomValues(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		if (typeof crypto.getRandomValues !== "function") throw new Error("getRandomValues");
		var u = new Uint8Array(9);
		var ret = crypto.getRandomValues(u);
		if (ret !== u) throw new Error("return identity");
		var n = 0;
		for (var i = 0; i < u.length; i++) n |= u[i];
		if (n === 0) throw new Error("still zero");
		if (crypto.webcrypto.getRandomValues !== crypto.getRandomValues) throw new Error("webcrypto");
		var tok = Buffer.from(crypto.getRandomValues(new Uint8Array(9))).toString("base64url");
		if (typeof tok !== "string" || tok.length < 8) throw new Error("token " + tok);
	`)
}

func TestNodeCryptoRandomUUID(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var u = crypto.randomUUID();
		if (typeof u !== "string" || u.length !== 36) throw new Error("shape " + u);
		if (u.charAt(14) !== "4") throw new Error("version " + u);
		var c = u.charAt(19);
		if ("89ab".indexOf(c) < 0) throw new Error("variant " + u);
		if (crypto.randomUUID() === u) throw new Error("same uuid");
	`)
}

func TestNodeCryptoHash(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var hex = crypto.hash("sha256", "abc", "hex");
		if (hex !== "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad") {
			throw new Error("hash " + hex);
		}
		var short = hex.substring(0, 8);
		if (short.length !== 8) throw new Error("slice");
	`)
}

func TestNodeCryptoESMDefaultHash(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), PrepareSource: bundle.TransformCJS})
	err := iso.ScriptMain(t.Context(), `
		import crypto from "node:crypto";
		if (typeof crypto.hash !== "function") throw new Error("hash " + typeof crypto.hash);
		var hex = crypto.hash("sha256", "abc", "hex");
		if (hex.indexOf("ba7816bf") !== 0) throw new Error("hex " + hex);
	`, "t.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeCryptoCreateHash(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		var hex = crypto.createHash("sha256").update("abc").digest("hex");
		if (hex !== "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad") {
			throw new Error("sha256 abc " + hex);
		}
		var hashes = crypto.getHashes();
		if (hashes.indexOf("sha256") < 0) throw new Error("getHashes sha256");
		if (hashes.indexOf("sha1") < 0) throw new Error("getHashes sha1");
		if (hashes.indexOf("md5") < 0) throw new Error("getHashes md5");
	`)
}

func TestNodeCryptoErrors(t *testing.T) {
	runNodeCrypto(t, `
		var crypto = require("crypto");
		function expectCode(fn, code) {
			try {
				fn();
				throw new Error("should throw " + code);
			} catch (e) {
				if (e.code !== code) throw new Error("want " + code + " got " + e.code + " " + e);
			}
		}
		expectCode(function () { crypto.randomBytes("nope"); }, "ERR_INVALID_ARG_TYPE");
		expectCode(function () { crypto.randomBytes(-1); }, "ERR_OUT_OF_RANGE");
		expectCode(function () { crypto.createHash("xyzzy"); }, "ERR_CRYPTO_UNKNOWN_DIGEST");
		expectCode(function () { crypto.createHash(1); }, "ERR_INVALID_ARG_TYPE");
	`)
}
