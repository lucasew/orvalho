package workers

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/dop251/goja"
)

func isESMLexerFile(file string) bool {
	if !strings.Contains(file, "es-module-lexer") {
		return false
	}
	return strings.HasSuffix(file, "lexer.js") || strings.HasSuffix(file, "lexer.cjs")
}

// patchESMLexer replaces the JS parse wrapper (UTF-16 copy via charCodeAt)
// with a host call into the same official lexer wasm. init stays the
// guest promise so instantiate still runs.
func (iso *Isolate) patchESMLexer(exports goja.Value) goja.Value {
	obj := iso.vm.NewObject()
	mustSet(obj, "parse", iso.jsESMParse)
	if src, ok := exports.(*goja.Object); ok && src != nil {
		if init := src.Get("init"); init != nil && !goja.IsUndefined(init) {
			mustSet(obj, "init", init)
		}
		if syn := src.Get("initSync"); syn != nil && !goja.IsUndefined(syn) {
			mustSet(obj, "initSync", syn)
		}
		if it := src.Get("ImportType"); it != nil && !goja.IsUndefined(it) {
			mustSet(obj, "ImportType", it)
		}
	}
	if obj.Get("init") == nil || goja.IsUndefined(obj.Get("init")) {
		p, resolve, _ := iso.vm.NewPromise()
		resolve(goja.Undefined())
		mustSet(obj, "init", iso.vm.ToValue(p))
	}
	mustSet(obj, "default", obj)
	return obj
}

func (iso *Isolate) jsESMParse(call goja.FunctionCall) goja.Value {
	src := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		src = call.Argument(0).String()
	}
	imps, exps, facade, hasMod, err := iso.parseESMLexer(src)
	if err != nil {
		panic(iso.vm.NewGoError(err))
	}
	impVals := make([]interface{}, 0, len(imps))
	for _, im := range imps {
		o := iso.vm.NewObject()
		if im.n != "" {
			mustSet(o, "n", im.n)
		}
		mustSet(o, "t", im.t)
		mustSet(o, "s", im.s)
		mustSet(o, "e", im.e)
		mustSet(o, "ss", im.ss)
		mustSet(o, "se", im.se)
		mustSet(o, "d", im.d)
		mustSet(o, "a", im.a)
		mustSet(o, "at", goja.Null())
		impVals = append(impVals, o)
	}
	expVals := make([]interface{}, 0, len(exps))
	for _, ex := range exps {
		o := iso.vm.NewObject()
		mustSet(o, "n", ex.n)
		mustSet(o, "s", ex.s)
		mustSet(o, "e", ex.e)
		mustSet(o, "ls", ex.ls)
		mustSet(o, "le", ex.le)
		if ex.ln != "" {
			mustSet(o, "ln", ex.ln)
		}
		expVals = append(expVals, o)
	}
	return iso.vm.NewArray(iso.vm.NewArray(impVals...), iso.vm.NewArray(expVals...), facade, hasMod)
}

type esmImport struct {
	n       string
	t, s, e int
	ss, se  int
	d, a    int
}

type esmExport struct {
	n, ln  string
	s, e   int
	ls, le int
}

func (iso *Isolate) parseESMLexer(src string) (imps []esmImport, exps []esmExport, facade, hasMod bool, err error) {
	st := iso.esmLexer
	if st == nil || st.mod == nil || st.mem == nil {
		return nil, nil, false, false, fmt.Errorf("es-module-lexer: wasm not ready (await init)")
	}
	u16 := utf16.Encode([]rune(src))
	n := len(u16)
	ctx := iso.activeCtx
	addr, err := wasmCallI32(ctx, st, "sa", uint64(n))
	if err != nil {
		return nil, nil, false, false, err
	}
	buf := make([]byte, (n+1)*2)
	for i, c := range u16 {
		binary.LittleEndian.PutUint16(buf[i*2:], c)
	}
	if !st.mem.Write(uint32(addr), buf) {
		return nil, nil, false, false, fmt.Errorf("es-module-lexer: write source")
	}
	ok, err := wasmCallI32(ctx, st, "parse")
	if err != nil {
		return nil, nil, false, false, err
	}
	if ok == 0 {
		idx, _ := wasmCallI32(ctx, st, "e")
		return nil, nil, false, false, fmt.Errorf("es-module-lexer: parse error at %d", idx)
	}
	for {
		more, err := wasmCallI32(ctx, st, "ri")
		if err != nil || more == 0 {
			break
		}
		s, _ := wasmCallI32(ctx, st, "is")
		e, _ := wasmCallI32(ctx, st, "ie")
		t, _ := wasmCallI32(ctx, st, "it")
		a, _ := wasmCallI32(ctx, st, "ai")
		d, _ := wasmCallI32(ctx, st, "id")
		ss, _ := wasmCallI32(ctx, st, "ss")
		se, _ := wasmCallI32(ctx, st, "se")
		im := esmImport{t: int(t), s: int(s), e: int(e), ss: int(ss), se: int(se), d: int(d), a: int(a)}
		if ip, _ := wasmCallI32(ctx, st, "ip"); ip != 0 {
			lo, hi := int(s), int(e)
			if d == -1 {
				lo, hi = int(s)-1, int(e)+1
			}
			im.n = decodeJSString(sliceUTF16(src, lo, hi))
		}
		imps = append(imps, im)
	}
	for {
		more, err := wasmCallI32(ctx, st, "re")
		if err != nil || more == 0 {
			break
		}
		s, _ := wasmCallI32(ctx, st, "es")
		e, _ := wasmCallI32(ctx, st, "ee")
		ls, _ := wasmCallI32(ctx, st, "els")
		le, _ := wasmCallI32(ctx, st, "ele")
		ex := esmExport{s: int(s), e: int(e), ls: int(ls), le: int(le)}
		ex.n = decodeJSString(sliceUTF16(src, int(s), int(e)))
		if ls >= 0 {
			ex.ln = decodeJSString(sliceUTF16(src, int(ls), int(le)))
		}
		exps = append(exps, ex)
	}
	if f, _ := wasmCallI32(ctx, st, "f"); f != 0 {
		facade = true
	}
	if ms, _ := wasmCallI32(ctx, st, "ms"); ms != 0 {
		hasMod = true
	}
	return imps, exps, facade, hasMod, nil
}

func wasmCallI32(ctx context.Context, st *wasmInstance, name string, args ...uint64) (int32, error) {
	fn := st.mod.ExportedFunction(name)
	if fn == nil {
		return 0, fmt.Errorf("es-module-lexer: missing export %s", name)
	}
	out, err := fn.Call(ctx, args...)
	if err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	return int32(out[0]), nil
}

func sliceUTF16(s string, start, end int) string {
	u := utf16.Encode([]rune(s))
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(u) {
		return ""
	}
	if end > len(u) {
		end = len(u)
	}
	return string(utf16.Decode(u[start:end]))
}

func decodeJSString(q string) string {
	if q == "" {
		return ""
	}
	if q[0] == '"' || q[0] == '\'' {
		if u, err := strconv.Unquote(q); err == nil {
			return u
		}
		if q[0] == '\'' && len(q) >= 2 && q[len(q)-1] == '\'' {
			inner := strings.ReplaceAll(q[1:len(q)-1], `\'`, `'`)
			if u, err := strconv.Unquote(`"` + strings.ReplaceAll(inner, `"`, `\"`) + `"`); err == nil {
				return u
			}
			return inner
		}
	}
	return q
}
