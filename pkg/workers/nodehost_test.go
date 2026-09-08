package workers

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestNodeConsole(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var c = require("node:console");
		if (typeof c.log !== "function") throw new Error("log");
		if (require("console") !== c) throw new Error("alias");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostPathIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var path = require("path");
		var posix = require("path/posix");
		var win32 = require("path/win32");
		if (posix !== path.posix) throw new Error("path/posix !== path.posix");
		if (win32 !== path.win32) throw new Error("path/win32 !== path.win32");
		if (path.sep !== "/") throw new Error("path.sep");
		if (win32.sep !== "\\") throw new Error("win32.sep");
	`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostTypesIdentity(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		if (require("util/types") !== require("util").types) throw new Error("types identity");
	`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeStreamConsumers(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var c = require("stream/consumers");
		if (require("node:stream/consumers") !== c) throw new Error("identity");
		if (typeof c.text !== "function") throw new Error("text");
		if (typeof c.json !== "function") throw new Error("json");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeStringDecoder(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var sd = require("string_decoder");
		if (require("node:string_decoder") !== sd) throw new Error("identity");
		if (typeof sd.StringDecoder !== "function") throw new Error("ctor");
		var d = new sd.StringDecoder("utf8");
		var s = d.write(Buffer.from("hi"));
		if (s !== "hi") throw new Error("write " + s);
		if (typeof d.end !== "function") throw new Error("end");
		if (d.end() !== "") throw new Error("end empty");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeConstants(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var c = require("constants");
		if (require("node:constants") !== c) throw new Error("identity");
		var os = require("os");
		if (c.os !== os.constants) throw new Error("os nest");
		if (typeof os.constants.signals.SIGTERM === "number") {
			if (c.signals.SIGTERM !== os.constants.signals.SIGTERM) throw new Error("SIGTERM");
		}
		var fs = require("fs");
		if (c.fs !== fs.constants) throw new Error("fs nest");
		if (typeof fs.constants.F_OK === "number" && c.F_OK !== fs.constants.F_OK) throw new Error("F_OK");
		if (typeof fs.constants.O_RDONLY === "number" && c.O_RDONLY !== fs.constants.O_RDONLY) throw new Error("O_RDONLY");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodePunycode(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var p = require("punycode");
		if (require("node:punycode") !== p) throw new Error("identity");
		if (typeof p.toASCII !== "function") throw new Error("toASCII");
		if (typeof p.toUnicode !== "function") throw new Error("toUnicode");
		if (p.toASCII("localhost") !== "localhost") throw new Error("ascii host");
		if (p.toASCII("example.com") !== "example.com") throw new Error("ascii domain");
		if (p.toUnicode("localhost") !== "localhost") throw new Error("unicode host");
		if (p.toUnicode("example.com") !== "example.com") throw new Error("unicode domain");
		if (p.ucs2) {
			var pts = p.ucs2.decode("ab");
			if (!pts || pts.length !== 2 || pts[0] !== 97 || pts[1] !== 98) throw new Error("ucs2");
		}
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeDomain(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var d = require("domain");
		if (require("node:domain") !== d) throw new Error("identity");
		if (typeof d.create !== "function") throw new Error("create");
		var local = d.create();
		if (typeof local.bind !== "function") throw new Error("bind");
		var n = 0;
		local.bind(function () { n = 1; })();
		if (n !== 1) throw new Error("called " + n);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostCommonDoesNotShadowFile(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), imports.NodeModules{
			FS: fstest.MapFS{
				"debug/src/common.js": {Data: []byte(`module.exports = function (e) { e.ok = 1; return e; };`)},
			},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var fn = require("debug/src/common");
		if (typeof fn !== "function") throw new Error("shadowed " + typeof fn);
		if (fn({}).ok !== 1) throw new Error("ok");
		var stub = require("common");
		if (typeof stub.mustCall !== "function") throw new Error("stub");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostCommonDoesNotShadowScopedPackage(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), imports.NodeModules{
			FS: fstest.MapFS{
				"@scope/common/package.json": {Data: []byte(`{"main":"index.js"}`)},
				"@scope/common/index.js":     {Data: []byte(`module.exports = { pkg: 1 };`)},
			},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var m = require("@scope/common");
		if (m.pkg !== 1) throw new Error("shadowed " + typeof m.mustCall);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostCommonDoesNotShadowDirIndex(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), imports.NodeModules{
			FS: fstest.MapFS{
				"foo/package.json":    {Data: []byte(`{}`)},
				"foo/common/index.js": {Data: []byte(`module.exports = { dir: 1 };`)},
			},
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var m = require("foo/common");
		if (m.dir !== 1) throw new Error("shadowed " + typeof m.mustCall);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSpawnWorkerdModule(t *testing.T) {
	var got string
	done := make(chan SpawnWait, 1)
	done <- SpawnWait{Code: 1}
	iso := New("", Options{
		Imports: NodeScriptImports(),
		Spawn: func(ctx context.Context, req SpawnReq) (Spawned, error) {
			got = req.File
			return stubSpawned{pid: 1, done: done}, nil
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var cp = require("child_process");
		cp.spawn(require("workerd"));
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/dev/null" {
		t.Fatalf("spawn file %q", got)
	}
}

func TestWorkerdStub(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var w = require("workerd");
		if (typeof w !== "string" && typeof w.default !== "string" && !w.version) {
			throw new Error("workerd stub " + w);
		}
		var n = require("@cloudflare/workerd-linux-64");
		if (n == null) throw new Error("optional pkg");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeStreamPromises(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var p = require("stream/promises");
		if (require("node:stream/promises") !== p) throw new Error("identity");
		if (typeof p.pipeline !== "function") throw new Error("pipeline");
		if (typeof p.finished !== "function") throw new Error("finished");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeAsyncHooksALS(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var ah = require("async_hooks");
		if (require("node:async_hooks") !== ah) throw new Error("identity");
		var als = new ah.AsyncLocalStorage();
		if (als.getStore() !== undefined) throw new Error("empty");
		var n = als.run(7, function () { return als.getStore(); });
		if (n !== 7) throw new Error("run " + n);
		if (typeof ah.createHook().enable !== "function") throw new Error("hook");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeUtilStyleText(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var util = require("util");
		if (typeof util.styleText !== "function") throw new Error("styleText");
		var s = util.styleText("red", "x");
		if (s.indexOf("x") < 0) throw new Error("text");
		var t = util.styleText(["bold", "dim"], "y");
		if (t.indexOf("y") < 0) throw new Error("multi");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeUtilTextEncoder(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var util = require("util");
		if (typeof util.TextEncoder !== "function") throw new Error("TextEncoder");
		if (typeof util.TextDecoder !== "function") throw new Error("TextDecoder");
		if (util.TextEncoder !== TextEncoder) throw new Error("alias");
		var u = new util.TextEncoder("utf-8");
		var b = u.encode("hi");
		if (b.length !== 2 || b[0] !== 104) throw new Error("encode");
		if (new util.TextDecoder().decode(b) !== "hi") throw new Error("decode");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeUtilPromisify(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var util = require("util");
		if (typeof util.promisify !== "function") throw new Error("promisify");
		var n = 0;
		function orig(cb) { cb(null, 7); }
		util.promisify(orig)().then(function (v) { n = v; });
		setTimeout(function () {
			if (n !== 7) throw new Error("val " + n);
		}, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeUtilDeprecate(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var util = require("util");
		if (typeof util.deprecate !== "function") throw new Error("deprecate");
		var n = 0;
		var wrapped = util.deprecate(function (x) { n = x; return x + 1; }, "msg");
		if (typeof wrapped !== "function") throw new Error("wrapped");
		var r = wrapped(6);
		if (r !== 7) throw new Error("val " + r);
		if (n !== 6) throw new Error("called " + n);
		var obj = { v: 3, f: function () { return this.v; } };
		obj.f = util.deprecate(obj.f, "bound");
		if (obj.f() !== 3) throw new Error("this");
		if (typeof util.deprecate(function(){}, "msg") !== "function") throw new Error("debug-style");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeHostImportsDoNotNeedTree(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), imports.NodeModules{}),
	})
	err := iso.ScriptMain(t.Context(), `require("assert").strictEqual(1, 1);`, "id.js")
	if err != nil {
		t.Fatal(err)
	}
}
