package imports

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func tree(files map[string]string) fs.FS {
	m := fstest.MapFS{}
	for p, body := range files {
		m[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

func lookupOK(t *testing.T, fsys fs.FS, spec, want string) {
	t.Helper()
	got, ok := (NodeModules{FS: fsys}).Lookup(spec)
	if !ok {
		t.Fatalf("Lookup(%q) miss", spec)
	}
	if got != want {
		t.Fatalf("Lookup(%q)=%q want %q", spec, got, want)
	}
}

func TestNodeModulesAtRoot(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"leftpad/package.json": `{"main":"index.js"}`,
		"leftpad/index.js":     `exports.n=1`,
	}), "leftpad", "leftpad/index.js")
}

func TestNodeModulesUnderNodeModulesDir(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"node_modules/leftpad/index.js": `exports.n=1`,
	}), "leftpad", "node_modules/leftpad/index.js")
}

func TestNodeModulesPackageJSONMain(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"pkg/package.json": `{"main":"lib/entry.js"}`,
		"pkg/lib/entry.js": `exports.n=1`,
	}), "pkg", "pkg/lib/entry.js")
}

func TestNodeModulesSubpath(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"pkg/package.json": `{}`,
		"pkg/lib/x.js":     `exports.n=1`,
	}), "pkg/lib/x", "pkg/lib/x.js")
}

func TestNodeModulesScoped(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"@scope/pkg/package.json": `{"main":"index.js"}`,
		"@scope/pkg/index.js":     `exports.n=1`,
	}), "@scope/pkg", "@scope/pkg/index.js")
}

func TestNodeModulesExistingFilePath(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"pkg/lib.js": ``,
	}), "pkg/lib.js", "pkg/lib.js")
}

func TestNodeModulesExportsRequire(t *testing.T) {
	t.Parallel()
	lookupOK(t, tree(map[string]string{
		"pkg/package.json": `{"exports":{"require":"./cjs.js","import":"./esm.js"}}`,
		"pkg/cjs.js":       `exports.n=1`,
		"pkg/esm.js":       `export const n=2`,
	}), "pkg", "pkg/cjs.js")
}

func TestNodeModulesExportsSubpathClosed(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"pkg/package.json": `{"exports":{".":"./index.js"}}`,
		"pkg/index.js":     `exports.n=1`,
		"pkg/secret.js":    `exports.n=2`,
	})
	if _, ok := (NodeModules{FS: fsys}).Lookup("pkg/secret"); ok {
		t.Fatal("exports must hide secret.js")
	}
}

func TestNodeModulesClimbFromNested(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"node_modules/.orvalho/foo@1.0.0/node_modules/foo/index.js": `require("bar")`,
		"node_modules/.orvalho/foo@1.0.0/node_modules/bar/index.js": `exports.n=1`,
		"node_modules/.orvalho/bar@2.0.0/node_modules/bar/index.js": `exports.n=2`,
	})
	got, ok := (NodeModules{
		FS:   fsys,
		From: "node_modules/.orvalho/foo@1.0.0/node_modules/foo/index.js",
	}).Lookup("bar")
	if !ok {
		t.Fatal("miss")
	}
	if got != "node_modules/.orvalho/foo@1.0.0/node_modules/bar/index.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesMiss(t *testing.T) {
	t.Parallel()
	if _, ok := (NodeModules{FS: tree(nil)}).Lookup("lodash"); ok {
		t.Fatal("empty tree must miss")
	}
	if _, ok := (NodeModules{FS: tree(map[string]string{"events/index.js": ``})}).Lookup("node:events"); ok {
		t.Fatal("scheme specifier is not a file lookup")
	}
}
