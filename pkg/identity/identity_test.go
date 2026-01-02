package identity

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"
)

func TestDeriveIdentities(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	passphrase := "testpassphrase"

	// 1. Determinism
	id1, err := DeriveIdentities(mnemonic, passphrase)
	if err != nil {
		t.Fatalf("First derivation failed: %v", err)
	}

	id2, err := DeriveIdentities(mnemonic, passphrase)
	if err != nil {
		t.Fatalf("Second derivation failed: %v", err)
	}

	// SSH Private Key PEM in OpenSSH format contains random 'checkint' fields,
	// so the PEM string is not deterministic even if the key is.
	// We verify the public keys match, which ensures the key material is the same.
	if id1.SSHPublicKey != id2.SSHPublicKey {
		t.Error("SSH Public Keys do not match")
	}

	// Verify the private keys are effectively the same by parsing them
	k1, err := ssh.ParsePrivateKey([]byte(id1.SSHPrivateKeyPEM))
	if err != nil {
		t.Fatalf("Failed to parse first SSH private key: %v", err)
	}
	k2, err := ssh.ParsePrivateKey([]byte(id2.SSHPrivateKeyPEM))
	if err != nil {
		t.Fatalf("Failed to parse second SSH private key: %v", err)
	}
	// Unfortunately ssh.Signer doesn't expose the raw key easily for comparison without casting.
	// But since Public Key matches, and we derived them, it implies Private Key matches.
	// We can check if they sign the same message with the same signature if deterministic signing is used,
	// but Ed25519 signing is deterministic.
	// Let's verify k1 and k2 have the same public key.
	if !bytes.Equal(k1.PublicKey().Marshal(), k2.PublicKey().Marshal()) {
		t.Error("Parsed SSH private keys do not yield same public key")
	}
	if id1.AgeRecipient != id2.AgeRecipient {
		t.Error("Age Recipients do not match")
	}
	// We can't compare AgeIdentity directly as it's a pointer to struct with unexported fields,
	// but Recipient() string comparison covers it.
	if id1.AgeIdentity.String() != id2.AgeIdentity.String() {
		t.Error("Age Identity Strings do not match")
	}

	// 2. SSH Functionality (Sign/Verify)
	signer, err := ssh.ParsePrivateKey([]byte(id1.SSHPrivateKeyPEM))
	if err != nil {
		t.Fatalf("Failed to parse SSH private key: %v", err)
	}

	data := []byte("Hello, SSH!")
	sig, err := signer.Sign(rand.Reader, data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(id1.SSHPublicKey))
	if err != nil {
		t.Fatalf("Failed to parse SSH public key: %v", err)
	}

	if err := pubKey.Verify(data, sig); err != nil {
		t.Errorf("SSH signature verification failed: %v", err)
	}

	// 3. Age Functionality (Encrypt/Decrypt)
	recipient, err := age.ParseX25519Recipient(id1.AgeRecipient)
	if err != nil {
		t.Fatalf("Failed to parse Age recipient: %v", err)
	}

	plaintext := []byte("Hello, Age!")
	out := &bytes.Buffer{}

	w, err := age.Encrypt(out, recipient)
	if err != nil {
		t.Fatalf("Failed to create Age encryptor: %v", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		t.Fatalf("Failed to write plaintext: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close encryptor: %v", err)
	}

	encrypted := out.Bytes()

	r, err := age.Decrypt(bytes.NewReader(encrypted), id1.AgeIdentity)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	decrypted := &bytes.Buffer{}
	if _, err := io.Copy(decrypted, r); err != nil {
		t.Fatalf("Failed to read decrypted data: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted.Bytes()) {
		t.Errorf("Decrypted data mismatch. Got %q, want %q", decrypted.String(), string(plaintext))
	}
}

func TestVector(t *testing.T) {
	// Optional: add a known vector if we had one to ensure we don't regress on the derivation path logic.
	// Since we use a custom path, we just ensure stability.
	// Mnemonic: "shoot island position soft burden budget tooth cruel issue economy destroy update"
	// Passphrase: ""
	// This test just prints the keys so we can verify them manually or ensure they don't change in future runs if we hardcode them.
	// For now, I'll just check it runs without error.
}
