package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRewriteNodeInvocation(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "orvalho", in: []string{"orvalho", "script", "run", "a.js"}, want: []string{"orvalho", "script", "run", "a.js"}},
		{name: "node file", in: []string{"/tmp/orvalho-node-1/node", "a.js"}, want: []string{"/tmp/orvalho-node-1/node", "script", "run", "a.js"}},
		{name: "bare node", in: []string{"node"}, want: []string{"node", "script", "run"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteNodeInvocation(tt.in)
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Fatalf("rewriteNodeInvocation(%q) = %q want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveScriptTargetFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.js")
	if err := os.WriteFile(path, []byte(`var x = 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	file, shell, err := resolveScriptTarget(dir, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	if file != path {
		t.Fatalf("file=%q want %q", file, path)
	}
	if shell != "" {
		t.Fatalf("shell=%q want empty", shell)
	}
}

func TestResolveScriptTargetPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"hello":"node main.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	file, shell, err := resolveScriptTarget(dir, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if file != "" {
		t.Fatalf("file=%q want empty", file)
	}
	if shell != "node main.js" {
		t.Fatalf("shell=%q want node main.js", shell)
	}
}

func TestResolveScriptTargetMissing(t *testing.T) {
	dir := t.TempDir()
	_, _, err := resolveScriptTarget(dir, "nope")
	if !errors.Is(err, ErrScriptMissing) {
		t.Fatalf("got %v want ErrScriptMissing", err)
	}
}

func TestWithPathPrefix(t *testing.T) {
	sep := string(os.PathListSeparator)
	got := withPathPrefix([]string{"FOO=1", "PATH=/bin"}, "/tmp/nodebin", "/tmp/proj/node_modules/.bin")
	wantPATH := "PATH=/tmp/nodebin" + sep + "/tmp/proj/node_modules/.bin" + sep + "/bin"
	found := false
	for _, e := range got {
		if e == "FOO=1" {
			continue
		}
		if e == wantPATH {
			found = true
			continue
		}
		t.Fatalf("unexpected env %q", e)
	}
	if !found {
		t.Fatalf("PATH prefix missing: %q", got)
	}
}

func TestRunScriptFileSucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.js")
	if err := os.WriteFile(path, []byte(`var x = 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path); err != nil {
		t.Fatal(err)
	}
}

func TestRunScriptFileRequire(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "node_modules", "leftpad")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "package.json"), []byte(`{"main":"index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "index.js"), []byte(`exports.pad = function (s) { return "0" + s; };`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.js")
	if err := os.WriteFile(path, []byte(`var p = require("leftpad"); if (p.pad("1") !== "01") throw new Error("bad");`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path); err != nil {
		t.Fatal(err)
	}
}

func TestScriptRunCLIFile(t *testing.T) {
	exe := orvalhoExe(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte(`var x = 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "script", "run", "main.js")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script run: %v\n%s", err, out)
	}
}

func TestScriptRunCLIMissing(t *testing.T) {
	exe := orvalhoExe(t)
	cmd := exec.Command(exe, "script", "run", "nope.js")
	cmd.Dir = t.TempDir()
	err := cmd.Run()
	if err == nil {
		t.Fatal("want exit 1 for missing file")
	}
}

func TestScriptRunCLIPackageJSONBinPath(t *testing.T) {
	exe := orvalhoExe(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"hello":"tool"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(binDir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho from-bin\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "script", "run", "hello")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script run hello: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "from-bin") {
		t.Fatalf("output %q", out)
	}
}

func TestScriptRunCLIPackageJSONNodeTrampoline(t *testing.T) {
	exe := orvalhoExe(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"hello":"node main.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// getBuiltinModule is isolate-only; host node would throw ReferenceError.
	src := `if (typeof getBuiltinModule !== "function") throw new Error("host node");`
	if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "script", "run", "hello")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script run hello: %v\n%s", err, out)
	}
}

var builtOrvalho = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "orvalho-script-test-*")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "orvalho")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
})

func orvalhoExe(t *testing.T) string {
	t.Helper()
	p, err := builtOrvalho()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
