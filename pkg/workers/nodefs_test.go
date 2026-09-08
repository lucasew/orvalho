package workers

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lucasew/orvalho/pkg/imports"
	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func nodeFSMap() fstest.MapFS {
	return fstest.MapFS{
		"hello.txt":     {Data: []byte("hi")},
		"dir/a.txt":     {Data: []byte("aa")},
		"dir/sub/b.txt": {Data: []byte("b")},
	}
}

func runNodeFS(t *testing.T, fsys fs.FS, src string) {
	t.Helper()
	iso := New("", Options{FS: fsys, Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeFSReadFileSync(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		if (require("node:fs") !== fs) throw new Error("node:fs identity");
		var s = fs.readFileSync("hello.txt", "utf8");
		if (s !== "hi") throw new Error("utf8 " + s);
		var o = fs.readFileSync("hello.txt", { encoding: "utf8" });
		if (o !== "hi") throw new Error("opts " + o);
		var b = fs.readFileSync("hello.txt");
		if (b.length !== 2) throw new Error("len " + b.length);
		if (b[0] !== 104 || b[1] !== 105) throw new Error("bytes");
		if (b.toString() !== "hi") throw new Error("toString " + b.toString());
		if (typeof Buffer !== "undefined" && Buffer.isBuffer && !Buffer.isBuffer(b)) throw new Error("not Buffer");
		if (typeof b.equals !== "function") throw new Error("equals");
		if (!b.equals(Buffer.from("hi"))) throw new Error("eq");
		if (b.toString("utf-8") !== "hi") throw new Error("utf-8 " + b.toString("utf-8"));
		if (!b.subarray(0, 1).equals(Buffer.from("h"))) throw new Error("subarray");
	`)
}

func TestNodeFSClose(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		if (typeof fs.close !== "function") throw new Error("close");
		if (typeof fs.closeSync !== "function") throw new Error("closeSync");
		var fd = fs.openSync("hello.txt");
		fs.closeSync(fd);
		var n = 0;
		fs.close(fd, function (err) {
			if (err) throw err;
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("cb");
		}, 0);
	`)
}

func TestNodeFSRm(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		if (typeof fs.rm !== "function") throw new Error("rm");
		if (typeof fs.rmSync !== "function") throw new Error("rmSync");
		if (typeof fs.promises.rm !== "function") throw new Error("promises.rm");
		try { fs.rmSync("nope.txt"); throw new Error("should throw"); } catch (e) {
			if (e.message === "should throw") throw e;
			if (e.code !== "ENOENT") throw new Error("code " + e.code);
		}
		fs.rmSync("nope.txt", { force: true });
		try { fs.rmSync("hello.txt"); throw new Error("ro should throw"); } catch (e) {
			if (e.message === "ro should throw") throw e;
			if (e.code !== "EROFS") throw new Error("ro " + e.code);
		}
		var n = 0;
		fs.promises.rm("nope2.txt", { force: true }).then(function () { n = 1; });
		setTimeout(function () {
			if (n !== 1) throw new Error("promises " + n);
		}, 0);
	`)
}

func TestNodeFSExistsSync(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		if (!fs.existsSync("hello.txt")) throw new Error("hello");
		if (!fs.existsSync("dir")) throw new Error("dir");
		if (fs.existsSync("nope.txt")) throw new Error("nope");
		if (fs.existsSync()) throw new Error("empty");
		if (fs.existsSync({})) throw new Error("obj");
		if (fs.existsSync("foo\0bar")) throw new Error("nul");
	`)
}

func TestNodeFSStatSync(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		var st = fs.statSync("hello.txt");
		if (!st.isFile()) throw new Error("file");
		if (st.isDirectory()) throw new Error("not dir");
		if (st.size !== 2) throw new Error("size " + st.size);
		var d = fs.statSync("dir");
		if (!d.isDirectory()) throw new Error("dir");
		if (d.isFile()) throw new Error("dir is file");
	`)
}

func TestNodeFSMissingENOENT(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		try {
			fs.readFileSync("missing.txt");
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "ENOENT") throw new Error("code " + e.code);
		}
		try {
			fs.statSync("missing.txt");
			throw new Error("stat should throw");
		} catch (e) {
			if (e.code !== "ENOENT") throw new Error("stat " + e.code);
		}
	`)
}

func TestNodeFSNilMount(t *testing.T) {
	runNodeFS(t, nil, `
		var fs = require("fs");
		if (!fs || typeof fs.readFileSync !== "function") throw new Error("missing fs");
		if (fs.existsSync("hello.txt")) throw new Error("exists");
		try {
			fs.readFileSync("hello.txt");
			throw new Error("should throw");
		} catch (e) {
			if (e.code !== "ENOENT") throw new Error("code " + e.code);
		}
		if (typeof fs.constants.F_OK !== "number") throw new Error("constants");
		if (Object.getPrototypeOf(fs.constants) !== null) throw new Error("proto");
	`)
}

func TestNodeFSRejectsEscape(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		if (fs.existsSync("../hello.txt")) throw new Error("escape exists");
		if (fs.existsSync("dir/../../hello.txt")) throw new Error("walkout exists");
		try {
			var s = fs.readFileSync("../hello.txt", "utf8");
			throw new Error("read escape " + s);
		} catch (e) {
			if (e.code !== "ENOENT") throw new Error("code " + e.code);
		}
		var ok = fs.readFileSync("dir/../hello.txt", "utf8");
		if (ok !== "hi") throw new Error("clean " + ok);
	`)
}

func TestNodeFSDoesNotNeedDirFS(t *testing.T) {
	var fsys fs.FS = nodeFSMap()
	if _, ok := fsys.(interface{ Open(string) (fs.File, error) }); !ok {
		t.Fatal("MapFS must implement fs.FS")
	}
	runNodeFS(t, fsys, `
		var fs = require("fs");
		if (fs.readFileSync("dir/a.txt", "utf8") !== "aa") throw new Error("map");
		if (fs.existsSync("/etc/passwd")) throw new Error("host leak");
		try {
			fs.readFileSync("/etc/passwd", "utf8");
			throw new Error("read host");
		} catch (e) {
			if (e.code !== "ENOENT") throw new Error("host " + e.code);
		}
	`)
}

func TestNodeFSExistsCallback(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		var assert = require("assert");
		assert.throws(function () { fs.exists("hello.txt"); }, { code: "ERR_INVALID_ARG_TYPE" });
		fs.exists("hello.txt", function (y) {
			if (y !== true) throw new Error("exists true");
		});
		fs.exists("nope.txt", function (y) {
			if (y !== false) throw new Error("exists false");
		});
		fs.exists({}, function (y) {
			if (y !== false) throw new Error("exists obj");
		});
	`)
}

func TestNodeFSNullBytes(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		var assert = require("assert");
		var bad = "foo\0bar";
		var methods = [
			"accessSync", "appendFileSync", "chmodSync", "chownSync",
			"lstatSync", "mkdirSync", "openSync", "readFileSync",
			"readdirSync", "readlinkSync", "realpathSync", "rmSync", "rmdirSync",
			"statSync", "truncateSync", "unlinkSync", "utimesSync",
			"writeFileSync"
		];
		methods.forEach(function (name) {
			assert.throws(function () { fs[name](bad); }, {
				code: "ERR_INVALID_ARG_VALUE",
				name: "TypeError"
			}, name);
		});
		assert.throws(function () { fs.copyFileSync(bad, "x"); }, { code: "ERR_INVALID_ARG_VALUE", name: "TypeError" });
		assert.throws(function () { fs.copyFileSync("x", bad); }, { code: "ERR_INVALID_ARG_VALUE", name: "TypeError" });
		var fileUrl = new URL("file:///C:/foo\0bar");
		var fileUrl2 = new URL("file:///C:/foo%00bar");
		assert.throws(function () { fs.readFileSync(fileUrl); }, { code: "ERR_INVALID_ARG_VALUE", name: "TypeError" });
		assert.throws(function () { fs.readFileSync(fileUrl2); }, { code: "ERR_INVALID_ARG_VALUE", name: "TypeError" });
		if (fs.existsSync(bad)) throw new Error("existsSync nul");
	`)
}

func TestNodeFSReadFileAsync(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		fs.readFile("hello.txt", "utf8", function (err, data) {
			if (err) throw err;
			if (data !== "hi") throw new Error("async " + data);
		});
		fs.readFile("missing.txt", function (err) {
			if (!err || err.code !== "ENOENT") throw new Error("async missing");
		});
	`)
}

func TestNodeFSWriteReadOnly(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		try {
			fs.writeFileSync("hello.txt", "nope");
			throw new Error("should EROFS");
		} catch (e) {
			if (e.code !== "EROFS") throw new Error("code " + e.code);
		}
	`)
}

func TestNormalizeGuest(t *testing.T) {
	got, err := normalizeGuest("dir/../hello.txt")
	if err != nil || got != "hello.txt" {
		t.Fatalf("clean: %q %v", got, err)
	}
	if _, err := normalizeGuest("../secret"); err == nil {
		t.Fatal("expected escape")
	}
	if _, err := normalizeGuest("a/../../b"); err == nil {
		t.Fatal("expected walkout")
	}
	got, err = normalizeGuest("/hello.txt")
	if err != nil || got != "hello.txt" {
		t.Fatalf("abs: %q %v", got, err)
	}
}

func TestStripCwdPrefix(t *testing.T) {
	got := stripCwdPrefix("/guest/root/hello.txt", "/guest/root")
	if got != "hello.txt" {
		t.Fatalf("abs: %q", got)
	}
	got = stripCwdPrefix("/guest/root", "/guest/root")
	if got != "." {
		t.Fatalf("cwd: %q", got)
	}
	got = stripCwdPrefix("guest/root/dir/a.txt", "/guest/root")
	if got != "dir/a.txt" {
		t.Fatalf("stripped slash: %q", got)
	}
	got = stripCwdPrefix("/guest/root/../root/hello.txt", "/guest/root")
	if got != "hello.txt" {
		t.Fatalf("clean: %q", got)
	}
	got = stripCwdPrefix("/hello.txt", "/guest/root")
	if got != "/hello.txt" {
		t.Fatalf("other abs: %q", got)
	}
	got = stripCwdPrefix("hello.txt", ".")
	if got != "hello.txt" {
		t.Fatalf("rel cwd: %q", got)
	}
}

func TestRequireFileURL(t *testing.T) {
	fsys := fstest.MapFS{
		"mod.js": {Data: []byte("exports.n = 3;\n")},
	}
	iso := New("", Options{
		FS:  fsys,
		Cwd: "/guest/root",
		Imports: append(NodeScriptImports(), imports.NodeModules{
			FS:   fsys,
			From: "t.js",
		}),
	})
	err := iso.ScriptMain(t.Context(), `
		var m = require("file:///guest/root/mod.js");
		if (!m || m.n !== 3) throw new Error("n " + (m && m.n));
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestOrvalhoESMParse(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports(), PrepareSource: bundle.TransformCJS})
	src := `
		function parse(E$1, g) {
			if (!C) return init.then((() => parse(E$1)));
		}
		var C, init;
		var src = "import { defineConfig } from 'astro/config';\nimport svelte from '@astrojs/svelte';\nexport default 1;\n";
		var got = __orvalhoESMParse(src);
		if (!got || !got[0] || got[0].length !== 2) throw new Error("imports " + (got && got[0] && got[0].length));
		if (got[0][0].n !== "astro/config") throw new Error("first " + got[0][0].n);
		if (got[0][1].n !== "@astrojs/svelte") throw new Error("second " + got[0][1].n);
	`
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeFSReaddirWithFileTypes(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		var names = fs.readdirSync("dir");
		if (names.indexOf("a.txt") < 0) throw new Error("names " + names);
		var ents = fs.readdirSync("dir", { withFileTypes: true });
		var a = null;
		var sub = null;
		for (var i = 0; i < ents.length; i++) {
			if (ents[i].name === "a.txt") a = ents[i];
			if (ents[i].name === "sub") sub = ents[i];
		}
		if (!a) throw new Error("missing a.txt");
		if (typeof a.isSymbolicLink !== "function") throw new Error("isSymbolicLink");
		if (!a.isFile()) throw new Error("a file");
		if (a.isDirectory()) throw new Error("a dir");
		if (a.isSymbolicLink()) throw new Error("a link");
		if (!sub || !sub.isDirectory()) throw new Error("sub");
		var n = 0;
		fs.readdir("dir", { withFileTypes: true }, function (err, got) {
			if (err) throw err;
			if (!got.some(function (e) { return e.name === "a.txt" && e.isFile(); })) throw new Error("async");
			n = 1;
		});
		setTimeout(function () {
			if (n !== 1) throw new Error("cb");
		}, 0);
	`)
}

func TestNodeFSCwdAbsoluteAndNative(t *testing.T) {
	iso := New("", Options{
		FS:      nodeFSMap(),
		Cwd:     "/guest/root",
		Imports: NodeScriptImports(),
	})
	err := iso.ScriptMain(t.Context(), `
		var fs = require("fs");
		if (typeof fs.realpathSync.native !== "function") throw new Error("native");
		if (fs.realpathSync.native !== fs.realpathSync) throw new Error("native ident");
		if (typeof fs.realpath.native !== "function") throw new Error("async native");
		if (fs.readFileSync("/guest/root/hello.txt", "utf8") !== "hi") throw new Error("abs read");
		if (fs.readFileSync("/hello.txt", "utf8") !== "hi") throw new Error("guest abs");
		if (!fs.statSync("/guest/root/dir").isDirectory()) throw new Error("dir");
		if (fs.realpathSync("/guest/root/hello.txt") !== "/guest/root/hello.txt") throw new Error("rp abs " + fs.realpathSync("/guest/root/hello.txt"));
		if (fs.realpathSync.native("hello.txt") !== "/guest/root/hello.txt") throw new Error("rp rel");
		if (fs.realpathSync(".") !== "/guest/root") throw new Error("rp cwd " + fs.realpathSync("."));
		if (fs.statSync("missing.txt", { throwIfNoEntry: false }) !== undefined) throw new Error("throwIfNoEntry");
		try {
			fs.statSync("missing.txt");
			throw new Error("stat should throw");
		} catch (e) {
			if (e.message === "stat should throw") throw e;
			if (e.code !== "ENOENT") throw new Error("stat " + e.code);
		}
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeFSAbsAfterChdir(t *testing.T) {
	iso := New("", Options{
		FS:      nodeFSMap(),
		Cwd:     "/guest/root",
		Imports: NodeScriptImports(),
	})
	err := iso.ScriptMain(t.Context(), `
		var fs = require("fs");
		process.chdir("/guest/root/dir");
		if (fs.readFileSync("/guest/root/hello.txt", "utf8") !== "hi") throw new Error("mount");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeFSExistsEnvBinary(t *testing.T) {
	iso := New("", Options{
		FS:      nodeFSMap(),
		Imports: NodeScriptImports(),
		ProcessEnv: map[string]string{
			"ESBUILD_BINARY_PATH": "/host/bin/esbuild",
		},
	})
	err := iso.ScriptMain(t.Context(), `
		var fs = require("fs");
		if (!fs.existsSync("/host/bin/esbuild")) throw new Error("env path");
		if (fs.existsSync("/host/bin/other")) throw new Error("other");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeFSPromisesIdentity(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var fs = require("fs");
		var p = require("fs/promises");
		if (require("node:fs/promises") !== p) throw new Error("node:fs/promises identity");
		if (fs.promises !== p) throw new Error("fs.promises identity");
		if (typeof p.readFile !== "function") throw new Error("readFile");
		if (typeof p.stat !== "function") throw new Error("stat");
	`)
}

func TestNodeFSPromisesReadFile(t *testing.T) {
	runNodeFS(t, nodeFSMap(), `
		var p = require("fs/promises");
		var got = "";
		var code = "";
		p.readFile("hello.txt", "utf8").then(function (s) { got = s; });
		p.readFile("missing.txt").then(function () {}, function (e) { code = e.code; });
		setTimeout(function () {
			if (got !== "hi") throw new Error("utf8 " + got);
			if (code !== "ENOENT") throw new Error("code " + code);
		}, 0);
	`)
}

func TestFileURLHasNUL(t *testing.T) {
	if !fileURLHasNUL("file:///C:/foo\x00bar") {
		t.Fatal("raw NUL")
	}
	if !fileURLHasNUL("file:///C:/foo%00bar") {
		t.Fatal("%00")
	}
	if fileURLHasNUL("file:///C:/foo") {
		t.Fatal("clean")
	}
}

func TestMapFSErr(t *testing.T) {
	code, _ := mapFSErr(fs.ErrNotExist)
	if code != "ENOENT" {
		t.Fatalf("got %s", code)
	}
	if !strings.Contains(errnoText("ENOENT"), "no such file") {
		t.Fatal("text")
	}
}
