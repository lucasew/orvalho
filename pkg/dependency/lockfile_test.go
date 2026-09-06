package dependency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageNameFromPath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"node_modules/leftpad":                "leftpad",
		"node_modules/@scope/pkg":             "@scope/pkg",
		"node_modules/foo/node_modules/bar":   "bar",
		"node_modules/@a/b/node_modules/@c/d": "@c/d",
	}
	for in, want := range cases {
		if got := packageNameFromPath(in); got != want {
			t.Fatalf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestReadWriteLockfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	g := &Graph{
		Name:            "app",
		Version:         "1.0.0",
		LockfileVersion: 3,
		Packages: map[string]lockPackage{
			"": {Name: "app", Version: "1.0.0", Dependencies: map[string]string{"leftpad": "^1.3.0"}},
			"node_modules/leftpad": {
				Name:      "leftpad",
				Version:   "1.3.0",
				Resolved:  "https://registry.npmjs.org/leftpad/-/leftpad-1.3.0.tgz",
				Integrity: "sha512-abc",
			},
		},
	}
	path := filepath.Join(dir, LockfileName)
	if err := WriteLockfile(path, g); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLockfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "app" || len(got.Nodes) != 1 || got.Nodes[0].Name != "leftpad" {
		t.Fatalf("%+v", got)
	}
	if got.Root["leftpad"] != "^1.3.0" {
		t.Fatalf("root %v", got.Root)
	}
}

func TestInstallRefusesYarnLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte("# yarn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Install(t.Context(), Options{Dir: dir})
	if err == nil {
		t.Fatal("expected lockfile error")
	}
}
