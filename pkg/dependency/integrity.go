package dependency

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// ParseIntegrity splits an npm SRI string (sha512-<base64>) into fetchurl
// algo + lowercase hex. The first algorithm in a space-separated list wins.
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
	raw, err := base64.StdEncoding.DecodeString(rest)
	if err != nil {
		// Some registries emit unpadded base64.
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(rest, "="))
		if err != nil {
			return "", "", fmt.Errorf("%w: %v", ErrIntegrity, err)
		}
	}
	return algo, hex.EncodeToString(raw), nil
}
