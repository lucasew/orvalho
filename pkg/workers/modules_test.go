package workers

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dop251/goja"
	"github.com/lucasew/orvalho/pkg/imports"
)

func importMap(m map[string]Binding) []Import {
	return []Import{Map(m)}
}

type pingObject struct{}

func (pingObject) Build(rt *goja.Runtime) (*goja.Object, error) {
	o := rt.NewObject()
	if err := o.Set("ping", func() string { return "pong" }); err != nil {
		return nil, err
	}
	return o, nil
}

type countBinding struct {
	n int
}

func (c *countBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	c.n++
	o := iso.vm.NewObject()
	if err := o.Set("n", c.n); err != nil {
		return nil, err
	}
	return o, nil
}

func tickOK(t *testing.T, iso *Isolate) {
	t.Helper()
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestRequireHostBinding(t *testing.T) {
	iso := New(`var got = require("orvalho:rebimboca-da-parafuseta").ping();`, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
		}),
	})
	tickOK(t, iso)
	if got := iso.vm.Get("got").String(); got != "pong" {
		t.Fatalf("got=%q want pong", got)
	}
}

func TestRequireScriptUsesInjectedBinding(t *testing.T) {
	iso := New(`var got = require("orvalho:shim").ping();`, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
			"orvalho:shim": NewScriptBinding(`
				var host = require("orvalho:rebimboca-da-parafuseta");
				exports.ping = function () { return host.ping(); };
			`),
		}),
	})
	tickOK(t, iso)
	if got := iso.vm.Get("got").String(); got != "pong" {
		t.Fatalf("got=%q want pong", got)
	}
}

func TestRequireCacheIdentity(t *testing.T) {
	iso := New(`
		var a = require("orvalho:rebimboca-da-parafuseta");
		var b = require("orvalho:rebimboca-da-parafuseta");
		var c = getBuiltinModule("orvalho:rebimboca-da-parafuseta");
		var same = a === b && b === c;
	`, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
		}),
	})
	tickOK(t, iso)
	if !iso.vm.Get("same").ToBoolean() {
		t.Fatal("require/getBuiltinModule must return the same cached object")
	}
}

func TestImportNotMaterializedUntilRequire(t *testing.T) {
	c := &countBinding{}
	iso := New(`var ran = true;`, Options{
		Imports: importMap(map[string]Binding{"orvalho:count": c}),
	})
	tickOK(t, iso)
	if c.n != 0 {
		t.Fatalf("Materialize ran at New/Tick without require: n=%d", c.n)
	}
	iso2 := New(`require("orvalho:count"); require("orvalho:count");`, Options{
		Imports: importMap(map[string]Binding{"orvalho:count": c}),
	})
	tickOK(t, iso2)
	if c.n != 1 {
		t.Fatalf("Materialize calls=%d want 1", c.n)
	}
}

func TestRequireMissingIsNotFound(t *testing.T) {
	iso := New(`require("orvalho:missing");`, Options{})
	_, err := iso.Tick(t.Context())
	if err == nil || !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("want ErrModuleNotFound, got %v", err)
	}
}

func TestEnvBindingIsNotAnImport(t *testing.T) {
	iso := New(`require("orvalho:rebimboca-da-parafuseta");`, Options{
		Bindings: map[string]Binding{
			"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
		},
	})
	_, err := iso.Tick(t.Context())
	if err == nil || !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("env Binding must not satisfy require, got %v", err)
	}
}

func TestRequireRejectsHostPathSpecifiers(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"",
		"/etc/passwd",
		"node:",
		"Node:events",
		"orvalho:/abs",
		"orvalho:../x",
		"orvalho:foo/../bar",
		"orvalho:foo//bar",
	}
	for _, spec := range invalid {
		t.Run("invalid/"+spec, func(t *testing.T) {
			t.Parallel()
			if imports.Valid(spec) {
				t.Fatalf("%q must not be a valid specifier", spec)
			}
			iso := New(`require(`+jsString(spec)+`);`, Options{})
			_, err := iso.Tick(t.Context())
			if err == nil || !errors.Is(err, ErrModuleSpecifier) {
				t.Fatalf("require(%q): want ErrModuleSpecifier, got %v", spec, err)
			}
		})
	}
}

func TestRequireCircularScripts(t *testing.T) {
	iso := New(`
		var a = require("orvalho:a");
		var ok = a.fromA === 1 && a.b.fromB === 2 && a.b.aSeen === 1;
	`, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:a": NewScriptBinding(`
				exports.fromA = 1;
				exports.b = require("orvalho:b");
			`),
			"orvalho:b": NewScriptBinding(`
				exports.fromB = 2;
				exports.aSeen = require("orvalho:a").fromA;
			`),
		}),
	})
	tickOK(t, iso)
	if !iso.vm.Get("ok").ToBoolean() {
		t.Fatal("circular require did not see partial exports")
	}
}

func TestRequireScriptReplacesExports(t *testing.T) {
	iso := New(`var got = require("orvalho:fn")();`, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:fn": NewScriptBinding(`module.exports = function () { return 7; };`),
		}),
	})
	tickOK(t, iso)
	if n := iso.vm.Get("got").ToInteger(); n != 7 {
		t.Fatalf("got=%d want 7", n)
	}
}

func TestRequireNoAmbientHostIO(t *testing.T) {
	iso := New(`
		var fsGlobal = typeof fs;
		var missing = false;
		var bare = false;
		var rel = false;
		try { require("node:fs"); } catch (e) { missing = true; }
		try { require("fs"); } catch (e) { bare = true; }
		try { require("./x"); } catch (e) { rel = true; }
	`, Options{})
	tickOK(t, iso)
	if iso.vm.Get("fsGlobal").String() != "undefined" {
		t.Fatal("ambient fs global must be absent")
	}
	if !iso.vm.Get("missing").ToBoolean() || !iso.vm.Get("bare").ToBoolean() || !iso.vm.Get("rel").ToBoolean() {
		t.Fatal("uninjected host I/O specifiers must throw")
	}
}

func TestRequireFromFetch(t *testing.T) {
	src := PrepareGuestScript(`
export default {
  async fetch() {
    return new Response(require("orvalho:rebimboca-da-parafuseta").ping());
  }
};
`)
	iso := New(src, Options{
		Imports: importMap(map[string]Binding{
			"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
		}),
	})
	res, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://x/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "pong" {
		t.Fatalf("body=%q want pong", res.Body)
	}
}

func TestRequireSchemeHandlerIsLazy(t *testing.T) {
	var seen []string
	iso := New(`var got = require("orvalho:rebimboca-da-parafuseta").ping();`, Options{
		Imports: []Import{
			Scheme{Name: "orvalho", Load: func(spec string) (Binding, error) {
				seen = append(seen, spec)
				if spec != "orvalho:rebimboca-da-parafuseta" {
					return nil, imports.ErrNotFound
				}
				return Bind(pingObject{}), nil
			}},
		},
	})
	tickOK(t, iso)
	if got := iso.vm.Get("got").String(); got != "pong" {
		t.Fatalf("got=%q want pong", got)
	}
	if len(seen) != 1 || seen[0] != "orvalho:rebimboca-da-parafuseta" {
		t.Fatalf("scheme load called with %v", seen)
	}
}

func TestRequireAliasHandler(t *testing.T) {
	iso := New(`var got = require("orvalho:buf").ping();`, Options{
		Imports: []Import{
			Alias{From: "orvalho:buf", To: "orvalho:rebimboca-da-parafuseta"},
			Map{
				"orvalho:rebimboca-da-parafuseta": Bind(pingObject{}),
			},
		},
	})
	tickOK(t, iso)
	if got := iso.vm.Get("got").String(); got != "pong" {
		t.Fatalf("got=%q want pong", got)
	}
}

func TestRequireNodeModules(t *testing.T) {
	fsys := fstest.MapFS{
		"leftpad/package.json": {Data: []byte(`{"main":"index.js"}`)},
		"leftpad/index.js":     {Data: []byte(`exports.pad = function (s) { return "0" + s; };`)},
	}
	iso := New(`var got = require("leftpad").pad("1");`, Options{
		Imports: []Import{NodeModules{FS: fsys}},
	})
	tickOK(t, iso)
	if got := iso.vm.Get("got").String(); got != "01" {
		t.Fatalf("got=%q want 01", got)
	}
}

func TestRequireNodeModulesRelativeInsidePackage(t *testing.T) {
	fsys := fstest.MapFS{
		"pkg/package.json": {Data: []byte(`{"main":"index.js"}`)},
		"pkg/index.js":     {Data: []byte(`module.exports = require("./lib");`)},
		"pkg/lib.js":       {Data: []byte(`module.exports = 4;`)},
	}
	iso := New(`var got = require("pkg");`, Options{
		Imports: []Import{NodeModules{FS: fsys}},
	})
	tickOK(t, iso)
	if n := iso.vm.Get("got").ToInteger(); n != 4 {
		t.Fatalf("got=%d want 4", n)
	}
}

func TestRequireNodeModulesDir(t *testing.T) {
	fsys := fstest.MapFS{
		"node_modules/leftpad/index.js": {Data: []byte(`exports.n = 9;`)},
	}
	iso := New(`var got = require("leftpad").n;`, Options{
		Imports: []Import{NodeModules{FS: fsys}},
	})
	tickOK(t, iso)
	if n := iso.vm.Get("got").ToInteger(); n != 9 {
		t.Fatalf("got=%d want 9", n)
	}
}

func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
