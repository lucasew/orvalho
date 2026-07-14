package ovpkg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"orvalho/pkg/ovpkg"
)

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

	// Manifest path and soft JSON
	if len(pkg.Manifest) == 0 {
		t.Fatal("expected non-empty manifest")
	}
	var manifest map[string]any
	if err := pkg.UnmarshalManifest(&manifest); err != nil {
		t.Fatalf("UnmarshalManifest: %v", err)
	}
	if manifest["id"] != "hello" {
		t.Errorf("id = %v, want hello", manifest["id"])
	}
	if manifest["entry"] != "worker.js" {
		t.Errorf("entry = %v, want worker.js", manifest["entry"])
	}

	// Payload file contents
	wantWorker, err := os.ReadFile(filepath.Join(dir, "worker.js"))
	if err != nil {
		t.Fatal(err)
	}
	gotWorker, err := pkg.Get("worker.js")
	if err != nil {
		t.Fatalf("Get worker.js: %v", err)
	}
	if !bytes.Equal(gotWorker, wantWorker) {
		t.Errorf("worker.js mismatch:\n got %q\nwant %q", gotWorker, wantWorker)
	}

	// List includes manifest + payload
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

	v, err := pkg.ManifestJSON()
	if err != nil {
		t.Fatalf("ManifestJSON: %v", err)
	}
	m := v.(map[string]any)
	if m["id"] != "cat-ssr" {
		t.Errorf("id = %v", m["id"])
	}
	// Soft access to nested fields without schema package
	if port, ok := m["port"].(float64); !ok || port != 8080 {
		t.Errorf("port = %v", m["port"])
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

	list := pkg.List()
	if len(list) != 4 { // orvalho.json + 3 files
		t.Fatalf("List len = %d (%v), want 4", len(list), list)
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

	got, err := pkg.Get("src/lib/util.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("nested ok")) {
		t.Errorf("util.js unexpected: %q", got)
	}

	// Get manifest via Get
	raw, err := pkg.Get(ovpkg.ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, pkg.Manifest) {
		t.Error("Get(orvalho.json) != Manifest")
	}
}

func TestWriteFromFileMapAndReadBack(t *testing.T) {
	manifest := []byte(`{"id":"map-demo","entry":"main.js","runtime":"js"}`)
	files := map[string][]byte{
		"main.js":       []byte("export default {}"),
		"static/hi.txt": []byte("hi"),
	}

	var buf bytes.Buffer
	if err := ovpkg.Write(&buf, manifest, files); err != nil {
		t.Fatalf("Write: %v", err)
	}

	pkg, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if !bytes.Equal(pkg.Manifest, manifest) {
		t.Errorf("manifest mismatch")
	}
	got, err := pkg.Get("static/hi.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestPackageFromMapAndWritePackage(t *testing.T) {
	p, err := ovpkg.PackageFromMap(
		[]byte(`{"id":"x","entry":"e.js","runtime":"js"}`),
		map[string][]byte{"e.js": []byte("// entry")},
	)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := ovpkg.WritePackage(&buf, p); err != nil {
		t.Fatal(err)
	}
	back, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, err := back.Get("e.js")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "// entry" {
		t.Errorf("got %q", data)
	}
}

func TestWriteFileOpenFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "pkg.zip")

	manifest, files, err := ovpkg.ReadDir(filepath.Join("testdata", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ovpkg.WriteFile(zipPath, manifest, files); err != nil {
		t.Fatal(err)
	}

	pkg, err := ovpkg.OpenFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID    string `json:"id"`
		Entry string `json:"entry"`
	}
	if err := pkg.UnmarshalManifest(&m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "hello" || m.Entry != "worker.js" {
		t.Errorf("manifest = %+v", m)
	}
}

func TestPackageFromDir(t *testing.T) {
	pkg, err := ovpkg.PackageFromDir(filepath.Join("testdata", "with-assets"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pkg.Files["assets/logo.txt"]; !ok {
		t.Fatalf("missing assets/logo.txt in Files: %v", pkg.List())
	}
	// Manifest not in Files
	if _, ok := pkg.Files[ovpkg.ManifestName]; ok {
		t.Error("Files must not contain orvalho.json")
	}
}

func TestMissingManifest_Dir(t *testing.T) {
	_, _, err := ovpkg.ReadDir(filepath.Join("testdata", "no-manifest"))
	if !errors.Is(err, ovpkg.ErrMissingManifest) {
		t.Fatalf("err = %v, want ErrMissingManifest", err)
	}
}

func TestBadManifestJSON_Dir(t *testing.T) {
	_, _, err := ovpkg.ReadDir(filepath.Join("testdata", "bad-manifest"))
	if err == nil {
		t.Fatal("expected error for invalid JSON manifest")
	}
}

func TestWriteRejectsEmptyManifest(t *testing.T) {
	var buf bytes.Buffer
	err := ovpkg.Write(&buf, nil, map[string][]byte{"a.js": []byte("x")})
	if !errors.Is(err, ovpkg.ErrMissingManifest) {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteRejectsInvalidJSONManifest(t *testing.T) {
	var buf bytes.Buffer
	err := ovpkg.Write(&buf, []byte("not-json"), map[string][]byte{"a.js": []byte("x")})
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestWriteRejectsManifestInFiles(t *testing.T) {
	var buf bytes.Buffer
	err := ovpkg.Write(&buf, []byte(`{}`), map[string][]byte{
		ovpkg.ManifestName: []byte(`{}`),
	})
	if !errors.Is(err, ovpkg.ErrDuplicatePath) {
		t.Fatalf("err = %v, want ErrDuplicatePath", err)
	}
}

func TestOpenRejectsZipWithoutManifest(t *testing.T) {
	// Build a raw zip without orvalho.json using archive via Write then strip —
	// easier: use packageFromMap-like empty and craft with store.
	// Write requires manifest, so craft zip manually in test helper.
	zipBytes := mustZip(t, map[string][]byte{"only.js": []byte("1")})
	_, err := ovpkg.OpenBytes(zipBytes)
	if !errors.Is(err, ovpkg.ErrMissingManifest) {
		t.Fatalf("err = %v, want ErrMissingManifest", err)
	}
}

func TestOpenRejectsZipSlipPaths(t *testing.T) {
	zipBytes := mustZip(t, map[string][]byte{
		ovpkg.ManifestName: []byte(`{"id":"x"}`),
		"../evil.js":       []byte("nope"),
	})
	_, err := ovpkg.OpenBytes(zipBytes)
	if !errors.Is(err, ovpkg.ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
}

func TestGetNotFound(t *testing.T) {
	pkg, err := ovpkg.PackageFromDir(filepath.Join("testdata", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pkg.Get("missing.js")
	if !errors.Is(err, ovpkg.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetIsolatesReturnedBytes(t *testing.T) {
	pkg, err := ovpkg.PackageFromDir(filepath.Join("testdata", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := pkg.Get("worker.js")
	if err != nil {
		t.Fatal(err)
	}
	a[0] = 'X'
	b, err := pkg.Get("worker.js")
	if err != nil {
		t.Fatal(err)
	}
	if b[0] == 'X' {
		t.Error("Get should return a copy")
	}
}

func TestWritePathNormalization(t *testing.T) {
	var buf bytes.Buffer
	err := ovpkg.Write(&buf, []byte(`{"id":"n"}`), map[string][]byte{
		`sub\file.js`: []byte("ok"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := ovpkg.OpenBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.Get("sub/file.js"); err != nil {
		t.Fatalf("normalized path missing: %v list=%v", err, pkg.List())
	}
}

func TestSoftManifestObjectFields(t *testing.T) {
	// Documents soft JSON reading without full schema validation (#26).
	pkg, err := ovpkg.PackageFromDir(filepath.Join("testdata", "with-assets"))
	if err != nil {
		t.Fatal(err)
	}
	var raw json.RawMessage
	if err := pkg.UnmarshalManifest(&raw); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("raw not valid")
	}
	// Unknown extra fields would also be accepted; soft parse only.
	type soft struct {
		ID     string   `json:"id"`
		Entry  string   `json:"entry"`
		Egress []string `json:"egress"`
	}
	var s soft
	if err := pkg.UnmarshalManifest(&s); err != nil {
		t.Fatal(err)
	}
	if s.ID != "cat-ssr" || len(s.Egress) != 1 {
		t.Errorf("soft = %+v", s)
	}
}

func TestWriteDirToOpenFileFixtureZipOnDisk(t *testing.T) {
	// End-to-end: fixture dir → zip file → open → compare to source files.
	fixtures := []string{"minimal", "with-assets", "nested"}
	outDir := t.TempDir()

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join("testdata", name)
			zipPath := filepath.Join(outDir, name+".zip")

			f, err := os.Create(zipPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := ovpkg.WriteDir(f, src); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			pkg, err := ovpkg.OpenFile(zipPath)
			if err != nil {
				t.Fatal(err)
			}

			// Every regular file in the fixture must be readable back.
			srcPkg, err := ovpkg.PackageFromDir(src)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(pkg.Manifest, srcPkg.Manifest) {
				t.Error("manifest bytes differ after zip round-trip")
			}
			if len(pkg.Files) != len(srcPkg.Files) {
				t.Fatalf("file count %d != %d", len(pkg.Files), len(srcPkg.Files))
			}
			for path, want := range srcPkg.Files {
				got, ok := pkg.Files[path]
				if !ok {
					t.Errorf("missing %s", path)
					continue
				}
				if !bytes.Equal(got, want) {
					t.Errorf("content mismatch for %s", path)
				}
			}
		})
	}
}

// mustZip builds a zip without going through ovpkg.Write (for negative tests).
func mustZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	b, err := rawZip(files)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
