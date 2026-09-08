package workers

import "testing"

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
		function dump(label, u) {
			throw new Error(label
				+ " proto=" + JSON.stringify(u.protocol)
				+ " href=" + JSON.stringify(u.href)
				+ " pathname=" + JSON.stringify(u.pathname)
				+ " host=" + JSON.stringify(u.host));
		}
		var file = new URL("file:///node_modules/@cloudflare/vite-plugin/dist/index.mjs");
		if (file.protocol !== "file:") dump("file url", file);
		var cwd = require("url").pathToFileURL("/tmp/orvalho-url");
		if (cwd.protocol !== "file:" || !cwd.href) dump("pathToFileURL", cwd);
		var back = require("url").fileURLToPath(cwd);
		if (back !== "/tmp/orvalho-url") throw new Error("round " + back);
		try {
			var bare = new URL("unenv/runtime/node/crypto");
			dump("bare specifier should throw", bare);
		} catch (e) {
			if (String(e).indexOf("Invalid URL") < 0 && String(e).indexOf("invalid") < 0) {
				throw new Error("bare err " + e);
			}
		}
		var nodeu = new URL("node:fs");
		if (nodeu.protocol !== "node:") dump("node:fs", nodeu);
	`)
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
