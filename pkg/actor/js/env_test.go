package js_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	actorjs "orvalho/pkg/actor/js"
)

func TestEnvStringAndAssetsFetch(t *testing.T) {
	src := actorjs.PrepareGuestScript(`
export default {
  async fetch(request, env, ctx) {
    // Avoid URL global (not in goja WinterTC subset yet).
    if (String(request.url).indexOf("/title") !== -1) {
      return new Response(env.SITE_TITLE || "", { status: 200 });
    }
    return env.ASSETS.fetch(request);
  }
};
`)
	files := map[string][]byte{
		"assets/hello.txt": []byte("hello-asset"),
	}
	iso := actorjs.New(src, actorjs.Options{
		Env: map[string]string{"SITE_TITLE": "Cats"},
		Bindings: map[string]actorjs.Binding{
			"ASSETS": &actorjs.AssetBinding{
				Root: "assets",
				Read: func(path string) ([]byte, bool) {
					b, ok := files[path]
					return b, ok
				},
			},
		},
	})

	// String env
	res, err := iso.Fetch(context.Background(), actorjs.HTTPRequest{
		Method: "GET",
		URL:    "http://127.0.0.1/title",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "Cats" {
		t.Fatalf("title body=%q", res.Body)
	}

	// Assets
	srv := httptest.NewServer(actorjs.Handler(iso))
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

	// Missing asset
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
	src := actorjs.PrepareGuestScript(`export default { async fetch(r, env) { return new Response("ok"); } };`)
	iso := actorjs.New(src, actorjs.Options{
		Env: map[string]string{"ASSETS": "x"},
		Bindings: map[string]actorjs.Binding{
			"ASSETS": &actorjs.AssetBinding{
				Root: "assets",
				Read: func(string) ([]byte, bool) { return nil, false },
			},
		},
	})
	_, err := iso.Fetch(context.Background(), actorjs.HTTPRequest{URL: "http://x/"})
	if err == nil || !strings.Contains(err.Error(), "clash") {
		t.Fatalf("want clash error, got %v", err)
	}
}
