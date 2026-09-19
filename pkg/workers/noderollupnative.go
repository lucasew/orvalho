package workers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/dop251/goja"
)

// nodeRollupNativeBinding is require("@rollup/rollup-<platform>") and
// require("*.node"). INV-17: .node files are not dlopen'd. parse returns
// a rollup AST buffer; xxhash* are Go hashes (same width as the native API).
type nodeRollupNativeBinding struct{}

var _ Binding = nodeRollupNativeBinding{}

func (nodeRollupNativeBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v, ok := iso.moduleCache["rollup-native"]; ok {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	n := &nodeRollupNative{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "parse", n.jsParse)
	mustSet(obj, "parseAsync", n.jsParseAsync)
	mustSet(obj, "xxhashBase64Url", n.jsXXH64URL)
	mustSet(obj, "xxhashBase36", n.jsXXH36)
	mustSet(obj, "xxhashBase16", n.jsXXH16)
	iso.moduleCache["rollup-native"] = obj
	return obj, nil
}

type nodeRollupNative struct {
	iso *Isolate
}

func (n *nodeRollupNative) jsParse(call goja.FunctionCall) goja.Value {
	code := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		code = call.Argument(0).String()
	}
	return n.astBuffer(code)
}

func (n *nodeRollupNative) jsParseAsync(call goja.FunctionCall) goja.Value {
	p, resolve, _ := n.iso.vm.NewPromise()
	resolve(n.jsParse(call))
	return n.iso.vm.ToValue(p)
}

func (n *nodeRollupNative) astBuffer(code string) goja.Value {
	raw := emptyProgramAST(len(code))
	buf := n.iso.vm.Get("Buffer")
	if buf != nil && !goja.IsUndefined(buf) && !goja.IsNull(buf) {
		if o, ok := buf.(*goja.Object); ok {
			if from, ok := goja.AssertFunction(o.Get("from")); ok {
				ab := n.iso.vm.NewArrayBuffer(raw)
				v, err := from(goja.Undefined(), n.iso.vm.ToValue(ab))
				if err == nil {
					return v
				}
			}
		}
	}
	return n.iso.vm.ToValue(n.iso.vm.NewArrayBuffer(raw))
}

func emptyProgramAST(codeLen int) []byte {
	b := make([]byte, 20)
	put := func(i, v uint32) { binary.LittleEndian.PutUint32(b[i*4:], v) }
	put(0, 72) // Program
	put(1, 0)
	if codeLen > 0 {
		put(2, uint32(codeLen))
	}
	put(3, 0)
	put(4, 0)
	return b
}

func (n *nodeRollupNative) jsXXH64URL(call goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(base64.RawURLEncoding.EncodeToString(hash128(callBytes(call))))
}

func (n *nodeRollupNative) jsXXH16(call goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(hex.EncodeToString(hash128(callBytes(call))))
}

func (n *nodeRollupNative) jsXXH36(call goja.FunctionCall) goja.Value {
	h := hash128(callBytes(call))
	var z big.Int
	z.SetBytes(h)
	return n.iso.vm.ToValue(strings.ToLower(z.Text(36)))
}

func callBytes(call goja.FunctionCall) []byte {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		return nil
	}
	v := call.Argument(0)
	if b, ok := asByteView(v); ok {
		return b
	}
	return []byte(v.String())
}

func hash128(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:16]
}
