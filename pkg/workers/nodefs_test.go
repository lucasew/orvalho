package workers

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
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
			"readdirSync", "readlinkSync", "realpathSync", "rmdirSync",
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
