package hostobject_test

import (
	"errors"
	"testing"

	"github.com/dop251/goja"

	"github.com/lucasew/orvalho/pkg/hostobject"
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

func TestMethodsHideFields(t *testing.T) {
	rt := goja.New()
	c := &counterHost{Secret: "nope"}
	obj, err := hostobject.New().Methods(c).Build(rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("counter", obj); err != nil {
		t.Fatal(err)
	}
	v, err := rt.RunString(`
		(function() {
			if (counter.secret !== undefined) return "leaked";
			counter.inc();
			return String(counter.get());
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1" {
		t.Fatalf("got %q", v.String())
	}
	if c.n != 1 {
		t.Fatalf("host n=%d", c.n)
	}
}

func TestSetProperty(t *testing.T) {
	rt := goja.New()
	obj, err := hostobject.New().Set("ping", func() string { return "pong" }).Build(rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("box", obj); err != nil {
		t.Fatal(err)
	}
	v, err := rt.RunString(`box.ping()`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "pong" {
		t.Fatalf("got %q", v.String())
	}
}

func TestNameClash(t *testing.T) {
	_, err := hostobject.New().Set("inc", 1).Methods(&counterHost{}).Build(goja.New())
	if err == nil || !errors.Is(err, hostobject.ErrNameClash) {
		t.Fatalf("want clash, got %v", err)
	}
}

func TestMethodsNeedPointer(t *testing.T) {
	_, err := hostobject.New().Methods(counterHost{}).Build(goja.New())
	if err == nil || !errors.Is(err, hostobject.ErrMethodsNeedPointer) {
		t.Fatalf("want pointer error, got %v", err)
	}
}

func TestBuildNilRuntime(t *testing.T) {
	_, err := hostobject.New().Set("x", 1).Build(nil)
	if err == nil || !errors.Is(err, hostobject.ErrNoRuntime) {
		t.Fatalf("want no runtime, got %v", err)
	}
}

func TestTwoRecipesAreIndependent(t *testing.T) {
	a := hostobject.New().Set("n", 1)
	b := hostobject.New().Set("n", 2)
	rt := goja.New()
	oa, err := a.Build(rt)
	if err != nil {
		t.Fatal(err)
	}
	ob, err := b.Build(rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("a", oa); err != nil {
		t.Fatal(err)
	}
	if err := rt.Set("b", ob); err != nil {
		t.Fatal(err)
	}
	v, err := rt.RunString(`a.n + "," + b.n`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1,2" {
		t.Fatalf("got %q", v.String())
	}
}
