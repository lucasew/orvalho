package imports

import "testing"

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
