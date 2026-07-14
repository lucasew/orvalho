package cuex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPackageMinimal(t *testing.T) {
	cfg, err := LoadPackage([]byte(`
id: "cat-ssr"
entry: "dist/worker.js"
runtime: "js"
`), "orvalho.cue")
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := LookupString(cfg.Value, "id")
	if err != nil || !ok || id != "cat-ssr" {
		t.Fatalf("id=%q ok=%v err=%v", id, ok, err)
	}
}

func TestLoadPackageRejectsOpenProxy(t *testing.T) {
	_, err := LoadPackage([]byte(`
id: "x"
entry: "a.js"
runtime: "js"
egress: ["*"]
`), "orvalho.cue")
	if err == nil {
		t.Fatal("expected error for bare * egress")
	}
}

func TestLoadPackageRequiresFields(t *testing.T) {
	_, err := LoadPackage([]byte(`id: "x"`), "orvalho.cue")
	if err == nil {
		t.Fatal("expected missing entry/runtime error")
	}
}

func TestLoadHostEmpty(t *testing.T) {
	cfg, err := LoadHost(nil, "orvalho.cue")
	if err != nil {
		t.Fatal(err)
	}
	listen, ok, err := LookupString(cfg.Value, "http.listen")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || listen != "127.0.0.1:7840" {
		t.Fatalf("listen=%q ok=%v", listen, ok)
	}
}

func TestLoadHostIdentity(t *testing.T) {
	cfg, err := LoadHost([]byte(`
role: "manager"
identity: { keyPath: "manager.key" }
`), "orvalho.cue")
	if err != nil {
		t.Fatal(err)
	}
	p, ok, err := LookupString(cfg.Value, "identity.keyPath")
	if err != nil || !ok || p != "manager.key" {
		t.Fatalf("keyPath=%q ok=%v err=%v", p, ok, err)
	}
}

func TestLoadHostDataDir(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadHostDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("nil cfg")
	}
	path := filepath.Join(dir, InstanceFilename)
	if err := os.WriteFile(path, []byte("role: \"worker\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadHostDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	role, ok, err := LookupString(cfg2.Value, "role")
	if err != nil || !ok || role != "worker" {
		t.Fatalf("role=%q ok=%v err=%v", role, ok, err)
	}
}
