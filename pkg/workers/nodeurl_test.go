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
		var bare = new URL("/tmp/orvalho-url");
		var p2 = url.fileURLToPath(bare);
		if (p2 !== "/tmp/orvalho-url") throw new Error("bare " + p2);
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
