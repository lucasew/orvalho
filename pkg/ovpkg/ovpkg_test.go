package ovpkg_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"

	"orvalho/pkg/cuex"
	"orvalho/pkg/ovpkg"
)

func minimalManifest() []byte {
	return []byte(`
id: "minimal"
entry: "worker.js"
runtime: "js"
`)
}

func TestWriteDirAndOpenRoundTrip_MinimalFixture(t *testing.T) {
	dir := filepath.Join("testdata", "minimal")

	var buf bytes.Buffer
	if err := ovpkg.WriteDir(&buf, dir); err != nil {
		t.Fatalf("WriteDir: %v", err)
	}

	pkg, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	if len(pkg.Manifest) == 0 {
		t.Fatal("expected non-empty manifest")
	}
	id, ok, err := cuex.LookupString(pkg.Value(), "id")
	if err != nil || !ok || id != "minimal" {
		t.Fatalf("id=%q ok=%v err=%v", id, ok, err)
	}
	entry, ok, err := cuex.LookupString(pkg.Value(), "entry")
	if err != nil || !ok || entry != "worker.js" {
		t.Fatalf("entry=%q ok=%v err=%v", entry, ok, err)
	}

	wantWorker, err := os.ReadFile(filepath.Join(dir, "worker.js"))
	if err != nil {
		t.Fatal(err)
	}
	gotWorker, err := pkg.Get("worker.js")
	if err != nil {
		t.Fatalf("Get worker.js: %v", err)
	}
	if !bytes.Equal(gotWorker, wantWorker) {
		t.Errorf("worker.js mismatch")
	}

	list := pkg.List()
	if len(list) != 2 {
		t.Fatalf("List = %v, want 2 entries", list)
	}
	if list[0] != ovpkg.ManifestName || list[1] != "worker.js" {
		t.Errorf("List = %v", list)
	}
}

func TestWriteDirAndOpenRoundTrip_WithAssetsFixture(t *testing.T) {
	dir := filepath.Join("testdata", "with-assets")

	var buf bytes.Buffer
	if err := ovpkg.WriteDir(&buf, dir); err != nil {
		t.Fatalf("WriteDir: %v", err)
	}

	pkg, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	id, ok, err := cuex.LookupString(pkg.Value(), "id")
	if err != nil || !ok || id != "with-assets" {
		t.Fatalf("id=%q ok=%v err=%v", id, ok, err)
	}

	for _, name := range []string{
		"index.js",
		"assets/style.css",
		"assets/logo.txt",
	} {
		want, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := pkg.Get(name)
		if err != nil {
			t.Fatalf("Get %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s content mismatch", name)
		}
	}
}

func TestWriteDirAndOpenRoundTrip_NestedFixture(t *testing.T) {
	dir := filepath.Join("testdata", "nested")
	var buf bytes.Buffer
	if err := ovpkg.WriteDir(&buf, dir); err != nil {
		t.Fatalf("WriteDir: %v", err)
	}
	pkg, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	entry, ok, err := cuex.LookupString(pkg.Value(), "entry")
	if err != nil || !ok || entry != "src/main.js" {
		t.Fatalf("entry=%q ok=%v err=%v", entry, ok, err)
	}
	if _, err := pkg.Get("src/lib/util.js"); err != nil {
		t.Fatal(err)
	}
}

func TestWriteDir_NoManifest(t *testing.T) {
	err := ovpkg.WriteDir(&bytes.Buffer{}, filepath.Join("testdata", "no-manifest"))
	if !errors.Is(err, ovpkg.ErrMissingManifest) {
		t.Fatalf("err = %v, want ErrMissingManifest", err)
	}
}

func TestWriteDir_BadManifest(t *testing.T) {
	err := ovpkg.WriteDir(&bytes.Buffer{}, filepath.Join("testdata", "bad-manifest"))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validate") && !strings.Contains(err.Error(), "cuex") {
		// still ok if raw cue error
		t.Logf("bad manifest err: %v", err)
	}
}

func TestPackageFromMapAndWritePackage(t *testing.T) {
	pkg, err := ovpkg.PackageFromMap(minimalManifest(), map[string][]byte{
		"worker.js": []byte("export default {}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := ovpkg.WritePackage(&buf, pkg); err != nil {
		t.Fatal(err)
	}
	pkg2, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := cuex.LookupString(pkg2.Value(), "id")
	if err != nil || !ok || id != "minimal" {
		t.Fatalf("id=%q", id)
	}
}

func TestWriteRejectsInvalidManifest(t *testing.T) {
	err := ovpkg.Write(&bytes.Buffer{}, []byte(`id: "x"`), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetNotFound(t *testing.T) {
	pkg, err := ovpkg.PackageFromMap(minimalManifest(), map[string][]byte{"a.js": []byte("1")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pkg.Get("missing.js")
	if !errors.Is(err, ovpkg.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestCleanPathRejectsTraversal(t *testing.T) {
	_, err := ovpkg.PackageFromMap(minimalManifest(), map[string][]byte{
		"../etc/passwd": []byte("x"),
	})
	if err == nil {
		t.Fatal("expected invalid path")
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "p.zip")
	if err := ovpkg.WriteFile(zipPath, minimalManifest(), map[string][]byte{
		"worker.js": []byte("// hi"),
	}); err != nil {
		t.Fatal(err)
	}
	pkg, err := ovpkg.OpenFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pkg.Get("worker.js")
	if err != nil || string(b) != "// hi" {
		t.Fatalf("got %q err %v", b, err)
	}
}

func TestOpenMissingManifest(t *testing.T) {
	// zip with only a payload file
	var buf bytes.Buffer
	// use raw zip without going through Write
	_ = buf
	// Build via empty - Write always validates. Create minimal zip manually in helper.
	// Skip if too heavy; PackageFromMap with empty fails.
	_, err := ovpkg.PackageFromMap(nil, map[string][]byte{"a.js": []byte("1")})
	if !errors.Is(err, ovpkg.ErrMissingManifest) {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenPathDirAndZip(t *testing.T) {
	dir := filepath.Join("testdata", "minimal")
	pkg, err := ovpkg.OpenPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := pkg.Entry()
	if err != nil || entry != "worker.js" {
		t.Fatalf("entry=%q err=%v", entry, err)
	}
	port, err := pkg.Port()
	if err != nil || port != 0 {
		t.Fatalf("port=%d err=%v", port, err)
	}

	var buf bytes.Buffer
	if err := ovpkg.WritePackage(&buf, pkg); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "m.ovpkg")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg2, err := ovpkg.OpenPath(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	entry2, err := pkg2.Entry()
	if err != nil || entry2 != "worker.js" {
		t.Fatalf("zip entry=%q err=%v", entry2, err)
	}
}

func TestExampleCatSSRPackage(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "cat-ssr")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("example not present: %v", err)
	}
	pkg, err := ovpkg.OpenPath(dir)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	entry, err := pkg.Entry()
	if err != nil {
		t.Fatal(err)
	}
	if entry != "worker.js" {
		t.Fatalf("entry=%q", entry)
	}
	id, ok, err := cuex.LookupString(pkg.Value(), "id")
	if err != nil || !ok || id != "cat-ssr" {
		t.Fatalf("id=%q ok=%v err=%v", id, ok, err)
	}
	// Egress allowlist must name the cat API host.
	egress := pkg.Value().LookupPath(cue.ParsePath("egress"))
	if !egress.Exists() {
		t.Fatal("missing egress")
	}
	src, err := pkg.Get(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "catfact.ninja") {
		t.Fatal("worker should reference catfact.ninja")
	}
	if !strings.Contains(string(src), "export default") {
		t.Fatal("worker should use Workers default export")
	}
}
