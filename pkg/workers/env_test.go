package workers_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lucasew/orvalho/pkg/workers"
)

func TestEnvStringAndAssetsFetch(t *testing.T) {
	src := workers.PrepareGuestScript(`
export default {
  async fetch(request, env, ctx) {
    if (String(request.url).indexOf("/title") !== -1) {
      return new Response(env.SITE_TITLE || "", { status: 200 });
    }
    return env.ASSETS.fetch(request);
  }
};
`)
	fsys := fstest.MapFS{
		"assets/hello.txt": {Data: []byte("hello-asset")},
	}
	iso := workers.New(src, workers.Options{
		Env: map[string]string{"SITE_TITLE": "Cats"},
		Bindings: map[string]workers.Binding{
			"ASSETS": workers.NewAssetBinding(fsys, "assets"),
		},
	})

	res, err := iso.Fetch(context.Background(), workers.HTTPRequest{
		Method: "GET",
		URL:    "http://127.0.0.1/title",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "Cats" {
		t.Fatalf("title body=%q", res.Body)
	}

	srv := httptest.NewServer(workers.Handler(iso))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if string(body) != "hello-asset" {
		t.Fatalf("body=%q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type=%q", ct)
	}

	resp2, err := http.Get(srv.URL + "/nope.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("want 404 got %d", resp2.StatusCode)
	}
}

func TestEnvNameClash(t *testing.T) {
	src := workers.PrepareGuestScript(`export default { async fetch(r, env) { return new Response("ok"); } };`)
	iso := workers.New(src, workers.Options{
		Env: map[string]string{"ASSETS": "x"},
		Bindings: map[string]workers.Binding{
			"ASSETS": workers.NewAssetBinding(fstest.MapFS{}, "assets"),
		},
	})
	_, err := iso.Fetch(context.Background(), workers.HTTPRequest{URL: "http://x/"})
	if err == nil || !strings.Contains(err.Error(), "clash") {
		t.Fatalf("want clash error, got %v", err)
	}
}

func TestURLSearchParamsGet(t *testing.T) {
	src := workers.PrepareGuestScript(`
export default {
  async fetch(request, env, ctx) {
    var u = new URL(request.url);
    var q = u.searchParams.get("query");
    return new Response(q === null ? "null" : q, { status: 200 });
  }
};
`)
	iso := workers.New(src, workers.Options{})
	res, err := iso.Fetch(context.Background(), workers.HTTPRequest{
		Method: "GET",
		URL:    "http://127.0.0.1:8788/search?query=teste",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "teste" {
		t.Fatalf("searchParams.get(query)=%q want teste", res.Body)
	}
}
