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
		"fs",
		"events",
		"lodash",
		"@scope/pkg",
		"@scope/pkg/sub",
		"./secret",
		"../etc/passwd",
	}
	for _, spec := range ok {
		if !Valid(spec) {
			t.Fatalf("%q should be valid", spec)
		}
	}
	bad := []string{
		"",
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

func TestChainTriedInOrder(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:x",
		Map[string]{"orvalho:x": "first"},
		Map[string]{"orvalho:x": "second"},
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
		Map[string]{"orvalho:a": "A"},
		Map[string]{"orvalho:b": "B"},
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
	h := Scheme[string]{Name: "orvalho", Load: func(spec string) (string, error) {
		seen = append(seen, spec)
		if spec == "orvalho:rebimboca-da-parafuseta" {
			return "ok", nil
		}
		return "", ErrNotFound
	}}
	got, err := Resolve("orvalho:rebimboca-da-parafuseta", h)
	if err != nil || got != "ok" {
		t.Fatalf("got %q %v", got, err)
	}
	_, err = Resolve("node:fs", h)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for node:fs, got %v", err)
	}
	if len(seen) != 1 || seen[0] != "orvalho:rebimboca-da-parafuseta" {
		t.Fatalf("load called with %v", seen)
	}
}

func TestAliasThenMap(t *testing.T) {
	t.Parallel()
	got, err := Resolve("orvalho:buf",
		Alias[string]{From: "orvalho:buf", To: "orvalho:buffer"},
		Map[string]{"orvalho:buffer": "buf"},
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
		Scheme[string]{Name: "orvalho", Load: func(string) (string, error) { return "", ErrNotFound }},
		Map[string]{"orvalho:x": "fallback"},
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
