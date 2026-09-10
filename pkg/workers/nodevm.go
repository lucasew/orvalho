package workers

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// nodeVMBinding materializes require("vm") / require("node:vm").
// There is one isolate: runInThisContext compiles in the current global
// (jiti's module wrapper). createContext marks an object; runInContext
// evaluates with that object via `with`.
type nodeVMBinding struct{}

var _ Binding = nodeVMBinding{}

func (nodeVMBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"vm", "node:vm"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeVM(iso), nil
}

type nodeVM struct {
	iso *Isolate
	ctx map[*goja.Object]struct{}
}

func newNodeVM(iso *Isolate) *goja.Object {
	n := &nodeVM{iso: iso, ctx: map[*goja.Object]struct{}{}}
	obj := iso.vm.NewObject()
	mustSet(obj, "Script", n.jsScript)
	mustSet(obj, "runInThisContext", n.jsRunInThisContext)
	mustSet(obj, "runInNewContext", n.jsRunInNewContext)
	mustSet(obj, "runInContext", n.jsRunInContext)
	mustSet(obj, "createContext", n.jsCreateContext)
	mustSet(obj, "isContext", n.jsIsContext)
	mustSet(obj, "compileFunction", n.jsCompileFunction)
	consts := iso.vm.NewObject()
	mustSet(consts, "USE_MAIN_CONTEXT_DEFAULT_LOADER", 1)
	mustSet(consts, "DONT_CONTEXTIFY", 2)
	mustSet(obj, "constants", consts)
	mustSet(obj, "default", obj)
	return obj
}

func (n *nodeVM) jsRunInThisContext(call goja.FunctionCall) goja.Value {
	return n.run(scriptFilename(call.Argument(1)), jsToString(call.Argument(0)))
}

func (n *nodeVM) jsRunInNewContext(call goja.FunctionCall) goja.Value {
	code := jsToString(call.Argument(0))
	filename := scriptFilename(call.Argument(2))
	if len(call.Arguments) < 2 || goja.IsUndefined(call.Argument(1)) || goja.IsNull(call.Argument(1)) {
		return n.run(filename, code)
	}
	return n.runWith(filename, code, call.Argument(1).ToObject(n.iso.vm))
}

func (n *nodeVM) jsRunInContext(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		panic(n.iso.vm.NewTypeError("vm.runInContext requires a contextified sandbox"))
	}
	return n.runWith(scriptFilename(call.Argument(2)), jsToString(call.Argument(0)), call.Argument(1).ToObject(n.iso.vm))
}

func (n *nodeVM) jsCreateContext(call goja.FunctionCall) goja.Value {
	var obj *goja.Object
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		obj = call.Argument(0).ToObject(n.iso.vm)
	} else {
		obj = n.iso.vm.NewObject()
	}
	n.ctx[obj] = struct{}{}
	return obj
}

func (n *nodeVM) jsIsContext(call goja.FunctionCall) goja.Value {
	o, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return n.iso.vm.ToValue(false)
	}
	_, marked := n.ctx[o]
	return n.iso.vm.ToValue(marked)
}

func (n *nodeVM) jsCompileFunction(call goja.FunctionCall) goja.Value {
	code := jsToString(call.Argument(0))
	var b strings.Builder
	b.WriteString("(function (")
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		arr := call.Argument(1).ToObject(n.iso.vm)
		count := int(arr.Get("length").ToInteger())
		for i := 0; i < count; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(arr.Get(strconv.Itoa(i)).String())
		}
	}
	b.WriteString(") {\n")
	b.WriteString(code)
	b.WriteString("\n})")
	filename := "evalmachine.<anonymous>"
	if len(call.Arguments) > 2 {
		filename = scriptFilename(call.Argument(2))
	}
	return n.run(filename, b.String())
}

func (n *nodeVM) jsScript(call goja.ConstructorCall) *goja.Object {
	code := jsToString(call.Argument(0))
	filename := scriptFilename(call.Argument(1))
	obj := call.This
	mustSet(obj, "runInThisContext", func(goja.FunctionCall) goja.Value {
		return n.run(filename, code)
	})
	mustSet(obj, "runInNewContext", func(fc goja.FunctionCall) goja.Value {
		if len(fc.Arguments) == 0 || goja.IsUndefined(fc.Argument(0)) || goja.IsNull(fc.Argument(0)) {
			return n.run(filename, code)
		}
		return n.runWith(filename, code, fc.Argument(0).ToObject(n.iso.vm))
	})
	mustSet(obj, "runInContext", func(fc goja.FunctionCall) goja.Value {
		if len(fc.Arguments) == 0 {
			panic(n.iso.vm.NewTypeError("Script.runInContext requires a contextified sandbox"))
		}
		return n.runWith(filename, code, fc.Argument(0).ToObject(n.iso.vm))
	})
	mustSet(obj, "createCachedData", n.jsCachedData)
	return obj
}

func (n *nodeVM) jsCachedData(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(n.iso.vm.NewArrayBuffer(nil))
}

func (n *nodeVM) run(filename, code string) goja.Value {
	v, err := n.iso.vm.RunScript(filename, code)
	if err != nil {
		n.throw(err)
	}
	return v
}

func (n *nodeVM) runWith(filename, code string, sandbox *goja.Object) goja.Value {
	// Hooked eval is global (no `with` scope). Bind the sandbox and
	// compile the guest source inside `with` so reads hit that object.
	prev := n.iso.vm.Get("__orvalhoSandbox")
	mustRuntimeSet(n.iso.vm, "__orvalhoSandbox", sandbox)
	defer func() {
		if prev == nil {
			_ = n.iso.vm.Set("__orvalhoSandbox", goja.Undefined())
			return
		}
		mustRuntimeSet(n.iso.vm, "__orvalhoSandbox", prev)
	}()
	return n.run(filename, "(function () { with (__orvalhoSandbox) { return ("+code+"); } })()")
}

func (n *nodeVM) throw(err error) {
	if e, ok := err.(*goja.Exception); ok {
		panic(e)
	}
	panic(n.iso.vm.NewGoError(err))
}

func scriptFilename(v goja.Value) string {
	const anon = "evalmachine.<anonymous>"
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return anon
	}
	if o, ok := v.(*goja.Object); ok {
		if f := o.Get("filename"); f != nil && !goja.IsUndefined(f) && !goja.IsNull(f) {
			if s := f.String(); s != "" {
				return s
			}
		}
		return anon
	}
	if s := v.String(); s != "" {
		return s
	}
	return anon
}
