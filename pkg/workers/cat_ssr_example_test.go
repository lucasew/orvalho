package workers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orvalho/pkg/ovpkg"
	"orvalho/pkg/workers"
)

func loadCatSSR(t *testing.T) (*ovpkg.Package, string) {
	t.Helper()
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
	return pkg, string(src)
}

func TestCatSSRExampleHandlerFallback(t *testing.T) {
	_, src := loadCatSSR(t)
	// Inject fetch with empty allowlist → all hosts denied → fallback page.
	iso := workers.New(workers.PrepareGuestScript(src), workers.Options{
		Fetch: workers.HTTPFetch(nil, nil, 0),
	})
	srv := httptest.NewServer(workers.Handler(iso))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Cat fact") {
		t.Fatalf("missing title: %s", truncate(s, 200))
	}
	if !strings.Contains(s, "fallback") {
		t.Fatalf("expected fallback badge: %s", truncate(s, 400))
	}
}

func TestCatSSRExampleLiveViaAllowlist(t *testing.T) {
	pkg, src := loadCatSSR(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"fact":   "A group of cats is a clowder.",
			"length": 28,
		})
	}))
	defer upstream.Close()

	src = strings.Replace(src, "https://catfact.ninja/fact", upstream.URL+"/fact", 1)

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	egress, err := pkg.Egress()
	if err != nil {
		t.Fatal(err)
	}
	allow := append(workers.EgressList(egress), u.Host)

	iso := workers.New(workers.PrepareGuestScript(src), workers.Options{
		Fetch: workers.HTTPFetch(allow, upstream.Client(), 0),
	})
	srv := httptest.NewServer(workers.Handler(iso))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "clowder") {
		t.Fatalf("expected live fact in HTML: %s", truncate(s, 500))
	}
	if !strings.Contains(s, "live") {
		t.Fatalf("expected live badge: %s", truncate(s, 500))
	}
	if strings.Contains(s, "class=\"badge fallback\"") {
		t.Fatalf("should not be fallback: %s", truncate(s, 500))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
