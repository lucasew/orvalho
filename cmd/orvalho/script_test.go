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

func TestProcessEnvMapHome(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	env := processEnvMap()
	if env["HOME"] != "/" {
		t.Fatalf("HOME=%q want /", env["HOME"])
	}
}

func TestHostTreeFSWriteAndEscape(t *testing.T) {
	dir := t.TempDir()
	h := newHostTreeFS(dir)
	if err := h.WriteFile("a.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("write: %q %v", got, err)
	}
	if err := h.Mkdir(".config/astro", 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, ".config", "astro"))
	if err != nil || !st.IsDir() {
		t.Fatalf("mkdir: %v", err)
	}
	if err := h.WriteFile("../escape.txt", []byte("no"), 0o644); err == nil {
		t.Fatal("want escape on write")
	}
	if err := h.Mkdir("../escape-dir", 0o755); err == nil {
		t.Fatal("want escape on mkdir")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); err == nil {
		t.Fatal("wrote outside root")
	}
	if err := h.Remove("a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove: %v", err)
	}
}

func TestRunScriptFileWriteFS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.js")
	src := `
		var fs = require("fs");
		var os = require("os");
		var path = require("path");
		if (os.homedir() === "/home/someone") throw new Error("host home");
		fs.mkdirSync(os.homedir() + "/.config/astro");
		fs.writeFileSync("out.txt", "ok");
		if (fs.readFileSync("out.txt", "utf8") !== "ok") throw new Error("read");
		var abs = path.join(process.cwd(), "out.txt");
		if (fs.readFileSync(abs, "utf8") !== "ok") throw new Error("abs read");
		if (typeof fs.realpathSync.native !== "function") throw new Error("native");
		if (fs.realpathSync.native(abs) !== abs.replace(/\\/g, "/")) throw new Error("rp " + fs.realpathSync(abs));
	`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil || string(got) != "ok" {
		t.Fatalf("out: %q %v", got, err)
	}
	if st, err := os.Stat(filepath.Join(dir, ".config", "astro")); err != nil || !st.IsDir() {
		t.Fatalf("guest home mkdir: %v", err)
	}
}

func TestHostTreeFSRealpathFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	h := newHostTreeFS(dir)
	got, err := h.Realpath("link/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "real/a.txt" {
		t.Fatalf("realpath=%q", got)
	}
}

func TestRunScriptFileSucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.js")
	if err := os.WriteFile(path, []byte(`var x = 1;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunScriptFileShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.js")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env node\nvar x = 1;\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEvalFSRelFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	slot := filepath.Join(dir, "node_modules", ".orvalho", "pkg@1.0.0", "node_modules", "pkg")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slot, "index.js"), []byte("exports.n=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".orvalho", "pkg@1.0.0", "node_modules", "pkg"), filepath.Join(dir, "node_modules", "pkg")); err != nil {
		t.Fatal(err)
	}
	got := evalFSRel(dir, "node_modules/pkg/index.js")
	want := "node_modules/.orvalho/pkg@1.0.0/node_modules/pkg/index.js"
	if got != want {
		t.Fatalf("evalFSRel=%q want %q", got, want)
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
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunScriptFileViteImportMetaPackageJSON(t *testing.T) {
	dir := t.TempDir()
	chunk := filepath.Join(dir, "vite", "dist", "node", "chunks")
	if err := os.MkdirAll(chunk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vite", "package.json"), []byte(`{"version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same URL walk Vite's logger.js uses against import.meta.url.
	logger := `
import { readFileSync } from "fs";
const { version } = JSON.parse(readFileSync(new URL("../../package.json", new URL("../../../src/node/constants.ts", import.meta.url))).toString());
export { version };
`
	if err := os.WriteFile(filepath.Join(chunk, "logger.js"), []byte(logger), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `
import { version } from "./vite/dist/node/chunks/logger.js";
if (version !== "1.2.3") throw new Error("version " + version);
`
	path := filepath.Join(dir, "main.mjs")
	if err := os.WriteFile(path, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunScriptFileESM(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.mjs"), []byte("export const n = 7;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "main.mjs")
	src := "import { n } from './lib.mjs';\nif (n !== 7) throw new Error('n');\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runScriptFile(t.Context(), dir, path, nil); err != nil {
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
