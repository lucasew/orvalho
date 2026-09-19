package main

import (
	"testing"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
)

func TestServeAddrOmitted(t *testing.T) {
	got, err := lewcmd.Parse[serveCmd]()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Addr.Value() != "" {
		t.Fatalf("addr = %q, want empty (package port or :8787 at Run)", got.Addr.Value())
	}
}

func TestDepInstallWorkDirDefault(t *testing.T) {
	got, err := lewcmd.Parse[depInstallCmd]()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Dir.Value() != "." {
		t.Fatalf("dir = %q, want .", got.Dir.Value())
	}
}
