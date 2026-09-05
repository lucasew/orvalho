package imports

import (
	"errors"
	"testing"
)

func TestValid(t *testing.T) {
	t.Parallel()
	ok := []string{
		"node:events",
		"node:fs/promises",
		"orvalho:rebimboca-da-parafuseta",
		"orvalho:host",
		"cloudflare:workers",
	}
	for _, spec := range ok {
		if !Valid(spec) {
			t.Fatalf("%q should be valid", spec)
		}
	}
	bad := []string{
		"",
		"fs",
		"events",
		"./secret",
		"../etc/passwd",
		"/etc/passwd",
		"node:",
		"Node:events",
		"orvalho:/abs",
		"orvalho:../x",
		"orvalho:foo/../bar",
		"orvalho:foo//bar",
	}
	for _, spec := range bad {
		if Valid(spec) {
			t.Fatalf("%q must not be valid", spec)
		}
	}
}

func TestResolveInvalidNeverReachesHandler(t *testing.T) {
	t.Parallel()
	called := false
	h := func(spec string, next Resolver[string]) (string, error) {
		called = true
		return next(spec)
	}
	_, err := Resolve("./secret", h)
	if !errors.Is(err, ErrSpecifier) {
		t.Fatalf("want ErrSpecifier, got %v", err)
	}
	if called {
		t.Fatal("handler must not run for an invalid specifier")
	}
}

func TestChainFirstHandlerWins(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:x",
		Map(map[string]string{"orvalho:x": "first"}),
		Map(map[string]string{"orvalho:x": "second"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "first" {
		t.Fatalf("got %q want first", got)
	}
}

func TestChainPassThrough(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:b",
		Map(map[string]string{"orvalho:a": "A"}),
		Map(map[string]string{"orvalho:b": "B"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "B" {
		t.Fatalf("got %q want B", got)
	}
}

func TestSchemeLoadsLazily(t *testing.T) {
	t.Parallel()
	var seen []string
	h := Scheme("orvalho", func(spec string) (string, error) {
		seen = append(seen, spec)
		if spec == "orvalho:rebimboca-da-parafuseta" {
			return "ok", nil
		}
		return "", ErrNotFound
	})
	got, err := Resolve("orvalho:rebimboca-da-parafuseta", h)
	if err != nil || got != "ok" {
		t.Fatalf("got %q %v", got, err)
	}
	_, err = Resolve("node:fs", h)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for node:fs, got %v", err)
	}
	if len(seen) != 1 || seen[0] != "orvalho:rebimboca-da-parafuseta" {
		t.Fatalf("load called with %v, want only the orvalho specifier", seen)
	}
}

func TestAliasThenMap(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:buf",
		Alias[string]("orvalho:buf", "orvalho:buffer"),
		Map(map[string]string{"orvalho:buffer": "buf"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "buf" {
		t.Fatalf("got %q want buf", got)
	}
}

func TestSchemeNotFoundContinuesChain(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:x",
		Scheme("orvalho", func(string) (string, error) { return "", ErrNotFound }),
		Map(map[string]string{"orvalho:x": "fallback"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback" {
		t.Fatalf("got %q want fallback", got)
	}
}

func TestResolveMiss(t *testing.T) {
	t.Parallel()
	_, err := Resolve[string]("orvalho:missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
