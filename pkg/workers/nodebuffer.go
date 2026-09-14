package workers

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// Native Buffer.toString / atob / btoa. The JS versions did
// `s += String.fromCharCode(b[i])` per byte; goja copies on every
// Concat, so Vite's readFile().toString() allocated hundreds of GB.

func (iso *Isolate) jsBufferToString(call goja.FunctionCall) goja.Value {
	raw := destBytes(call.Argument(0))
	if raw == nil {
		if o, ok := call.Argument(0).(*goja.Object); ok {
			raw = typedArrayBytes(o)
		}
	}
	if raw == nil {
		return iso.vm.ToValue("")
	}
	start := uint32(0)
	end := uint32(len(raw))
	if len(call.Arguments) > 2 {
		start = toUint32(call.Argument(2))
	}
	if len(call.Arguments) > 3 && !goja.IsUndefined(call.Argument(3)) && !goja.IsNull(call.Argument(3)) {
		end = toUint32(call.Argument(3))
	}
	if start > uint32(len(raw)) {
		start = uint32(len(raw))
	}
	if end > uint32(len(raw)) {
		end = uint32(len(raw))
	}
	if start >= end {
		return iso.vm.ToValue("")
	}
	return iso.vm.ToValue(decodeBuffer(raw[start:end], bufferEncOf(call.Argument(1))))
}

func bufferEncOf(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "utf8"
	}
	enc := strings.ToLower(v.String())
	switch enc {
	case "utf-8", "utf8":
		return "utf8"
	case "ucs2", "ucs-2", "utf16le", "utf-16le":
		return "utf16le"
	case "binary":
		return "latin1"
	default:
		return enc
	}
}

func decodeBuffer(b []byte, enc string) string {
	switch enc {
	case "hex":
		return hex.EncodeToString(b)
	case "base64":
		return base64.StdEncoding.EncodeToString(b)
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(b)
	case "latin1":
		return latin1ToString(b)
	case "ascii":
		out := make([]byte, len(b))
		for i, c := range b {
			out[i] = c & 0x7f
		}
		return string(out)
	case "utf16le":
		if len(b)%2 == 1 {
			b = b[:len(b)-1]
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
		}
		return string(utf16.Decode(u))
	default:
		if !utf8.Valid(b) {
			b = bytes.ToValidUTF8(b, []byte("\uFFFD"))
		}
		return string(b)
	}
}

func latin1ToString(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func toUint32(v goja.Value) uint32 {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	return uint32(v.ToInteger())
}

func (iso *Isolate) jsBtoa(call goja.FunctionCall) goja.Value {
	s := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		s = call.Argument(0).String()
	}
	raw := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 255 {
			panic(iso.vm.NewTypeError("InvalidCharacterError"))
		}
		raw = append(raw, byte(r))
	}
	return iso.vm.ToValue(base64.StdEncoding.EncodeToString(raw))
}

func (iso *Isolate) jsAtob(call goja.FunctionCall) goja.Value {
	s := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		s = call.Argument(0).String()
	}
	cleaned := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' {
			cleaned = append(cleaned, c)
		}
	}
	for len(cleaned)%4 != 0 {
		cleaned = append(cleaned, '=')
	}
	raw, err := base64.StdEncoding.DecodeString(string(cleaned))
	if err != nil {
		panic(iso.vm.NewTypeError("InvalidCharacterError"))
	}
	return iso.vm.ToValue(latin1ToString(raw))
}
