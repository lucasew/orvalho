package workers

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strconv"

	"github.com/dop251/goja"
)

// nodeZlibBinding materializes require("zlib") / require("node:zlib").
type nodeZlibBinding struct{}

var _ Binding = nodeZlibBinding{}

func (nodeZlibBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"zlib", "node:zlib"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeZlib(iso), nil
}

type nodeZlib struct {
	iso *Isolate
}

func newNodeZlib(iso *Isolate) *goja.Object {
	n := &nodeZlib{iso: iso}
	obj := iso.vm.NewObject()
	consts := zlibConstants(iso)
	codes := zlibCodes(iso)
	mustDefineRO(iso, obj, "constants", consts)
	mustDefineRO(iso, obj, "codes", codes)
	mustSet(obj, "gzip", n.asyncCodec(gzipEncode))
	mustSet(obj, "gunzip", n.asyncCodec(gzipDecode))
	mustSet(obj, "unzip", n.asyncCodec(gzipDecode))
	mustSet(obj, "deflate", n.asyncCodec(zlibEncode))
	mustSet(obj, "inflate", n.asyncCodec(zlibDecode))
	mustSet(obj, "deflateRaw", n.asyncCodec(rawEncode))
	mustSet(obj, "inflateRaw", n.asyncCodec(rawDecode))
	mustSet(obj, "gzipSync", n.syncCodec(gzipEncode))
	mustSet(obj, "gunzipSync", n.syncCodec(gzipDecode))
	mustSet(obj, "unzipSync", n.syncCodec(gzipDecode))
	mustSet(obj, "deflateSync", n.syncCodec(zlibEncode))
	mustSet(obj, "inflateSync", n.syncCodec(zlibDecode))
	mustSet(obj, "deflateRawSync", n.syncCodec(rawEncode))
	mustSet(obj, "inflateRawSync", n.syncCodec(rawDecode))
	mustSet(obj, "createGzip", n.makeCreateStream(gzipEncode))
	mustSet(obj, "createGunzip", n.makeCreateStream(gzipDecode))
	mustSet(obj, "createDeflate", n.makeCreateStream(zlibEncode))
	mustSet(obj, "createInflate", n.makeCreateStream(zlibDecode))
	mustSet(obj, "createUnzip", n.makeCreateStream(gzipDecode))
	mustSet(obj, "createDeflateRaw", n.makeCreateStream(rawEncode))
	mustSet(obj, "createInflateRaw", n.makeCreateStream(rawDecode))
	mustSet(obj, "default", obj)
	return obj
}

func (n *nodeZlib) asyncCodec(fn func([]byte) ([]byte, error)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		in := n.inputBytes(call.Argument(0))
		cb := lastFunc(call)
		if cb == nil {
			e := n.iso.vm.NewTypeError(`The "callback" argument must be of type function`)
			_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
			panic(e)
		}
		var fire goja.Callable
		fire = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			out, err := fn(in)
			if err != nil {
				_, _ = cb(goja.Undefined(), n.iso.vm.NewGoError(err))
				return goja.Undefined(), nil
			}
			_, _ = cb(goja.Undefined(), goja.Null(), n.encode(out))
			return goja.Undefined(), nil
		}
		n.iso.timers.schedule(fire, nil, 0, 0, n.iso.now())
		return goja.Undefined()
	}
}

func (n *nodeZlib) syncCodec(fn func([]byte) ([]byte, error)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		out, err := fn(n.inputBytes(call.Argument(0)))
		if err != nil {
			panic(n.iso.vm.NewGoError(err))
		}
		return n.encode(out)
	}
}

func (n *nodeZlib) inputBytes(v goja.Value) []byte {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		e := n.iso.vm.NewTypeError(`The "buffer" argument must be of type string or an instance of Buffer`)
		_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
		panic(e)
	}
	if s, ok := v.Export().(string); ok {
		return []byte(s)
	}
	if b, ok := asByteView(v); ok {
		return b
	}
	e := n.iso.vm.NewTypeError(`The "buffer" argument must be of type string or an instance of Buffer`)
	_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
	panic(e)
}

func (n *nodeZlib) encode(data []byte) goja.Value {
	cp := append([]byte(nil), data...)
	v := uint8ArrayOf(n.iso, cp)
	if o, ok := v.(*goja.Object); ok {
		mustSet(o, "toString", func(goja.FunctionCall) string { return string(cp) })
	}
	return v
}

func (n *nodeZlib) makeCreateStream(fn func([]byte) ([]byte, error)) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		var buf []byte
		s := n.iso.vm.NewObject()
		attachEmitter(s)
		emit := func(ev string, args ...goja.Value) {
			if fn, ok := goja.AssertFunction(s.Get("emit")); ok {
				call := append([]goja.Value{n.iso.vm.ToValue(ev)}, args...)
				_, _ = fn(s, call...)
			}
		}
		mustSet(s, "write", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				buf = append(buf, valueBytes(call.Argument(0))...)
			}
			return n.iso.vm.ToValue(true)
		})
		mustSet(s, "end", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				if _, isFn := goja.AssertFunction(call.Argument(0)); !isFn {
					buf = append(buf, valueBytes(call.Argument(0))...)
				}
			}
			out, err := fn(buf)
			if err != nil {
				emit("error", n.iso.vm.NewGoError(err))
				return s
			}
			if len(out) > 0 {
				emit("data", n.encode(out))
			}
			emit("end")
			return s
		})
		mustSet(s, "pipe", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				return call.Argument(0)
			}
			return s
		})
		mustSet(s, "pause", func(call goja.FunctionCall) goja.Value { return s })
		mustSet(s, "resume", func(call goja.FunctionCall) goja.Value { return s })
		mustSet(s, "destroy", func(call goja.FunctionCall) goja.Value { return s })
		return s
	}
}

func gzipEncode(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gzipDecode(in []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func zlibEncode(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zlibDecode(in []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func rawEncode(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(in); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func rawDecode(in []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(in))
	defer r.Close()
	return io.ReadAll(r)
}

func zlibConstants(iso *Isolate) *goja.Object {
	c := iso.vm.NewObject()
	c.SetPrototype(nil)
	vals := map[string]int{
		"Z_NO_FLUSH": 0, "Z_PARTIAL_FLUSH": 1, "Z_SYNC_FLUSH": 2,
		"Z_FULL_FLUSH": 3, "Z_FINISH": 4, "Z_BLOCK": 5, "Z_TREES": 6,
		"Z_OK": 0, "Z_STREAM_END": 1, "Z_NEED_DICT": 2,
		"Z_ERRNO": -1, "Z_STREAM_ERROR": -2, "Z_DATA_ERROR": -3,
		"Z_MEM_ERROR": -4, "Z_BUF_ERROR": -5, "Z_VERSION_ERROR": -6,
		"Z_NO_COMPRESSION": 0, "Z_BEST_SPEED": 1, "Z_BEST_COMPRESSION": 9,
		"Z_DEFAULT_COMPRESSION": -1,
		"Z_FILTERED":            1, "Z_HUFFMAN_ONLY": 2, "Z_RLE": 3, "Z_FIXED": 4,
		"Z_DEFAULT_STRATEGY": 0, "Z_DEFLATED": 8,
		"BROTLI_PARAM_QUALITY": 1, "BROTLI_PARAM_SIZE_HINT": 3,
	}
	for k, v := range vals {
		mustDefineRO(iso, c, k, v)
	}
	jsFreeze(iso, c)
	return c
}

func zlibCodes(iso *Isolate) *goja.Object {
	c := iso.vm.NewObject()
	c.SetPrototype(nil)
	pairs := []struct {
		name string
		code int
	}{
		{"Z_OK", 0}, {"Z_STREAM_END", 1}, {"Z_NEED_DICT", 2},
		{"Z_ERRNO", -1}, {"Z_STREAM_ERROR", -2}, {"Z_DATA_ERROR", -3},
		{"Z_MEM_ERROR", -4}, {"Z_BUF_ERROR", -5}, {"Z_VERSION_ERROR", -6},
	}
	for _, p := range pairs {
		mustDefineRO(iso, c, p.name, p.code)
		mustDefineRO(iso, c, strconv.Itoa(p.code), p.name)
	}
	jsFreeze(iso, c)
	return c
}

func mustDefineRO(iso *Isolate, obj *goja.Object, name string, val any) {
	held := iso.vm.ToValue(val)
	get := iso.vm.ToValue(func(goja.FunctionCall) goja.Value { return held })
	set := iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		panic(iso.vm.NewTypeError("Cannot assign to read only property '" + name + "' of object"))
	})
	if err := obj.DefineAccessorProperty(name, get, set, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		mustSet(obj, name, val)
	}
}

func jsFreeze(iso *Isolate, obj *goja.Object) {
	o := iso.vm.Get("Object")
	if o == nil {
		return
	}
	fn, ok := goja.AssertFunction(o.ToObject(iso.vm).Get("freeze"))
	if !ok {
		return
	}
	_, _ = fn(goja.Undefined(), obj)
}
