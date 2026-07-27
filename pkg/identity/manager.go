// Package identity provides manager key material for Orvalho.
//
// A manager identity is an Ed25519 keypair used as the sole install and
// deploy authority for a mesh (see SPEC.md). This package generates,
// persists, and loads that key material and exposes a stable public id.
// Package signing and mesh coupling live elsewhere.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// PEMTypePrivate is the PEM type for manager private keys (PKCS#8).
	PEMTypePrivate = "PRIVATE KEY"

	// DefaultKeyFile is the default relative path for dev key material.
	DefaultKeyFile = "manager.key"
)

// Sentinel errors for identity key material.
var (
	ErrNoPEMBlock        = errors.New("no PEM block found")
	ErrMultiplePEMBlocks = errors.New("multiple PEM blocks found")
	ErrEmptyPath         = errors.New("path is empty")
	ErrKeyFileExists     = errors.New("refusing to overwrite existing key file")
	ErrBadSeedLength     = errors.New("seed length wrong")
	ErrUnexpectedPEMType = errors.New("unexpected PEM type")
	ErrWrongKeyType      = errors.New("private key is not ed25519")
	ErrBadKeyLength      = errors.New("ed25519 private key length wrong")
)

// Manager is a manager authority keypair.
type Manager struct {
	privateKey ed25519.PrivateKey
}

// Generate creates a new random manager identity.
func Generate() (*Manager, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate manager key: %w", err)
	}
	return &Manager{privateKey: priv}, nil
}

// FromSeed builds a manager identity from a 32-byte Ed25519 seed.
// Useful for deterministic tests; production callers should use Generate.
func FromSeed(seed []byte) (*Manager, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%w: %d, want %d", ErrBadSeedLength, len(seed), ed25519.SeedSize)
	}
	return &Manager{privateKey: ed25519.NewKeyFromSeed(seed)}, nil
}

// PublicKey returns the Ed25519 public key.
func (m *Manager) PublicKey() ed25519.PublicKey {
	return m.privateKey.Public().(ed25519.PublicKey)
}

// PrivateKey returns the Ed25519 private key.
// Callers that only need to identify the manager should use PublicID.
func (m *Manager) PrivateKey() ed25519.PrivateKey {
	return m.privateKey
}

// PublicID returns a stable, non-secret identifier for this manager.
// Format: base64.RawURLEncoding of the 32-byte raw Ed25519 public key.
func (m *Manager) PublicID() string {
	return base64.RawURLEncoding.EncodeToString(m.PublicKey())
}

// Equal reports whether m and other have the same public key.
func (m *Manager) Equal(other *Manager) bool {
	if m == nil || other == nil {
		return m == other
	}
	return m.PublicKey().Equal(other.PublicKey())
}

// MarshalPrivatePEM encodes the private key as PKCS#8 PEM.
// Encoding is deterministic for a given key (unlike OpenSSH private key PEM,
// which embeds random check integers).
func (m *Manager) MarshalPrivatePEM() ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(m.privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: PEMTypePrivate, Bytes: der}), nil
}

// ParsePrivatePEM loads a manager identity from PKCS#8 PEM bytes.
func ParsePrivatePEM(pemBytes []byte) (*Manager, error) {
	block, rest := pem.Decode(pemBytes)
	if block == nil {
		return nil, ErrNoPEMBlock
	}
	extra, trailing := pem.Decode(rest)
	if extra != nil {
		return nil, ErrMultiplePEMBlocks
	}
	_ = trailing
	if block.Type != PEMTypePrivate {
		return nil, fmt.Errorf("%w %q, want %q", ErrUnexpectedPEMType, block.Type, PEMTypePrivate)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrWrongKeyType, key)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: %d, want %d", ErrBadKeyLength, len(priv), ed25519.PrivateKeySize)
	}
	return &Manager{privateKey: priv}, nil
}

// Save writes the private key PEM to path with mode 0600.
// Parent directories are created with mode 0700 when missing.
// Refuses to overwrite an existing file unless overwrite is true.
func (m *Manager) Save(path string, overwrite bool) error {
	if path == "" {
		return ErrEmptyPath
	}
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w %q", ErrKeyFileExists, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat key file: %w", err)
		}
	}

	pemBytes, err := m.MarshalPrivatePEM()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create key directory: %w", err)
		}
	}

	// Write via temp file in the same directory for atomic replace.
	tmp, err := os.CreateTemp(dir, ".manager-key-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp key file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			if remErr := os.Remove(tmpName); remErr != nil && !errors.Is(remErr, os.ErrNotExist) {
				// Best-effort cleanup after a failed install.
			}
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		closeErr := tmp.Close()
		return errors.Join(fmt.Errorf("chmod temp key file: %w", err), closeErr)
	}
	if _, err := tmp.Write(pemBytes); err != nil {
		closeErr := tmp.Close()
		return errors.Join(fmt.Errorf("write temp key file: %w", err), closeErr)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp key file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install key file: %w", err)
	}
	cleanup = false

	// Best-effort: ensure final mode is 0600 even if umask interfered on some platforms.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod key file: %w", err)
	}
	return nil
}

// Load reads a manager identity from a PKCS#8 PEM file.
func Load(path string) (*Manager, error) {
	if path == "" {
		return nil, ErrEmptyPath
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	m, err := ParsePrivatePEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("parse key file %q: %w", path, err)
	}
	return m, nil
}
