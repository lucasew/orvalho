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

type (
	sha1Hash   struct{ hash.Hash }
	sha256Hash struct{ hash.Hash }
	sha512Hash struct{ hash.Hash }
)

// Sum is a streaming SRI hasher. Write feeds the digest. String is algo-base64.
type Sum struct {
	hash.Hash
}

func SHA1() *Sum   { return &Sum{sha1Hash{sha1.New()}} }
func SHA256() *Sum { return &Sum{sha256Hash{sha256.New()}} }
func SHA512() *Sum { return &Sum{sha512Hash{sha512.New()}} }

func (s *Sum) String() string {
	return s.Algo() + "-" + base64.StdEncoding.EncodeToString(s.Sum(nil))
}

func (s *Sum) Hex() string {
	return hex.EncodeToString(s.Sum(nil))
}

func (s *Sum) Algo() string {
	switch s.Hash.(type) {
	case sha512Hash:
		return "sha512"
	case sha256Hash:
		return "sha256"
	case sha1Hash:
		return "sha1"
	default:
		return ""
	}
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
