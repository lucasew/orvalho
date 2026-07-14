package worker

import "testing"

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must be non-empty")
	}
}

func TestRole(t *testing.T) {
	if Role != "worker" {
		t.Fatalf("Role = %q, want worker", Role)
	}
}
