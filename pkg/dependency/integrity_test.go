package dependency

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestParseIntegrity(t *testing.T) {
	t.Parallel()
	sum := sha512.Sum512([]byte("hello"))
	sri := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	algo, hash, err := ParseIntegrity(sri)
	if err != nil {
		t.Fatal(err)
	}
	if algo != "sha512" {
		t.Fatalf("algo %q", algo)
	}
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %q", hash)
	}
}

func TestParseIntegrityEmpty(t *testing.T) {
	t.Parallel()
	if _, _, err := ParseIntegrity(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseIntegrityFirstWins(t *testing.T) {
	t.Parallel()
	sum := sha512.Sum512([]byte("hello"))
	first := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	algo, hash, err := ParseIntegrity(first + " sha256-deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if algo != "sha512" {
		t.Fatalf("algo %q", algo)
	}
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %q", hash)
	}
}
