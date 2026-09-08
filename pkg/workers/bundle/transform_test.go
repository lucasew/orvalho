package bundle_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lucasew/orvalho/pkg/workers/bundle"
)

func TestTransformCJSRewritesCopyProps(t *testing.T) {
	out, err := bundle.TransformCJS("import { execFile } from \"node:child_process\";\nexport const f = execFile;\n", "mod.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "var names = __getOwnPropNames(from);") {
		i := strings.Index(out, "__copyProps")
		dump := out
		if i >= 0 {
			end := i + 700
			if end > len(out) {
				end = len(out)
			}
			dump = out[i:end]
		}
		t.Fatalf("index-loop rewrite missing; __copyProps block:\n%s", dump)
	}
	if strings.Contains(out, "for (let key of __getOwnPropNames(from))") {
		t.Fatalf("for-of __copyProps survived:\n%s", out)
	}
	if !strings.Contains(out, "for (var i = 0; i < names.length; i++)") {
		t.Fatalf("index loop missing:\n%s", out)
	}
	if !strings.Contains(out, `get: ((k) => from[k]).bind(null, key)`) {
		t.Fatalf("key-binding getter missing:\n%s", out)
	}
}

func TestTransformCJSDropsRegexpDFlag(t *testing.T) {
	out, err := bundle.TransformCJS("export const r = new RegExp('x', 'dg');\n", "mod.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"dg"`) || strings.Contains(out, `'dg'`) {
		t.Fatalf("d flag survived: %s", out)
	}
}

func TestTransformCJSOptionalCatch(t *testing.T) {
	out, err := bundle.TransformCJS("try { x(); } catch { y(); }\nexport const n = 1;\n", "mod.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "catch {") {
		t.Fatalf("optional catch survived: %s", out)
	}
	if !strings.Contains(out, "exports") {
		t.Fatalf("expected CJS exports: %s", out)
	}
}

func TestCompileCJSBundlesSibling(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.mjs")
	main := filepath.Join(dir, "main.mjs")
	if err := os.WriteFile(lib, []byte("export const n = 7;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "import { n } from './lib.mjs';\nif (n !== 7) throw new Error('n');\n"
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "from './lib") || strings.Contains(out, `from "./lib`) {
		t.Fatalf("sibling import survived: %s", out)
	}
	if !strings.Contains(out, "7") {
		t.Fatalf("bundled lib missing: %s", out)
	}
}

func TestCompileCJSRewritesImportCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dyn.mjs")
	src := "export async function f() { return import('fs'); }\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, path)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(^|[^.\w$])import\(`).MatchString(out) {
		t.Fatalf("import() survived: %s", out)
	}
}

func TestCompileCJSRewritesIDContinue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "re.mjs")
	src := "export const r = /[$_\\u200C\\u200D\\p{ID_Continue}]/u;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ID_Continue") {
		t.Fatalf("ID_Continue survived: %s", out)
	}
	if !strings.Contains(out, `\p{L}`) {
		t.Fatalf("expected \\p{L}: %s", out)
	}
}

func TestCompileCJSASCIICharset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "u.mjs")
	src := "export const s = \"-\\u{10FFFF}\";\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `\u{`) {
		t.Fatalf("ES6 unicode escape survived: %s", out)
	}
}

func TestTransformCJSImportMetaURL(t *testing.T) {
	out, err := bundle.TransformCJS("export const u = import.meta.url;\n", "pkg/dist/file.js")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`import_meta\d*\s*=\s*\{\s*\}`).MatchString(out) {
		t.Fatalf("empty import_meta survived: %s", out)
	}
	if !strings.Contains(out, "__orvalhoFileURL(__orvalhoFilename)") {
		t.Fatalf("expected file URL from __orvalhoFilename: %s", out)
	}
}

func TestCompileCJSImportMetaURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.mjs")
	src := "import { createRequire } from 'module';\nexport const req = createRequire(import.meta.url);\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, path)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`import_meta\d*\s*=\s*\{\s*\}`).MatchString(out) {
		t.Fatalf("empty import_meta survived: %s", out)
	}
	if !strings.Contains(out, "__orvalhoFileURL(__orvalhoFilename)") {
		t.Fatalf("expected file URL from __orvalhoFilename: %s", out)
	}
}

func TestCompileCJSExportOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "only.mjs")
	src := "export const n = 1;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := bundle.CompileCJS(src, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "export const") || strings.Contains(out, "export {") {
		t.Fatalf("export survived: %s", out)
	}
	if !strings.Contains(out, "exports") {
		t.Fatalf("expected CJS exports: %s", out)
	}
}
