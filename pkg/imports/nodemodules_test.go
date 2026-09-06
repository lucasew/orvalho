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

func TestNodeModulesMiss(t *testing.T) {
	t.Parallel()
	if _, ok := (NodeModules{FS: tree(nil)}).Lookup("lodash"); ok {
		t.Fatal("empty tree must miss")
	}
	if _, ok := (NodeModules{FS: tree(map[string]string{"events/index.js": ``})}).Lookup("node:events"); ok {
		t.Fatal("scheme specifier is not a file lookup")
	}
}
