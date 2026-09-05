package imports

import (
	"errors"
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

func TestNodeModulesAtRoot(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"leftpad/package.json": `{"main":"index.js"}`,
		"leftpad/index.js":     `exports.n=1`,
	})
	got, err := Resolve("leftpad", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "leftpad/index.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesUnderNodeModulesDir(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"node_modules/leftpad/index.js": `exports.n=1`,
	})
	got, err := Resolve("leftpad", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "node_modules/leftpad/index.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesPackageJSONMain(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"pkg/package.json": `{"main":"lib/entry.js"}`,
		"pkg/lib/entry.js": `exports.n=1`,
	})
	got, err := Resolve("pkg", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "pkg/lib/entry.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesSubpath(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"pkg/package.json": `{}`,
		"pkg/lib/x.js":     `exports.n=1`,
	})
	got, err := Resolve("pkg/lib/x", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "pkg/lib/x.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesScoped(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"@scope/pkg/package.json": `{"main":"index.js"}`,
		"@scope/pkg/index.js":     `exports.n=1`,
	})
	got, err := Resolve("@scope/pkg", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "@scope/pkg/index.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesExistingFilePath(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		"pkg/lib.js": ``,
	})
	got, err := Resolve("pkg/lib.js", NodeModules{FS: fsys})
	if err != nil {
		t.Fatal(err)
	}
	if got != "pkg/lib.js" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesMissesCallNext(t *testing.T) {
	t.Parallel()
	got, err := Resolve("missing",
		NodeModules{FS: tree(nil)},
		Func[string](func(spec string, next Resolver[string]) (string, error) {
			if spec == "missing" {
				return "fallback", nil
			}
			return next(spec)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback" {
		t.Fatalf("got %q", got)
	}
}

func TestNodeModulesLeavesSchemesToNext(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{"events/index.js": ``})
	got, err := Resolve("node:events",
		NodeModules{FS: fsys},
		Map[string]{"node:events": "builtin"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "builtin" {
		t.Fatalf("got %q want builtin", got)
	}
}

func TestNodeModulesMissingIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := Resolve("lodash", NodeModules{FS: tree(nil)})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
