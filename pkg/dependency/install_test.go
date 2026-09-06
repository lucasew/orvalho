package dependency

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func npmTarball(t *testing.T, files map[string]string) (data []byte, sri string) {
	t.Helper()
	var buf bytes.Buffer
	sum := SHA512()
	w := io.MultiWriter(&buf, sum)
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name: "package/" + name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), sum.String()
}

func TestDefaultStoreDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	got, err := DefaultStoreDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "orvalho")
	if got != want {
		t.Fatalf("DefaultStoreDir() = %q; want %q", got, want)
	}
}

func TestStoreUsesUserCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := (Options{}).store()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "orvalho")
	if st.Dir != want {
		t.Fatalf("store().Dir = %q; want %q", st.Dir, want)
	}
}

func TestStoreDirFlagWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	flag := filepath.Join(dir, "explicit")
	st, err := (Options{StoreDir: flag}).store()
	if err != nil {
		t.Fatal(err)
	}
	if st.Dir != flag {
		t.Fatalf("store().Dir = %q; want %q", st.Dir, flag)
	}
}

func TestInstallIsolatedTree(t *testing.T) {
	t.Parallel()
	leftpadJS := "module.exports = function (s) { return s; }\n"
	leftpadJSON := `{"name":"leftpad","version":"1.3.0","main":"index.js"}`
	tg, integrity := npmTarball(t, map[string]string{
		"package.json": leftpadJSON,
		"index.js":     leftpadJS,
	})

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/leftpad/-/leftpad-1.3.0.tgz" {
			if _, err := w.Write(tg); err != nil {
				t.Errorf("write tarball: %v", err)
			}
			return
		}
		if r.URL.Path == "/leftpad" {
			if err := json.NewEncoder(w).Encode(map[string]any{
				"name":      "leftpad",
				"dist-tags": map[string]string{"latest": "1.3.0"},
				"versions": map[string]any{
					"1.3.0": map[string]any{
						"name":    "leftpad",
						"version": "1.3.0",
						"dist": map[string]string{
							"tarball":   srv.URL + "/leftpad/-/leftpad-1.3.0.tgz",
							"integrity": integrity,
						},
					},
				},
			}); err != nil {
				t.Errorf("encode packument: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "app",
  "version": "1.0.0",
  "dependencies": { "leftpad": "^1.3.0" }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := (Options{
		Dir:      dir,
		StoreDir: filepath.Join(dir, ".orvalho", "store"),
		Registry: srv.URL,
		HTTP:     srv.Client(),
	}).Install(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, "node_modules", "leftpad"))
	if err != nil {
		t.Fatal(err)
	}
	if target == "" {
		t.Fatal("empty symlink")
	}
	body, err := os.ReadFile(filepath.Join(dir, "node_modules", "leftpad", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != leftpadJS {
		t.Fatalf("got %q", body)
	}
	// Top level must not contain a hoisted mystery package.
	ents, err := os.ReadDir(filepath.Join(dir, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	for _, n := range names {
		if n != "leftpad" && n != ".orvalho" && n != ".bin" {
			t.Fatalf("unexpected top-level %q in %v", n, names)
		}
	}
}

func TestDetectForeignLockfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte("# yarn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DetectLockfile(dir)
	if err == nil {
		t.Fatal("expected unrecognized lockfile")
	}
}

func TestKeepOptional(t *testing.T) {
	t.Parallel()
	if keepOptional(nil) {
		t.Fatal("empty cpu must skip")
	}
	if keepOptional([]string{"x64"}) {
		t.Fatal("native cpu must skip")
	}
	if !keepOptional([]string{"wasm32"}) {
		t.Fatal("wasm32 must keep")
	}
}

func TestSatisfiesCaret(t *testing.T) {
	t.Parallel()
	if !Satisfies("1.3.0", "^1.0.0") {
		t.Fatal("1.3.0 should satisfy ^1.0.0")
	}
	if Satisfies("2.0.0", "^1.0.0") {
		t.Fatal("2.0.0 must not satisfy ^1.0.0")
	}
}

func TestSatisfiesHyphen(t *testing.T) {
	t.Parallel()
	if !Satisfies("1.5.0", "1.0.0 - 2.0.0") {
		t.Fatal("1.5.0 should satisfy 1.0.0 - 2.0.0")
	}
	if Satisfies("2.0.1", "1.0.0 - 2.0.0") {
		t.Fatal("2.0.1 must not satisfy 1.0.0 - 2.0.0")
	}
}

func TestReadManifestBadField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(`{"name":"app","dependencies":"nope"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readManifest(path)
	if !errors.Is(err, ErrManifest) {
		t.Fatalf("got %v", err)
	}
}

func TestParseVersionWraps(t *testing.T) {
	t.Parallel()
	_, err := parseVersion("nope")
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("got %v", err)
	}
	_, err = parseVersion("1.2.x")
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("got %v", err)
	}
}

func TestUnpackRejectsEscape(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("x")
	if err := tw.WriteHeader(&tar.Header{Name: "package/../../etc/passwd", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	err := unpackTarball(bytes.NewReader(buf.Bytes()), t.TempDir())
	if !errors.Is(err, ErrTarPath) {
		t.Fatalf("got %v", err)
	}
}
