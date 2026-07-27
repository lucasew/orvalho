package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateUnique(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if a.PublicID() == b.PublicID() {
		t.Fatal("two random managers share PublicID")
	}
	if a.Equal(b) {
		t.Fatal("two random managers report Equal")
	}
}

func TestFromSeedDeterministic(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	a, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}
	b, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}
	if a.PublicID() != b.PublicID() {
		t.Fatalf("PublicID mismatch: %q vs %q", a.PublicID(), b.PublicID())
	}
	if !a.Equal(b) {
		t.Fatal("same seed should yield Equal managers")
	}
	if !bytes.Equal(a.PrivateKey().Seed(), b.PrivateKey().Seed()) {
		t.Fatal("same seed should yield equal private seeds")
	}
}

func TestFromSeedRejectsBadLength(t *testing.T) {
	if _, err := FromSeed([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for short seed")
	}
}

func TestPublicIDIsRawPublicKey(t *testing.T) {
	seed := bytes.Repeat([]byte{0x07}, ed25519.SeedSize)
	m, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(m.PublicID())
	if err != nil {
		t.Fatalf("PublicID is not raw base64url: %v", err)
	}
	if !bytes.Equal(raw, m.PublicKey()) {
		t.Fatalf("PublicID decode mismatch\n got %x\nwant %x", raw, m.PublicKey())
	}
}

func TestPEMRoundTripDeterministic(t *testing.T) {
	seed := bytes.Repeat([]byte{0x11}, ed25519.SeedSize)
	m, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}

	pem1, err := m.MarshalPrivatePEM()
	if err != nil {
		t.Fatalf("MarshalPrivatePEM: %v", err)
	}
	pem2, err := m.MarshalPrivatePEM()
	if err != nil {
		t.Fatalf("MarshalPrivatePEM second: %v", err)
	}
	// PKCS#8 PEM for a fixed key is stable; assert that so we do not regress
	// into OpenSSH-style randomized private key encoding.
	if !bytes.Equal(pem1, pem2) {
		t.Fatal("PKCS#8 PEM encoding is not deterministic for the same key")
	}

	loaded, err := ParsePrivatePEM(pem1)
	if err != nil {
		t.Fatalf("ParsePrivatePEM: %v", err)
	}
	if loaded.PublicID() != m.PublicID() {
		t.Fatalf("PublicID after PEM round-trip: got %q want %q", loaded.PublicID(), m.PublicID())
	}
	if !bytes.Equal(loaded.PrivateKey().Seed(), m.PrivateKey().Seed()) {
		t.Fatal("private seed changed across PEM round-trip")
	}
}

func TestParsePrivatePEMErrors(t *testing.T) {
	if _, err := ParsePrivatePEM([]byte("not pem")); err == nil {
		t.Fatal("expected error for non-PEM")
	}

	// Wrong PEM type
	wrongType := []byte("-----BEGIN PUBLIC KEY-----\nAQAB\n-----END PUBLIC KEY-----\n")
	if _, err := ParsePrivatePEM(wrongType); err == nil {
		t.Fatal("expected error for wrong PEM type")
	}

	// Multiple blocks
	seed := bytes.Repeat([]byte{0x22}, ed25519.SeedSize)
	m, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed: %v", err)
	}
	one, err := m.MarshalPrivatePEM()
	if err != nil {
		t.Fatalf("MarshalPrivatePEM: %v", err)
	}
	two := append(append([]byte{}, one...), one...)
	if _, err := ParsePrivatePEM(two); err == nil {
		t.Fatal("expected error for multiple PEM blocks")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "manager.key")

	m, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := m.Save(path, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode = %o, want 0600", perm)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PublicID() != m.PublicID() {
		t.Fatalf("PublicID after load: got %q want %q", loaded.PublicID(), m.PublicID())
	}
	if !bytes.Equal(loaded.PrivateKey().Seed(), m.PrivateKey().Seed()) {
		t.Fatal("private seed changed across save/load")
	}
}

func TestSaveRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manager.key")

	m1, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := m1.Save(path, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m2, err := Generate()
	if err != nil {
		t.Fatalf("Generate second: %v", err)
	}
	if err := m2.Save(path, false); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	// With overwrite, public id should become m2's.
	if err := m2.Save(path, true); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PublicID() != m2.PublicID() {
		t.Fatalf("after overwrite PublicID = %q, want %q", loaded.PublicID(), m2.PublicID())
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.key"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

func TestSignVerifyWithLoadedKey(t *testing.T) {
	// Prove key material is usable without depending on PEM string equality.
	dir := t.TempDir()
	path := filepath.Join(dir, "manager.key")

	m, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := m.Save(path, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	msg := []byte("orvalho-manager-identity")
	sig := ed25519.Sign(loaded.PrivateKey(), msg)
	if !ed25519.Verify(m.PublicKey(), msg, sig) {
		t.Fatal("signature from loaded key not verified by original public key")
	}
}
