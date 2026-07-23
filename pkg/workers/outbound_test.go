package workers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOutboundFetchAllowlisted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fact" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"fact":"cats are liquid","length":15}`)
	}))
	defer upstream.Close()

	host := mustHost(t, upstream.URL)
	script := PrepareGuestScript(`
export default {
  async fetch() {
    var res = await fetch("` + upstream.URL + `/fact");
    var body = await res.text();
    return new Response(body, {
      status: res.status,
      headers: { "Content-Type": "application/json" }
    });
  }
};
`)
	iso := New(script, Options{
		Fetch: HTTPFetch(EgressList{host}, upstream.Client(), 0),
	})
	got, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://actor/"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 200 {
		t.Fatalf("status=%d", got.Status)
	}
	if !strings.Contains(got.Body, "cats are liquid") {
		t.Fatalf("body=%q", got.Body)
	}
}

func TestOutboundFetchDenied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secret")
	}))
	defer upstream.Close()

	script := PrepareGuestScript(`
export default {
  async fetch() {
    try {
      await fetch("` + upstream.URL + `/");
      return new Response("allowed", { status: 200 });
    } catch (e) {
      return new Response(String(e), { status: 403 });
    }
  }
};
`)
	iso := New(script, Options{
		Fetch: HTTPFetch(EgressList{"only-this.example"}, upstream.Client(), 0),
	})
	got, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://actor/"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 403 {
		t.Fatalf("status=%d body=%q", got.Status, got.Body)
	}
	if !strings.Contains(strings.ToLower(got.Body), "egress") {
		t.Fatalf("expected egress error in body, got %q", got.Body)
	}
}

func TestOutboundFetchEmptyAllowlistDenies(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "nope")
	}))
	defer upstream.Close()

	script := PrepareGuestScript(`
export default {
  async fetch() {
    try {
      await fetch("` + upstream.URL + `/");
      return new Response("ok", { status: 200 });
    } catch (e) {
      return new Response("denied", { status: 403 });
    }
  }
};
`)
	iso := New(script, Options{
		Fetch: HTTPFetch(nil, upstream.Client(), 0),
	})
	got, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://actor/"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 403 || got.Body != "denied" {
		t.Fatalf("got status=%d body=%q", got.Status, got.Body)
	}
}

func TestFetchGlobalAbsentWithoutDI(t *testing.T) {
	iso := New(``, Options{})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	v, err := iso.vm.RunString(`typeof fetch`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "undefined" {
		t.Fatalf("typeof fetch=%q want undefined", got)
	}
}

func TestFetchGlobalIsFunctionWhenInjected(t *testing.T) {
	iso := New(``, Options{Fetch: HTTPFetch(EgressList{"*"}, nil, 0)})
	if _, err := iso.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	v, err := iso.vm.RunString(`typeof fetch`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "function" {
		t.Fatalf("typeof fetch=%q", got)
	}
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}
