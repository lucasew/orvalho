package workers

import (
	"context"
	"fmt"
	"math"
	"reflect"

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

type wasmCompiled struct {
	mod wazero.CompiledModule
	raw []byte
}

type wasmInstance struct {
	mod    api.Module
	jsBuf  []byte
	memObj *goja.Object
}

func (iso *Isolate) wasmRuntime() wazero.Runtime {
	if iso.wasmRt == nil {
		iso.wasmRt = wazero.NewRuntime(context.Background())
	}
	return iso.wasmRt
}

func (iso *Isolate) installWebAssembly() {
	iso.wasmCompiled = make(map[*goja.Object]*wasmCompiled)
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
	cp := append([]byte(nil), raw...)
	compiled, err := iso.wasmRuntime().CompileModule(context.Background(), cp)
	if err != nil {
		panic(iso.vm.NewGoError(err))
	}
	iso.wasmCompiled[call.This] = &wasmCompiled{mod: compiled, raw: cp}
	return nil
}

func (iso *Isolate) ctorWasmInstance(call goja.ConstructorCall) *goja.Object {
	c := iso.compiledOf(call.Argument(0))
	if c == nil {
		panic(iso.vm.NewTypeError("WebAssembly.Instance: first argument must be a Module"))
	}
	inst, err := iso.instantiateCompiled(c, call.Argument(1))
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
	_, err := iso.wasmRuntime().CompileModule(context.Background(), raw)
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
	compiled, err := iso.wasmRuntime().CompileModule(context.Background(), cp)
	if err != nil {
		reject(iso.vm.NewGoError(err))
		return iso.vm.ToValue(p)
	}
	resolve(iso.newWasmModule(compiled, cp))
	return iso.vm.ToValue(p)
}

func (iso *Isolate) jsWasmInstantiate(call goja.FunctionCall) goja.Value {
	p, resolve, reject := iso.vm.NewPromise()
	arg0 := call.Argument(0)
	if compiled := iso.compiledOf(arg0); compiled != nil {
		inst, err := iso.instantiateCompiled(compiled, call.Argument(1))
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
	cp := append([]byte(nil), raw...)
	compiled, err := iso.wasmRuntime().CompileModule(context.Background(), cp)
	if err != nil {
		reject(iso.vm.NewGoError(err))
		return iso.vm.ToValue(p)
	}
	jsMod := iso.newWasmModule(compiled, cp)
	inst, err := iso.instantiateCompiled(iso.wasmCompiled[jsMod], call.Argument(1))
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

func (iso *Isolate) compiledOf(v goja.Value) *wasmCompiled {
	o, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	return iso.wasmCompiled[o]
}

func (iso *Isolate) newWasmModule(compiled wazero.CompiledModule, raw []byte) *goja.Object {
	o := iso.vm.NewObject()
	iso.wasmCompiled[o] = &wasmCompiled{mod: compiled, raw: raw}
	return o
}

func (iso *Isolate) instantiateJSImports(c *wasmCompiled, importObj goja.Value) error {
	imps := c.mod.ImportedFunctions()
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
	rt := iso.wasmRuntime()
	ctx := context.Background()
	for modName, defs := range byMod {
		b := rt.NewHostModuleBuilder(modName)
		for _, def := range defs {
			_, name, _ := def.Import()
			params := def.ParamTypes()
			results := def.ResultTypes()
			jsFn := jsImportFn(jsRoot, modName, name)
			b.NewFunctionBuilder().
				WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, stack []uint64) {
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

func (iso *Isolate) instantiateCompiled(c *wasmCompiled, importObj goja.Value) (*goja.Object, error) {
	if c == nil {
		return nil, fmt.Errorf("workers: nil wasm module")
	}
	if err := iso.instantiateJSImports(c, importObj); err != nil {
		return nil, err
	}
	iso.wasmSeq++
	mod, err := iso.wasmRuntime().InstantiateModule(context.Background(), c.mod, wazero.NewModuleConfig().WithName(fmt.Sprintf("m%d", iso.wasmSeq)))
	if err != nil {
		return nil, err
	}
	st := &wasmInstance{mod: mod}
	exports := iso.vm.NewObject()
	for name := range c.mod.ExportedFunctions() {
		fn := mod.ExportedFunction(name)
		if fn == nil {
			continue
		}
		mustSet(exports, name, iso.wrapWasmFn(st, fn))
	}
	if mem := liveMemory(mod.ExportedMemory("memory")); mem != nil {
		st.attachMemory(iso, exports, mem)
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
	inst := iso.vm.NewObject()
	mustSet(inst, "exports", exports)
	return inst, nil
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
		results, err := fn.Call(context.Background(), args...)
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

func (st *wasmInstance) attachMemory(iso *Isolate, exports *goja.Object, mem api.Memory) {
	size := mem.Size()
	st.jsBuf = make([]byte, size)
	if data, ok := mem.Read(0, size); ok {
		copy(st.jsBuf, data)
	}
	st.memObj = iso.vm.NewObject()
	mustSet(st.memObj, "buffer", iso.vm.NewArrayBuffer(st.jsBuf))
	mustSet(st.memObj, "grow", func(call goja.FunctionCall) goja.Value {
		delta := uint32(0)
		if len(call.Arguments) > 0 {
			delta = uint32(call.Arguments[0].ToInteger())
		}
		prev, ok := mem.Grow(delta)
		if !ok {
			panic(iso.vm.NewGoError(fmt.Errorf("WebAssembly.Memory.grow failed")))
		}
		st.syncToWasm()
		newSize := mem.Size()
		nb := make([]byte, newSize)
		copy(nb, st.jsBuf)
		st.jsBuf = nb
		mustSet(st.memObj, "buffer", iso.vm.NewArrayBuffer(st.jsBuf))
		st.syncFromWasm()
		return iso.vm.ToValue(prev)
	})
	mustSet(exports, "memory", st.memObj)
}

func (st *wasmInstance) syncToWasm() {
	if st.mod == nil {
		return
	}
	mem := st.mod.ExportedMemory("memory")
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
	mem := st.mod.ExportedMemory("memory")
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
