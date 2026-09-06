package dependency

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

// Sum is a streaming SRI hasher. String is algo-base64.
type Sum struct {
	hash.Hash
	algo string
}

func SHA1() *Sum   { return &Sum{sha1.New(), "sha1"} }
func SHA256() *Sum { return &Sum{sha256.New(), "sha256"} }
func SHA512() *Sum { return &Sum{sha512.New(), "sha512"} }

func (s *Sum) String() string {
	return s.algo + "-" + base64.StdEncoding.EncodeToString(s.Sum(nil))
}

func (s *Sum) Hex() string {
	return hex.EncodeToString(s.Sum(nil))
}

func (s *Sum) Algo() string {
	return s.algo
}

// ParseIntegrity splits an npm SRI string into fetchurl algo + lowercase hex.
// The first algorithm in a space-separated list wins.
func ParseIntegrity(sri string) (algo, hash string, err error) {
	sri = strings.TrimSpace(sri)
	if sri == "" {
		return "", "", fmt.Errorf("%w: empty", ErrIntegrity)
	}
	first := sri
	if i := strings.IndexByte(sri, ' '); i >= 0 {
		first = sri[:i]
	}
	algo, rest, ok := strings.Cut(first, "-")
	if !ok || rest == "" {
		return "", "", fmt.Errorf("%w: %q", ErrIntegrity, sri)
	}
	algo = strings.ToLower(strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return -1
		}
	}, algo))
	raw, err := decodeSRI(rest)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrIntegrity, err)
	}
	return algo, hex.EncodeToString(raw), nil
}

func decodeSRI(rest string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(rest)
	if err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(rest, "="))
}
