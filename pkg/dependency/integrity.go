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

// Sum is a streaming SRI hasher. Write feeds the digest. String is algo-base64.
type Sum struct {
	hash.Hash
}

type (
	sha1Sum   struct{ hash.Hash }
	sha256Sum struct{ hash.Hash }
	sha512Sum struct{ hash.Hash }
)

// NewSum wraps h so Algo can type-switch (stdlib digest types are unexported).
func NewSum(h hash.Hash) *Sum {
	switch h.Size() {
	case sha512.Size:
		return &Sum{sha512Sum{h}}
	case sha256.Size:
		return &Sum{sha256Sum{h}}
	case sha1.Size:
		return &Sum{sha1Sum{h}}
	default:
		return &Sum{h}
	}
}

func (s *Sum) String() string {
	return s.Algo() + "-" + base64.StdEncoding.EncodeToString(s.Sum(nil))
}

func (s *Sum) Hex() string {
	return hex.EncodeToString(s.Sum(nil))
}

func (s *Sum) Algo() string {
	switch s.Hash.(type) {
	case sha512Sum:
		return "sha512"
	case sha256Sum:
		return "sha256"
	case sha1Sum:
		return "sha1"
	default:
		return fmt.Sprintf("sha%d", 8*s.Size())
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
