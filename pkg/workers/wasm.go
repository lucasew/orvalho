package workers

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/lucasew/orvalho/pkg/wasm"

	"github.com/dop251/goja"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func liveMemory(mem api.Memory) api.Memory {
	if mem == nil {
		return nil
	}
	v := reflect.ValueOf(mem)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}
	return mem
}

type wasmInstance struct {
	mod    api.Module
	mem    api.Memory
	jsBuf  []byte
	jsAB   goja.ArrayBuffer
	memObj *goja.Object
}

func (st *wasmInstance) hasResume() bool {
	return st != nil && st.mod != nil && st.mod.ExportedFunction("resume") != nil
}

func (iso *Isolate) wasmHub() *wasm.Hub {
	if iso.wasm == nil {
		iso.wasm = wasm.NewHub(iso.activeCtx)
	}
	return iso.wasm
}

func (iso *Isolate) installWebAssembly() {
	iso.wasmMods = make(map[*goja.Object]*wasm.Bin)
	wa := iso.vm.NewObject()
	mustSet(wa, "compile", iso.jsWasmCompile)
	mustSet(wa, "instantiate", iso.jsWasmInstantiate)
	mustSet(wa, "validate", iso.jsWasmValidate)
	mustSet(wa, "Module", iso.ctorWasmModule)
	mustSet(wa, "Instance", iso.ctorWasmInstance)
	mustSet(wa, "Memory", func(call goja.ConstructorCall) *goja.Object {
		mustSet(call.This, "buffer", iso.vm.NewArrayBuffer(make([]byte, 65536)))
		return nil
	})
	mustSet(wa, "Table", func(call goja.ConstructorCall) *goja.Object { return nil })
	mustRuntimeSet(iso.vm, "WebAssembly", wa)
}

func (iso *Isolate) ctorWasmModule(call goja.ConstructorCall) *goja.Object {
	raw, ok := asByteView(call.Argument(0))
	if !ok {
		panic(iso.vm.NewTypeError("WebAssembly.Module: first argument must be a BufferSource"))
	}
	iso.wasmMods[call.This] = iso.wasmHub().Bin(raw)
	return nil
}

func (iso *Isolate) ctorWasmInstance(call goja.ConstructorCall) *goja.Object {
	b := iso.binOf(call.Argument(0))
	if b == nil {
		panic(iso.vm.NewTypeError("WebAssembly.Instance: first argument must be a Module"))
	}
	inst, err := iso.instantiateCompiled(b, call.Argument(1))
	if err != nil {
		panic(iso.vm.NewGoError(err))
	}
	if exp := inst.Get("exports"); exp != nil {
		mustSet(call.This, "exports", exp)
	}
	return nil
}

func (iso *Isolate) jsWasmValidate(call goja.FunctionCall) goja.Value {
	raw, ok := asByteView(call.Argument(0))
	if !ok {
		return iso.vm.ToValue(false)
	}
	_, err := iso.wasmHub().Bin(raw).Compiled(iso.activeCtx, iso.wasmHub().Runtime())
	return iso.vm.ToValue(err == nil)
}

func (iso *Isolate) jsWasmCompile(call goja.FunctionCall) goja.Value {
	p, resolve, reject := iso.vm.NewPromise()
	raw, ok := asByteView(call.Argument(0))
	if !ok {
		reject(iso.vm.NewTypeError("WebAssembly.compile: first argument must be a BufferSource"))
		return iso.vm.ToValue(p)
	}
	cp := append([]byte(nil), raw...)
	resolve(iso.newWasmModule(iso.wasmHub().Bin(cp)))
	return iso.vm.ToValue(p)
}

func (iso *Isolate) jsWasmInstantiate(call goja.FunctionCall) goja.Value {
	p, resolve, reject := iso.vm.NewPromise()
	arg0 := call.Argument(0)
	if b := iso.binOf(arg0); b != nil {
		inst, err := iso.instantiateCompiled(b, call.Argument(1))
		if err != nil {
			reject(iso.vm.NewGoError(err))
			return iso.vm.ToValue(p)
		}
		resolve(inst)
		return iso.vm.ToValue(p)
	}
	raw, ok := asByteView(arg0)
	if !ok {
		reject(iso.vm.NewTypeError("WebAssembly.instantiate: first argument must be a BufferSource or Module"))
		return iso.vm.ToValue(p)
	}
	b := iso.wasmHub().Bin(raw)
	jsMod := iso.newWasmModule(b)
	inst, err := iso.instantiateCompiled(b, call.Argument(1))
	if err != nil {
		reject(iso.vm.NewGoError(err))
		return iso.vm.ToValue(p)
	}
	out := iso.vm.NewObject()
	mustSet(out, "module", jsMod)
	mustSet(out, "instance", inst)
	resolve(out)
	return iso.vm.ToValue(p)
}

func (iso *Isolate) binOf(v goja.Value) *wasm.Bin {
	o, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	return iso.wasmMods[o]
}

func (iso *Isolate) newWasmModule(b *wasm.Bin) *goja.Object {
	o := iso.wasmBrandObject("Module")
	iso.wasmMods[o] = b
	return o
}

func (iso *Isolate) wasmBrandObject(ctorName string) *goja.Object {
	wa, ok := iso.vm.Get("WebAssembly").(*goja.Object)
	if !ok || wa == nil {
		return iso.vm.NewObject()
	}
	ctor, ok := wa.Get(ctorName).(*goja.Object)
	if !ok || ctor == nil {
		return iso.vm.NewObject()
	}
	proto, ok := ctor.Get("prototype").(*goja.Object)
	if !ok || proto == nil {
		return iso.vm.NewObject()
	}
	return iso.vm.CreateObject(proto)
}

func (iso *Isolate) instantiateJSImports(b *wasm.Bin, importObj goja.Value) error {
	compiled, err := b.Compiled(iso.activeCtx, iso.wasmHub().Runtime())
	if err != nil {
		return err
	}
	imps := compiled.ImportedFunctions()
	if len(imps) == 0 {
		return nil
	}
	byMod := map[string][]api.FunctionDefinition{}
	for _, def := range imps {
		modName, _, ok := def.Import()
		if !ok {
			continue
		}
		byMod[modName] = append(byMod[modName], def)
	}
	jsRoot, _ := importObj.(*goja.Object)
	rt := iso.wasmHub().Runtime()
	ctx := iso.activeCtx
	for modName, defs := range byMod {
		b := rt.NewHostModuleBuilder(modName)
		for _, def := range defs {
			_, name, _ := def.Import()
			params := def.ParamTypes()
			results := def.ResultTypes()
			jsFn := jsImportFn(jsRoot, modName, name)
			gojsName := name
			useGoJS := modName == "gojs"
			b.NewFunctionBuilder().
				WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, stack []uint64) {
					if inst := iso.wasmActive; inst != nil {
						inst.rebindMemory(iso)
						inst.syncFromWasm()
					}
					if useGoJS {
						if iso.wasmGo == nil {
							iso.wasmGo = newWasmGoJS(iso)
						}
						sp := uint32(0)
						if len(stack) > 0 {
							sp = uint32(stack[0])
						}
						iso.wasmGo.handle(gojsName, sp)
						if inst := iso.wasmActive; inst != nil {
							inst.syncToWasm()
						}
						return
					}
					if jsFn == nil {
						return
					}
					args := make([]goja.Value, len(params))
					for i, t := range params {
						args[i] = iso.vm.ToValue(wasmToJS(stack[i], t))
					}
					ret, err := jsFn(goja.Undefined(), args...)
					if err != nil {
						panic(err)
					}
					if inst := iso.wasmActive; inst != nil {
						inst.syncToWasm()
					}
					if len(results) > 0 && ret != nil {
						stack[0] = jsToWasm(ret, results[0])
					}
				}), params, results).
				Export(name)
		}
		if prev := rt.Module(modName); prev != nil {
			_ = prev.Close(ctx)
		}
		if _, err := b.Instantiate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func jsImportFn(root *goja.Object, modName, name string) goja.Callable {
	if root == nil {
		return nil
	}
	mod := root.Get(modName)
	o, ok := mod.(*goja.Object)
	if !ok {
		return nil
	}
	fn, ok := goja.AssertFunction(o.Get(name))
	if !ok {
		return nil
	}
	return fn
}

func wasmToJS(v uint64, t api.ValueType) any {
	switch t {
	case api.ValueTypeI32:
		return int32(v)
	case api.ValueTypeI64:
		return int64(v)
	case api.ValueTypeF32:
		return math.Float32frombits(uint32(v))
	case api.ValueTypeF64:
		return math.Float64frombits(v)
	default:
		return int64(v)
	}
}

func jsToWasm(v goja.Value, t api.ValueType) uint64 {
	n := v.ToFloat()
	switch t {
	case api.ValueTypeI32:
		return api.EncodeI32(int32(int64(n)))
	case api.ValueTypeI64:
		return api.EncodeI64(int64(n))
	case api.ValueTypeF32:
		return api.EncodeF32(float32(n))
	case api.ValueTypeF64:
		return api.EncodeF64(n)
	default:
		return uint64(int64(n))
	}
}

func (iso *Isolate) instantiateCompiled(b *wasm.Bin, importObj goja.Value) (*goja.Object, error) {
	if b == nil {
		return nil, fmt.Errorf("workers: nil wasm module")
	}
	compiled, err := b.Compiled(iso.activeCtx, iso.wasmHub().Runtime())
	if err != nil {
		return nil, err
	}
	iso.trace("wasm instantiate %dB", len(b.Bytes()))
	if err := iso.instantiateJSImports(b, importObj); err != nil {
		iso.trace("wasm instantiate fail: %v", err)
		return nil, err
	}
	iso.wasmSeq++
	t0 := time.Now()
	mod, err := iso.wasmHub().Runtime().InstantiateModule(iso.activeCtx, compiled, wazero.NewModuleConfig().WithName(fmt.Sprintf("m%d", iso.wasmSeq)))
	if err != nil {
		iso.trace("wasm instantiate fail: %v", err)
		return nil, err
	}
	iso.trace("wasm instantiate ok %dB %s", len(b.Bytes()), time.Since(t0).Round(time.Millisecond))
	st := &wasmInstance{mod: mod}
	exports := iso.vm.NewObject()
	for name := range compiled.ExportedFunctions() {
		fn := mod.ExportedFunction(name)
		if fn == nil {
			continue
		}
		mustSet(exports, name, iso.wrapWasmFn(st, fn))
	}
	for name := range mod.ExportedMemoryDefinitions() {
		if mem := liveMemory(mod.ExportedMemory(name)); mem != nil {
			st.attachMemory(iso, exports, name, mem)
		}
	}
	for _, name := range exportedGlobalNames(mod) {
		g := mod.ExportedGlobal(name)
		if g == nil {
			continue
		}
		o := iso.vm.NewObject()
		mustSet(o, "value", globalJS(g))
		mustSet(exports, name, o)
	}
	inst := iso.wasmBrandObject("Instance")
	mustSet(inst, "exports", exports)
	if iso.wasmGo != nil && mod.ExportedFunction("resume") != nil {
		iso.wasmGo.inst = st
	}
	if isESMLexerWasm(mod) {
		iso.esmLexer = st
	}
	return inst, nil
}

func isESMLexerWasm(mod api.Module) bool {
	return mod != nil &&
		mod.ExportedFunction("parse") != nil &&
		mod.ExportedFunction("sa") != nil &&
		mod.ExportedFunction("ri") != nil &&
		mod.ExportedFunction("is") != nil &&
		mod.ExportedFunction("ie") != nil
}

func exportedGlobalNames(mod api.Module) []string {
	// Walk a small fixed set used by guests; plus any we can probe via
	// ExportedFunctionDefinitions-style API is unavailable for globals.
	// Common lexer / WASI names:
	cands := []string{"__heap_base", "__data_end", "__stack_pointer"}
	var out []string
	for _, n := range cands {
		if mod.ExportedGlobal(n) != nil {
			out = append(out, n)
		}
	}
	return out
}

func globalJS(g api.Global) any {
	v := g.Get()
	switch g.Type() {
	case api.ValueTypeI32:
		return int32(v)
	case api.ValueTypeI64:
		return int64(v)
	case api.ValueTypeF32:
		return math.Float32frombits(uint32(v))
	case api.ValueTypeF64:
		return math.Float64frombits(v)
	default:
		return int64(v)
	}
}

func (iso *Isolate) wrapWasmFn(st *wasmInstance, fn api.Function) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		prev := iso.wasmActive
		iso.wasmActive = st
		defer func() {
			// Go wasm parks in run() and later resume()s from
			// _makeFuncWrapper. A prior module (xxhash) must not
			// steal wasmActive back or gojs hits the wrong instance.
			if st.hasResume() {
				return
			}
			iso.wasmActive = prev
		}()
		st.syncToWasm()
		params := fn.Definition().ParamTypes()
		args := make([]uint64, len(params))
		for i, t := range params {
			var n float64
			if i < len(call.Arguments) && call.Arguments[i] != nil {
				n = call.Arguments[i].ToFloat()
			}
			switch t {
			case api.ValueTypeI32:
				args[i] = api.EncodeI32(int32(int64(n)))
			case api.ValueTypeI64:
				args[i] = api.EncodeI64(int64(n))
			case api.ValueTypeF32:
				args[i] = api.EncodeF32(float32(n))
			case api.ValueTypeF64:
				args[i] = api.EncodeF64(n)
			}
		}
		results, err := fn.Call(iso.activeCtx, args...)
		st.rebindMemory(iso)
		st.syncFromWasm()
		if err != nil {
			panic(iso.vm.NewGoError(err))
		}
		if len(results) == 0 {
			return goja.Undefined()
		}
		rts := fn.Definition().ResultTypes()
		if len(rts) == 0 {
			return iso.vm.ToValue(results[0])
		}
		switch rts[0] {
		case api.ValueTypeI32:
			return iso.vm.ToValue(int32(results[0]))
		case api.ValueTypeI64:
			return iso.vm.ToValue(int64(results[0]))
		case api.ValueTypeF32:
			return iso.vm.ToValue(math.Float32frombits(uint32(results[0])))
		case api.ValueTypeF64:
			return iso.vm.ToValue(math.Float64frombits(results[0]))
		default:
			return iso.vm.ToValue(results[0])
		}
	}
}

// wasmJSMemMin is the starting wasm memory we give Go wasm_exec.
// jsBuf always matches mem.Size(); a larger JS view than wasm (or the
// reverse) makes es-module-lexer grow forever and OOM.
const wasmJSMemMin = 64 << 20

// wasmJSMemMax caps Memory.grow. A wrong heap_base used to request
// gigabytes and take the host down.
const wasmJSMemMax = 256 << 20

func (st *wasmInstance) attachMemory(iso *Isolate, exports *goja.Object, name string, mem api.Memory) {
	if mem.Size() < wasmJSMemMin {
		need := wasmJSMemMin - mem.Size()
		pages := (need + 65535) / 65536
		if _, ok := mem.Grow(uint32(pages)); !ok {
			iso.trace("wasm memory pregrow %d pages failed, size=%d", pages, mem.Size())
		}
	}
	st.mem = mem
	st.memObj = iso.vm.NewObject()
	st.bindJSMem(iso)
	mustSet(st.memObj, "grow", func(call goja.FunctionCall) goja.Value {
		delta := uint32(0)
		if len(call.Arguments) > 0 {
			n := call.Argument(0).ToInteger()
			if n < 0 {
				n = 0
			}
			delta = uint32(n)
		}
		next := uint64(mem.Size()) + uint64(delta)*65536
		if next > wasmJSMemMax {
			panic(iso.vm.NewTypeError("WebAssembly.Memory.grow: exceeds maximum"))
		}
		prev, ok := mem.Grow(delta)
		if !ok {
			panic(iso.vm.NewGoError(fmt.Errorf("WebAssembly.Memory.grow failed")))
		}
		st.syncToWasm()
		st.bindJSMem(iso)
		return iso.vm.ToValue(prev)
	})
	mustSet(exports, name, st.memObj)
	if name != "memory" && exports.Get("memory") == nil {
		mustSet(exports, "memory", st.memObj)
	}
}

// bindJSMem makes memory.buffer a fresh ArrayBuffer whose length is
// mem.Size() and detaches the previous one. The WebAssembly JS API
// detaches on grow; rollup's wasm-bindgen cache only refreshes a
// Uint8Array when its byteLength is 0, so a live stale view writes
// into the old buffer and parse then slices a garbage length (OOM).
func (st *wasmInstance) bindJSMem(iso *Isolate) {
	size := st.mem.Size()
	if size > wasmJSMemMax {
		panic(iso.vm.NewTypeError("WebAssembly.Memory: exceeds maximum"))
	}
	buf := make([]byte, size)
	copy(buf, st.jsBuf)
	if data, ok := st.mem.Read(0, size); ok {
		copy(buf, data)
	}
	prev := st.jsAB
	st.jsBuf = buf
	st.jsAB = iso.vm.NewArrayBuffer(st.jsBuf)
	mustSet(st.memObj, "buffer", st.jsAB)
	if prev != (goja.ArrayBuffer{}) {
		prev.Detach()
	}
}

func (st *wasmInstance) rebindMemory(iso *Isolate) {
	if st.mem != nil && uint32(len(st.jsBuf)) != st.mem.Size() {
		st.bindJSMem(iso)
		return
	}
	st.syncFromWasm()
}

func (st *wasmInstance) syncToWasm() {
	if st.mod == nil {
		return
	}
	mem := st.mem
	if mem == nil || st.jsBuf == nil {
		return
	}
	n := uint32(len(st.jsBuf))
	if mem.Size() < n {
		n = mem.Size()
	}
	_ = mem.Write(0, st.jsBuf[:n])
}

func (st *wasmInstance) syncFromWasm() {
	if st.mod == nil {
		return
	}
	mem := st.mem
	if mem == nil || st.jsBuf == nil {
		return
	}
	n := uint32(len(st.jsBuf))
	if mem.Size() < n {
		n = mem.Size()
	}
	if data, ok := mem.Read(0, n); ok {
		copy(st.jsBuf, data)
	}
}
