package js_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	actorjs "orvalho/pkg/actor/js"
)

func TestNeedsBundle(t *testing.T) {
	if actorjs.NeedsBundle(`export default { fetch() {} }`) {
		t.Fatal("single-file export default without import should not need bundle")
	}
	if !actorjs.NeedsBundle(`import { w } from "./chunks/x.mjs"; export { w as default };`) {
		t.Fatal("esm import should need bundle")
	}
}

func TestBundleEntrySimple(t *testing.T) {
	if _, err := exec.LookPath("esbuild"); err != nil {
		t.Skip("esbuild not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.js"), []byte(`export function hi() { return "hi"; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entry.mjs"), []byte(`
import { hi } from "./lib.js";
export default {
  async fetch(request, env, ctx) {
    return new Response(hi(), { status: 200 });
  }
};
`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := actorjs.BundleEntry(actorjs.BundleOptions{
		PackageDir: dir,
		Entry:      "entry.mjs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "__orvalhoWorker") {
		t.Fatal("expected IIFE global name in bundle")
	}
	if !strings.Contains(out, "globalThis.default") {
		t.Fatal("expected default export footer")
	}
	// Should inline hi()
	if !strings.Contains(out, "hi") {
		t.Fatal("expected lib symbol in bundle")
	}
}
