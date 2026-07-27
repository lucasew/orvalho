package workers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRoundTrip(t *testing.T) {
	script := PrepareGuestScript(`
export default {
  async fetch(request) {
    return new Response("echo:" + request.method + ":" + (await request.text()), {
      status: 201,
      headers: { "Content-Type": "text/plain" }
    });
  }
};
`)
	iso := New(script, Options{})
	srv := httptest.NewServer(Handler(iso))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/p", "text/plain", strings.NewReader("xyz"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "echo:POST:xyz" {
		t.Fatalf("body=%q", b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("ct=%q", ct)
	}
}
