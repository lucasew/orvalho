package workers

import (
	"bytes"
	"unicode/utf8"

	"github.com/dop251/goja"
)

func (iso *Isolate) installTextCodec() {
	iso.vm.Set("TextEncoder", func(call goja.ConstructorCall) *goja.Object {
		return call.This
	})
	encProto := iso.vm.Get("TextEncoder").ToObject(iso.vm).Get("prototype").ToObject(iso.vm)
	mustSet(encProto, "encode", iso.textEncode)
	mustSet(encProto, "encodeInto", iso.textEncodeInto)
	mustSet(encProto, "encoding", "utf-8")

	iso.vm.Set("TextDecoder", func(call goja.ConstructorCall) *goja.Object {
		return call.This
	})
	decProto := iso.vm.Get("TextDecoder").ToObject(iso.vm).Get("prototype").ToObject(iso.vm)
	mustSet(decProto, "decode", iso.textDecode)
	mustSet(decProto, "encoding", "utf-8")
}

func (iso *Isolate) textEncode(call goja.FunctionCall) goja.Value {
	s := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		s = call.Argument(0).String()
	}
	return iso.uint8Array([]byte(s))
}

func (iso *Isolate) textEncodeInto(call goja.FunctionCall) goja.Value {
	s := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		s = call.Argument(0).String()
	}
	dst := destBytes(call.Argument(1))
	jsRead, written := 0, 0
	byteOff := 0
	for byteOff < len(s) {
		r, size := utf8.DecodeRuneInString(s[byteOff:])
		n := utf8.RuneLen(r)
		if n < 0 || written+n > len(dst) {
			break
		}
		copy(dst[written:], s[byteOff:byteOff+size])
		written += n
		byteOff += size
		if r > 0xFFFF {
			jsRead += 2
		} else {
			jsRead += 1
		}
	}
	out := iso.vm.NewObject()
	mustSet(out, "read", jsRead)
	mustSet(out, "written", written)
	return out
}

func (iso *Isolate) textDecode(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		return iso.vm.ToValue("")
	}
	b := valueBytes(call.Argument(0))
	if !utf8.Valid(b) {
		b = bytes.ToValidUTF8(b, []byte("\uFFFD"))
	}
	return iso.vm.ToValue(string(b))
}

func (iso *Isolate) uint8Array(b []byte) goja.Value {
	ab := iso.vm.NewArrayBuffer(b)
	ctor, ok := iso.vm.Get("Uint8Array").(*goja.Object)
	if !ok {
		return iso.vm.ToValue(ab)
	}
	v, err := iso.vm.New(ctor, iso.vm.ToValue(ab))
	if err != nil {
		return iso.vm.ToValue(ab)
	}
	return v
}

// destBytes is the live backing store of a TypedArray/DataView (no copy).
func destBytes(v goja.Value) []byte {
	o, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	buf := o.Get("buffer")
	if buf == nil || goja.IsUndefined(buf) || goja.IsNull(buf) {
		return nil
	}
	ab, ok := buf.Export().(goja.ArrayBuffer)
	if !ok {
		return nil
	}
	raw := ab.Bytes()
	if raw == nil {
		return nil
	}
	off := 0
	if x := o.Get("byteOffset"); x != nil && !goja.IsUndefined(x) {
		off = int(x.ToInteger())
	}
	n := len(raw) - off
	if x := o.Get("byteLength"); x != nil && !goja.IsUndefined(x) {
		n = int(x.ToInteger())
	}
	if off < 0 || n < 0 || off+n > len(raw) {
		return nil
	}
	return raw[off : off+n]
}
