package identity

import (
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"strings"

	"filippo.io/age"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/stellar/go/exp/crypto/derivation"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/ssh"
)

// Constants defined in requirements
const (
	CoinType    = 59356 // 59356'
	SSHPathBase = "m/44'/59356'/0'/0'"
	AgePathBase = "m/44'/59356'/1'/0'"
)

// Identity holds the derived keys and identity information.
type Identity struct {
	SSHPrivateKeyPEM string
	SSHPublicKey     string
	AgeIdentity      *age.X25519Identity
	AgeRecipient     string
}

// Derive derives SSH and Age keys from a BIP-39 mnemonic.
func Derive(mnemonic, passphrase string) (*Identity, error) {
	// 1. Generate Seed from Mnemonic
	seed := bip39.NewSeed(mnemonic, passphrase)

	// 2. Derive SSH Key
	sshPEM, sshPub, err := deriveSSHKey(seed)
	if err != nil {
		return nil, err
	}

	// 3. Derive Age Key
	ageIdentity, ageRecipient, err := deriveAgeKey(seed)
	if err != nil {
		return nil, err
	}

	return &Identity{
		SSHPrivateKeyPEM: sshPEM,
		SSHPublicKey:     sshPub,
		AgeIdentity:      ageIdentity,
		AgeRecipient:     ageRecipient,
	}, nil
}

func deriveSSHKey(seed []byte) (string, string, error) {
	// Path: m / 44' / 59356' / 0' / 0'
	sshKey, err := derivation.DeriveForPath(SSHPathBase, seed)
	if err != nil {
		return "", "", fmt.Errorf("failed to derive SSH key path: %w", err)
	}

	// The derived key is the seed for Ed25519
	if len(sshKey.Key) != 32 {
		return "", "", fmt.Errorf("derived SSH key length is %d, expected 32", len(sshKey.Key))
	}
	sshPrivKey := ed25519.NewKeyFromSeed(sshKey.Key)
	sshPubKey := sshPrivKey.Public().(ed25519.PublicKey)

	// Convert to SSH Public Key format
	sshPubObj, err := ssh.NewPublicKey(sshPubKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create SSH public key: %w", err)
	}
	sshAuthorizedKey := string(ssh.MarshalAuthorizedKey(sshPubObj))

	// Convert to PEM format (OpenSSH)
	pemBlock, err := ssh.MarshalPrivateKey(sshPrivKey, "")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal SSH private key: %w", err)
	}
	sshPemBytes := pem.EncodeToMemory(pemBlock)

	return string(sshPemBytes), strings.TrimSpace(sshAuthorizedKey), nil
}

func deriveAgeKey(seed []byte) (*age.X25519Identity, string, error) {
	// Path: m / 44' / 59356' / 1' / 0'
	ageKey, err := derivation.DeriveForPath(AgePathBase, seed)
	if err != nil {
		return nil, "", fmt.Errorf("failed to derive Age key path: %w", err)
	}

	if len(ageKey.Key) != 32 {
		return nil, "", fmt.Errorf("derived Age key length is %d, expected 32", len(ageKey.Key))
	}

	// Encode to Bech32 with HRP "AGE-SECRET-KEY-" to use age.ParseX25519Identity
	converted, err := bech32.ConvertBits(ageKey.Key, 8, 5, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert bits for bech32: %w", err)
	}

	encoded, err := bech32.Encode("age-secret-key-", converted)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode bech32: %w", err)
	}

	// age expects uppercase HRP for secret keys?
	encoded = strings.ToUpper(encoded)

	ageIdentity, err := age.ParseX25519Identity(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse generated Age identity: %w", err)
	}

	return ageIdentity, ageIdentity.Recipient().String(), nil
}
