package js_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	actorjs "orvalho/pkg/actor/js"
	"orvalho/pkg/ovpkg"
)

func TestCatSSRExampleHandlerFallback(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "cat-ssr")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("example not present: %v", err)
	}
	pkg, err := ovpkg.OpenPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := pkg.Entry()
	if err != nil {
		t.Fatal(err)
	}
	src, err := pkg.Get(entry)
	if err != nil {
		t.Fatal(err)
	}
	iso := actorjs.New(actorjs.PrepareGuestScript(string(src)), actorjs.Options{})
	srv := httptest.NewServer(actorjs.Handler(iso))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type=%q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Cat fact") {
		t.Fatalf("missing title in body: %s", truncate(s, 200))
	}
	if !strings.Contains(s, "fallback") {
		t.Fatalf("expected offline fallback badge without host fetch: %s", truncate(s, 400))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
