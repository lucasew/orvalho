package main

import (
	"errors"
	"os"
	"testing"

	lewcmd "github.com/lewtec/lewkit/x/cmd"
)

func TestServeAddrOmitted(t *testing.T) {
	got, err := lewcmd.Parse[serveCmd]()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Addr.Value() != ":8787" {
		t.Fatalf("addr = %q, want :8787", got.Addr.Value())
	}
}

func TestServeAddrBarePort(t *testing.T) {
	got, err := lewcmd.Parse[serveCmd]("--addr", "8787")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Addr.Value() != ":8787" {
		t.Fatalf("addr = %q, want :8787", got.Addr.Value())
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

func TestServeDoesNotRequireDataDir(t *testing.T) {
	got, err := lewcmd.Parse[lewcmd.App[orvalhoCLI]]("serve", ".")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Args.Identity != nil || got.Args.ConfigCmd != nil {
		t.Fatal("serve selected host command pointers")
	}
	if hostDataDir(got.Args) != "" {
		t.Fatalf("data-dir = %q, want empty", hostDataDir(got.Args))
	}
}

func TestIdentityRequiresExistingDataDir(t *testing.T) {
	dir := t.TempDir()
	got, err := lewcmd.Parse[lewcmd.App[orvalhoCLI]]("identity", "generate", "--data-dir", dir)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if hostDataDir(got.Args) != dir {
		t.Fatalf("data-dir = %q, want %q", hostDataDir(got.Args), dir)
	}
	_, err = lewcmd.Parse[lewcmd.App[orvalhoCLI]]("identity", "generate")
	if !errors.Is(err, lewcmd.ErrMissingValue) {
		t.Fatalf("missing --data-dir: %v", err)
	}
	_, err = lewcmd.Parse[lewcmd.App[orvalhoCLI]]("identity", "generate", "--data-dir", dir+"/nope")
	if err == nil {
		t.Fatal("expected error for missing data-dir")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing dir: %v", err)
	}
}
