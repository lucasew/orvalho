package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func runNodeURL(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestURLResolveFileRelative(t *testing.T) {
	runNodeURL(t, `
		var inner = new URL("../../../src/node/constants.ts", "file:///vite/dist/node/chunks/logger.js");
		if (inner.href !== "file:///vite/src/node/constants.ts") throw new Error("inner " + inner.href);
		var pkg = new URL("../../package.json", inner);
		if (pkg.href !== "file:///vite/package.json") throw new Error("pkg " + pkg.href);
		var httpRel = new URL("c", "https://example.com/a/b");
		if (httpRel.href !== "https://example.com/a/c") throw new Error("http " + httpRel.href);
	`)
}

func TestNodeURLIdentity(t *testing.T) {
	runNodeURL(t, `
		if (require("url") !== require("node:url")) throw new Error("url identity");
	`)
}

func TestNodeURLPathToFileURL(t *testing.T) {
	runNodeURL(t, `
		var url = require("url");
		var href = url.pathToFileURL("test/").href;
		if (href.indexOf("file:///") !== 0) throw new Error("scheme " + href);
		if (href.charAt(href.length - 1) !== "/") throw new Error("slash " + href);
		var pct = url.pathToFileURL("a%b").href;
		if (pct.indexOf("%25") < 0) throw new Error("percent " + pct);
	`)
}

func TestNodeURLFileURLToPathAcceptsURLObject(t *testing.T) {
	runNodeURL(t, `
		var url = require("url");
		var u = new URL("file:///tmp/orvalho-url");
		var p = url.fileURLToPath(u);
		if (p !== "/tmp/orvalho-url") throw new Error("obj " + p);
		var again = url.pathToFileURL(p).toString();
		if (again.indexOf("file://") !== 0) throw new Error("href " + again);
		var bare = new URL("/tmp/orvalho-url");
		var p2 = url.fileURLToPath(bare);
		if (p2 !== "/tmp/orvalho-url") throw new Error("bare " + p2);
		try { new URL("unenv/node/inspector/promises"); throw new Error("relative URL"); }
		catch (e) {
			if (e.message === "relative URL") throw e;
			if (e.code !== "ERR_INVALID_URL" && e.name !== "TypeError") throw new Error("rel " + e.code + " " + e.message);
		}
	`)
}

func TestNodeURLFileURLToPathRoundTrip(t *testing.T) {
	runNodeURL(t, `
		var url = require("url");
		var p = "/tmp/orvalho-url";
		var back = url.fileURLToPath(url.pathToFileURL(p));
		if (back !== p) throw new Error("round-trip " + back);
	`)
}

func TestURLBareSpecifierThrowsLikeNode(t *testing.T) {
	runNodeURL(t, `
		var file = new URL("file:///node_modules/@cloudflare/vite-plugin/dist/index.mjs");
		if (file.protocol !== "file:") throw new Error("file proto " + file.protocol + " href=" + file.href);
		if (require("url").fileURLToPath(file) !== "/node_modules/@cloudflare/vite-plugin/dist/index.mjs") {
			throw new Error("file path " + require("url").fileURLToPath(file));
		}
		var cwd = require("url").pathToFileURL("/tmp/orvalho-url");
		if (cwd.protocol !== "file:" || String(cwd.href).indexOf("file://") !== 0) {
			throw new Error("pathToFileURL " + cwd.protocol + " " + cwd.href);
		}
		try {
			new URL("unenv/runtime/node/crypto");
			throw new Error("bare specifier should throw");
		} catch (e) {
			if (e.message === "bare specifier should throw") throw e;
			if (e.code !== "ERR_INVALID_URL") throw new Error("bare code " + e.code + " " + e);
		}
		var nodeu = new URL("node:fs");
		if (nodeu.protocol !== "node:") throw new Error("node proto " + nodeu.protocol + " href=" + nodeu.href);
		if (nodeu.href !== "node:fs") throw new Error("node href " + nodeu.href);
	`)
}

func TestGuestImportMetaURLIsFile(t *testing.T) {
	iso := New("", Options{
		Imports:       NodeScriptImports(),
		PrepareSource: bundle.TransformCJS,
	})
	err := iso.ScriptMain(t.Context(), `
		import { fileURLToPath, pathToFileURL } from "node:url";
		var href = import.meta.url;
		if (typeof href !== "string" || href.indexOf("file://") !== 0) throw new Error("meta " + href);
		var u = new URL(href);
		if (u.protocol !== "file:") throw new Error("protocol " + JSON.stringify(u.protocol) + " href=" + u.href);
		var p = fileURLToPath(u);
		if (!p || p.charAt(0) !== "/") throw new Error("path " + p);
		if (fileURLToPath(pathToFileURL(p)) !== p) throw new Error("round " + p);
		if (fileURLToPath(pathToFileURL("/abs/foo")) !== "/abs/foo") throw new Error("abs");
	`, "pkg/dist/plugin.mjs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeURLFormatInvalid(t *testing.T) {
	runNodeURL(t, `
		var url = require("url");
		function expectType(fn) {
			try {
				fn();
				throw new Error("should throw");
			} catch (e) {
				if (e.name !== "TypeError") throw new Error("name " + e.name);
				if (e.code !== "ERR_INVALID_ARG_TYPE") throw new Error("code " + e.code);
			}
		}
		expectType(function () { url.format(undefined); });
		expectType(function () { url.format(null); });
		expectType(function () { url.format(true); });
		expectType(function () { url.format(false); });
		expectType(function () { url.format(0); });
		expectType(function () { url.format(function () {}); });
		expectType(function () { url.format(Symbol("foo")); });
		if (url.format("") !== "") throw new Error("empty string");
		if (url.format({}) !== "") throw new Error("empty object");
	`)
}
