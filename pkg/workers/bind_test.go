package workers_test

import (
	"errors"
	"testing"

	"github.com/lucasew/orvalho/pkg/hostobject"
	"github.com/lucasew/orvalho/pkg/workers"
)

type counterHost struct {
	n      int
	Secret string
}

func (c *counterHost) Inc() int {
	c.n++
	return c.n
}

func (c *counterHost) Get() int {
	return c.n
}

func TestBindMethods(t *testing.T) {
	src := workers.PrepareGuestScript(`
export default {
  async fetch(request, env) {
    if (env.COUNTER.secret !== undefined) {
      return new Response("leaked", { status: 500 });
    }
    env.COUNTER.inc();
    return new Response(String(env.COUNTER.get()), { status: 200 });
  }
};
`)
	c := &counterHost{Secret: "nope"}
	iso := workers.New(src, workers.Options{
		Bindings: map[string]workers.Binding{
			"COUNTER": workers.Bind(hostobject.New().Methods(c)),
		},
	})
	res, err := iso.Fetch(t.Context(), workers.HTTPRequest{URL: "http://x/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "1" {
		t.Fatalf("body=%q want 1", res.Body)
	}
	if c.n != 1 {
		t.Fatalf("host n=%d", c.n)
	}
}

func TestBindSet(t *testing.T) {
	src := workers.PrepareGuestScript(`
export default {
  async fetch(request, env) {
    return new Response(env.BOX.ping(), { status: 200 });
  }
};
`)
	iso := workers.New(src, workers.Options{
		Bindings: map[string]workers.Binding{
			"BOX": workers.Bind(hostobject.New().Set("ping", func() string { return "pong" })),
		},
	})
	res, err := iso.Fetch(t.Context(), workers.HTTPRequest{URL: "http://x/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != "pong" {
		t.Fatalf("body=%q", res.Body)
	}
}

func TestBindNameClash(t *testing.T) {
	src := workers.PrepareGuestScript(`export default { async fetch() { return new Response("ok"); } };`)
	iso := workers.New(src, workers.Options{
		Bindings: map[string]workers.Binding{
			"BOX": workers.Bind(hostobject.New().Set("inc", 1).Methods(&counterHost{})),
		},
	})
	_, err := iso.Fetch(t.Context(), workers.HTTPRequest{URL: "http://x/"})
	if err == nil || !errors.Is(err, hostobject.ErrNameClash) {
		t.Fatalf("want clash, got %v", err)
	}
}
