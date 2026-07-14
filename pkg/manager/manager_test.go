package manager

import "testing"

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must be non-empty")
	}
}

func TestRole(t *testing.T) {
	if Role != "manager" {
		t.Fatalf("Role = %q, want manager", Role)
	}
}
