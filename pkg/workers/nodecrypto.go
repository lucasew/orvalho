package workers

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
	"math"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// nodeCryptoBinding materializes guest require("crypto") / require("node:crypto").
type nodeCryptoBinding struct{}

var _ Binding = nodeCryptoBinding{}

func (nodeCryptoBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"crypto", "node:crypto"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeCrypto(iso), nil
}

type nodeCrypto struct {
	iso *Isolate
}

func newNodeCrypto(iso *Isolate) *goja.Object {
	n := &nodeCrypto{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "randomBytes", n.jsRandomBytes)
	mustSet(obj, "randomFillSync", n.jsRandomFillSync)
	mustSet(obj, "randomFill", n.jsRandomFill)
	mustSet(obj, "randomUUID", n.jsRandomUUID)
	mustSet(obj, "createHash", n.jsCreateHash)
	mustSet(obj, "hash", n.jsHash)
	mustSet(obj, "getHashes", n.jsGetHashes)
	mustSet(obj, "getRandomValues", n.jsGetRandomValues)
	web := iso.vm.NewObject()
	mustSet(web, "getRandomValues", obj.Get("getRandomValues"))
	mustSet(obj, "webcrypto", web)
	mustSet(obj, "default", obj)
	return obj
}

func (n *nodeCrypto) jsGetHashes(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue([]string{"md5", "sha1", "sha256"})
}

func (n *nodeCrypto) jsRandomBytes(call goja.FunctionCall) goja.Value {
	size := n.requireSize(call.Argument(0), "size")
	var cb goja.Callable
	if len(call.Arguments) > 1 {
		if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
			cb = fn
		} else if !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			n.throwType("ERR_INVALID_ARG_TYPE", `The "callback" argument must be of type function. Received `+jsReceived(call.Argument(1)))
		}
	}
	buf := n.randBytes(size)
	if cb == nil {
		return n.uint8Array(buf)
	}
	n.nextTick(cb, goja.Null(), n.uint8Array(buf))
	return goja.Undefined()
}

func (n *nodeCrypto) jsRandomFillSync(call goja.FunctionCall) goja.Value {
	view, buf, _ := n.parseFill(call, false)
	n.fillRand(view)
	return buf
}

func (n *nodeCrypto) jsRandomFill(call goja.FunctionCall) goja.Value {
	view, buf, cb := n.parseFill(call, true)
	n.fillRand(view)
	n.nextTick(cb, goja.Null(), buf)
	return goja.Undefined()
}

func (n *nodeCrypto) jsGetRandomValues(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "typedArray" argument must be an integer TypedArray. Received undefined`)
	}
	buf := call.Argument(0)
	view, ok := asByteView(buf)
	if !ok {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "typedArray" argument must be an integer TypedArray. Received `+jsReceived(buf))
	}
	if len(view) > 65536 {
		n.throwError("ERR_CRYPTO_OPERATION_FAILED", "The ArrayBufferView's byte length ("+strconv.Itoa(len(view))+") exceeds the number of bytes of entropy available via this API (65536).")
	}
	n.fillRand(view)
	return buf
}

func (n *nodeCrypto) jsRandomUUID(goja.FunctionCall) goja.Value {
	var b [16]byte
	n.fillRand(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	const hexdigits = "0123456789abcdef"
	var out [36]byte
	j := 0
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[j] = '-'
			j++
		}
		out[j] = hexdigits[v>>4]
		out[j+1] = hexdigits[v&0x0f]
		j += 2
	}
	return n.iso.vm.ToValue(string(out[:]))
}

func (n *nodeCrypto) jsHash(call goja.FunctionCall) goja.Value {
	algo := n.requireString(call.Argument(0), "algorithm")
	h, ok := newHasher(algo)
	if !ok {
		n.throwError("ERR_CRYPTO_UNKNOWN_DIGEST", "Digest method not supported")
	}
	if len(call.Arguments) < 2 {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "data" argument must be of type string or an instance of Buffer, TypedArray, or DataView. Received undefined`)
	}
	enc := ""
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
		if _, ok := exportJSString(call.Argument(2)); !ok {
			n.throwType("ERR_INVALID_ARG_TYPE", `The "outputEncoding" argument must be of type string. Received `+jsReceived(call.Argument(2)))
		}
		enc = call.Argument(2).String()
	}
	nh := &nodeHash{iso: n.iso, h: h}
	data := nh.bytesFrom(call.Argument(1), "")
	_, _ = h.Write(data)
	return nh.encodeDigest(h.Sum(nil), enc)
}

func (n *nodeCrypto) jsCreateHash(call goja.FunctionCall) goja.Value {
	algo := n.requireString(call.Argument(0), "algorithm")
	h, ok := newHasher(algo)
	if !ok {
		n.throwError("ERR_CRYPTO_UNKNOWN_DIGEST", "Digest method not supported")
	}
	nh := &nodeHash{iso: n.iso, h: h}
	obj := n.iso.vm.NewObject()
	mustSet(obj, "update", nh.jsUpdate)
	mustSet(obj, "digest", nh.jsDigest)
	return obj
}

type nodeHash struct {
	iso  *Isolate
	h    hash.Hash
	done bool
}

func (h *nodeHash) jsUpdate(call goja.FunctionCall) goja.Value {
	if h.done {
		h.throwError("ERR_CRYPTO_HASH_FINALIZED", "Digest already called")
	}
	if len(call.Arguments) == 0 {
		h.throwType("ERR_INVALID_ARG_TYPE", `The "data" argument must be of type string or an instance of Buffer, TypedArray, or DataView. Received undefined`)
	}
	enc := ""
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		if _, ok := call.Argument(1).Export().(string); !ok {
			h.throwType("ERR_INVALID_ARG_TYPE", `The "inputEncoding" argument must be of type string. Received `+jsReceived(call.Argument(1)))
		}
		enc = call.Argument(1).String()
	}
	data := h.bytesFrom(call.Argument(0), enc)
	_, _ = h.h.Write(data)
	return call.This
}

func (h *nodeHash) jsDigest(call goja.FunctionCall) goja.Value {
	if h.done {
		h.throwError("ERR_CRYPTO_HASH_FINALIZED", "Digest already called")
	}
	h.done = true
	sum := h.h.Sum(nil)
	enc := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		if _, ok := call.Argument(0).Export().(string); !ok {
			h.throwType("ERR_INVALID_ARG_TYPE", `The "encoding" argument must be of type string. Received `+jsReceived(call.Argument(0)))
		}
		enc = call.Argument(0).String()
	}
	return h.encodeDigest(sum, enc)
}

func (h *nodeHash) encodeDigest(sum []byte, enc string) goja.Value {
	switch normalizeEnc(enc) {
	case "", "buffer":
		return uint8ArrayOf(h.iso, sum)
	case "hex":
		return h.iso.vm.ToValue(hex.EncodeToString(sum))
	case "base64":
		return h.iso.vm.ToValue(base64.StdEncoding.EncodeToString(sum))
	case "base64url":
		return h.iso.vm.ToValue(base64.RawURLEncoding.EncodeToString(sum))
	case "latin1", "binary":
		runes := make([]rune, len(sum))
		for i, b := range sum {
			runes[i] = rune(b)
		}
		return h.iso.vm.ToValue(string(runes))
	case "utf8":
		return h.iso.vm.ToValue(string(sum))
	default:
		h.throwType("ERR_UNKNOWN_ENCODING", "Unknown encoding: "+enc)
	}
	return goja.Undefined()
}

func (h *nodeHash) bytesFrom(v goja.Value, enc string) []byte {
	if s, ok := exportJSString(v); ok {
		return decodeEncoded(h.iso, s, enc)
	}
	if b, ok := asByteView(v); ok {
		return b
	}
	h.throwType("ERR_INVALID_ARG_TYPE", `The "data" argument must be of type string or an instance of Buffer, TypedArray, or DataView. Received `+jsReceived(v))
	return nil
}

func (h *nodeHash) throwType(code, msg string) {
	throwJS(h.iso, "TypeError", code, msg)
}

func (h *nodeHash) throwError(code, msg string) {
	throwJS(h.iso, "Error", code, msg)
}

func (n *nodeCrypto) parseFill(call goja.FunctionCall, needCB bool) ([]byte, goja.Value, goja.Callable) {
	if len(call.Arguments) == 0 {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "buffer" argument must be an instance of Buffer, TypedArray, DataView, or ArrayBuffer. Received undefined`)
	}
	buf := call.Argument(0)
	view, ok := asByteView(buf)
	if !ok {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "buffer" argument must be an instance of Buffer, TypedArray, DataView, or ArrayBuffer. Received `+jsReceived(buf))
	}

	offset := 0
	size := len(view)
	var cb goja.Callable
	i := 1
	if i < len(call.Arguments) {
		a := call.Argument(i)
		if fn, ok := goja.AssertFunction(a); ok {
			cb = fn
			i = len(call.Arguments)
		} else if !goja.IsUndefined(a) {
			offset = n.requireSize(a, "offset")
			i++
		} else {
			i++
		}
	}
	if i < len(call.Arguments) {
		a := call.Argument(i)
		if fn, ok := goja.AssertFunction(a); ok {
			cb = fn
			i = len(call.Arguments)
		} else if !goja.IsUndefined(a) {
			size = n.requireSize(a, "size")
			i++
		} else {
			i++
		}
	}
	if i < len(call.Arguments) {
		if fn, ok := goja.AssertFunction(call.Argument(i)); ok {
			cb = fn
		} else if !goja.IsUndefined(call.Argument(i)) && !goja.IsNull(call.Argument(i)) {
			n.throwType("ERR_INVALID_ARG_TYPE", `The "callback" argument must be of type function. Received `+jsReceived(call.Argument(i)))
		}
	}
	if offset > len(view) {
		n.throwRange("ERR_OUT_OF_RANGE", `The value of "offset" is out of range. It must be >= 0 && <= `+strconv.Itoa(len(view))+`. Received `+strconv.Itoa(offset))
	}
	if size > len(view)-offset {
		n.throwRange("ERR_OUT_OF_RANGE", `The value of "size" is out of range. It must be >= 0 && <= `+strconv.Itoa(len(view)-offset)+`. Received `+strconv.Itoa(size))
	}
	if needCB && cb == nil {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "callback" argument must be of type function. Received undefined`)
	}
	return view[offset : offset+size], buf, cb
}

func (n *nodeCrypto) requireSize(v goja.Value, name string) int {
	f, ok := jsNumber(v)
	if !ok {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "`+name+`" argument must be of type number. Received `+jsReceived(v))
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < 0 || f > math.MaxInt32 {
		n.throwRange("ERR_OUT_OF_RANGE", `The value of "`+name+`" is out of range. It must be >= 0 && <= 2147483647. Received `+v.String())
	}
	return int(f)
}

func (n *nodeCrypto) requireString(v goja.Value, name string) string {
	s, ok := exportJSString(v)
	if !ok {
		n.throwType("ERR_INVALID_ARG_TYPE", `The "`+name+`" argument must be of type string. Received `+jsReceived(v))
	}
	return s
}

func (n *nodeCrypto) randBytes(size int) []byte {
	b := make([]byte, size)
	n.fillRand(b)
	return b
}

func (n *nodeCrypto) fillRand(b []byte) {
	if len(b) == 0 {
		return
	}
	if _, err := rand.Read(b); err != nil {
		n.throwError("ERR_CRYPTO_OPERATION_FAILED", "random fill failed")
	}
}

func (n *nodeCrypto) uint8Array(b []byte) goja.Value {
	return uint8ArrayOf(n.iso, b)
}

func (n *nodeCrypto) nextTick(fn goja.Callable, args ...goja.Value) {
	n.iso.timers.schedule(fn, args, 0, 0, n.iso.now())
}

func (n *nodeCrypto) throwType(code, msg string) {
	throwJS(n.iso, "TypeError", code, msg)
}

func (n *nodeCrypto) throwRange(code, msg string) {
	throwJS(n.iso, "RangeError", code, msg)
}

func (n *nodeCrypto) throwError(code, msg string) {
	throwJS(n.iso, "Error", code, msg)
}

func uint8ArrayOf(iso *Isolate, b []byte) goja.Value {
	ab := iso.vm.NewArrayBuffer(b)
	ctor, ok := goja.AssertConstructor(iso.vm.Get("Uint8Array"))
	if !ok {
		return iso.vm.ToValue(ab)
	}
	v, err := ctor(nil, iso.vm.ToValue(ab))
	if err != nil {
		panic(err)
	}
	return v
}

func newHasher(name string) (hash.Hash, bool) {
	switch strings.ToLower(name) {
	case "md5":
		return md5.New(), true
	case "sha1":
		return sha1.New(), true
	case "sha256":
		return sha256.New(), true
	}
	return nil, false
}

func asByteView(v goja.Value) ([]byte, bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, false
	}
	switch x := v.Export().(type) {
	case []byte:
		return x, true
	case goja.ArrayBuffer:
		if x.Bytes() == nil {
			return nil, false
		}
		return x.Bytes(), true
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, false
	}
	buf := obj.Get("buffer")
	if buf == nil || goja.IsUndefined(buf) || goja.IsNull(buf) {
		return nil, false
	}
	ab, ok := buf.Export().(goja.ArrayBuffer)
	if !ok || ab.Bytes() == nil {
		return nil, false
	}
	data := ab.Bytes()
	off := 0
	if o := obj.Get("byteOffset"); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
		off = int(o.ToInteger())
	}
	ln := len(data) - off
	if l := obj.Get("byteLength"); l != nil && !goja.IsUndefined(l) && !goja.IsNull(l) {
		ln = int(l.ToInteger())
	}
	if off < 0 || ln < 0 || off+ln > len(data) {
		return nil, false
	}
	return data[off : off+ln], true
}

func decodeEncoded(iso *Isolate, s, enc string) []byte {
	switch normalizeEnc(enc) {
	case "", "utf8", "ascii":
		return []byte(s)
	case "hex":
		if len(s)%2 == 1 {
			s = s[:len(s)-1]
		}
		b, err := hex.DecodeString(s)
		if err != nil {
			throwJS(iso, "TypeError", "ERR_INVALID_ARG_VALUE", "The argument 'encoding' is invalid for data of length "+strconv.Itoa(len(s)))
		}
		return b
	case "base64":
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(s)
		}
		if err != nil {
			return []byte{}
		}
		return b
	case "base64url":
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			b, err = base64.URLEncoding.DecodeString(s)
		}
		if err != nil {
			return []byte{}
		}
		return b
	case "latin1", "binary":
		out := make([]byte, 0, len(s))
		for _, r := range s {
			out = append(out, byte(r))
		}
		return out
	default:
		throwJS(iso, "TypeError", "ERR_UNKNOWN_ENCODING", "Unknown encoding: "+enc)
		return nil
	}
}

func normalizeEnc(enc string) string {
	e := strings.ToLower(enc)
	e = strings.ReplaceAll(e, "-", "")
	e = strings.ReplaceAll(e, "_", "")
	if e == "utf8" || e == "utf16le" || e == "ucs2" {
		return e
	}
	return strings.ToLower(enc)
}

func exportJSString(v goja.Value) (string, bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", false
	}
	if s, ok := v.Export().(string); ok {
		return s, true
	}
	return "", false
}

func jsNumber(v goja.Value) (float64, bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0, false
	}
	switch x := v.Export().(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func jsReceived(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	return v.String()
}

func throwJS(iso *Isolate, kind, code, msg string) {
	ctor, ok := goja.AssertConstructor(iso.vm.Get(kind))
	if !ok {
		e := iso.vm.NewGoError(errors.New(msg))
		_ = e.Set("code", code)
		panic(e)
	}
	o, err := ctor(nil, iso.vm.ToValue(msg))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", code)
	panic(o)
}
