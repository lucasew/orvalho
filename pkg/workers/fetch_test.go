package workers

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPrepareGuestScriptExportDefault(t *testing.T) {
	src := PrepareGuestScript(`export default { async fetch() { return 1; } };`)
	if strings.Contains(src, "export default") {
		t.Fatalf("export default not rewritten: %s", src)
	}
	if !strings.Contains(src, "globalThis.default =") {
		t.Fatalf("missing globalThis.default: %s", src)
	}
}

func TestFetchDefaultHandler(t *testing.T) {
	script := PrepareGuestScript(`
export default {
  async fetch(request, env, ctx) {
    return new Response("hello " + request.method + " " + request.url, {
      status: 200,
      headers: { "X-Test": "1" }
    });
  }
};
`)
	iso := New(script, Options{})
	got, err := iso.Fetch(t.Context(), HTTPRequest{
		Method: "GET",
		URL:    "http://actor.test/hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 200 {
		t.Fatalf("status=%d", got.Status)
	}
	if got.Body != "hello GET http://actor.test/hi" {
		t.Fatalf("body=%q", got.Body)
	}
	if got.Headers["x-test"] != "1" {
		t.Fatalf("headers=%v", got.Headers)
	}
}

func TestFetchMissingDefault(t *testing.T) {
	iso := New(`var x = 1;`, Options{})
	_, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://x/"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrMissingDefaultExport) {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchErrorSurfaces(t *testing.T) {
	script := PrepareGuestScript(`
export default {
  fetch() { throw new Error("boom"); }
};
`)
	iso := New(script, Options{})
	_, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://x/"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrFetchRejected) {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchAwaitsTimerPromise(t *testing.T) {
	script := PrepareGuestScript(`
export default {
  async fetch() {
    await new Promise(function(resolve) {
      setTimeout(resolve, 20);
    });
    return new Response("later", { status: 200 });
  }
};
`)
	iso := New(script, Options{})
	base := time.Unix(1_700_000_000, 0)
	now := base
	iso.now = func() time.Time { return now }

	// Drive Fetch in a goroutine while advancing fake time from the outside
	// is hard because Fetch holds the lock. Instead advance now before each
	// internal tick by making now jump forward gradually via side channel:
	// each call to now() advances 10ms so pending setTimeout becomes due.
	calls := 0
	iso.now = func() time.Time {
		calls++
		return base.Add(time.Duration(calls*10) * time.Millisecond)
	}

	got, err := iso.Fetch(t.Context(), HTTPRequest{URL: "http://x/"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "later" {
		t.Fatalf("body=%q", got.Body)
	}
}
