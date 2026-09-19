package workers

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// nodeRollupParseAstBinding is import { parseAst } from "rollup/parseAst".
// Vite SSR walks ESTree (ImportDeclaration, start/end). The native
// parse() buffer is a different format; we use acorn from the guest tree.
type nodeRollupParseAstBinding struct{}

var _ Binding = nodeRollupParseAstBinding{}

func isRollupParseAstFile(spec string) bool {
	if !strings.Contains(spec, "rollup") {
		return false
	}
	return strings.HasSuffix(spec, "/parseAst") || strings.HasSuffix(spec, "/parseAst.js")
}

func (nodeRollupParseAstBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v, ok := iso.moduleCache["rollup/parseAst"]; ok {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	n := &nodeRollupParseAst{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "parseAst", n.jsParseAst)
	mustSet(obj, "parseAstAsync", n.jsParseAstAsync)
	iso.moduleCache["rollup/parseAst"] = obj
	return obj, nil
}

type nodeRollupParseAst struct {
	iso     *Isolate
	parseFn goja.Callable
}

func (n *nodeRollupParseAst) acornParse() (goja.Callable, error) {
	if n.parseFn != nil {
		return n.parseFn, nil
	}
	mod, err := n.iso.loadModule("acorn")
	if err != nil {
		return nil, err
	}
	o, ok := mod.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("%w: acorn", ErrModuleNotFound)
	}
	fn, ok := goja.AssertFunction(o.Get("parse"))
	if !ok {
		return nil, fmt.Errorf("%w: acorn.parse", ErrModuleNotFound)
	}
	n.parseFn = fn
	return fn, nil
}

func (n *nodeRollupParseAst) doParse(call goja.FunctionCall) (goja.Value, error) {
	code := ""
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		code = call.Argument(0).String()
	}
	allowReturn, jsx := false, false
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		if o, ok := call.Argument(1).(*goja.Object); ok {
			if v := o.Get("allowReturnOutsideFunction"); v != nil && v.ToBoolean() {
				allowReturn = true
			}
			if v := o.Get("jsx"); v != nil && v.ToBoolean() {
				jsx = true
			}
		}
	}
	_ = jsx
	parse, err := n.acornParse()
	if err != nil {
		return goja.Undefined(), err
	}
	opts := n.iso.vm.NewObject()
	mustSet(opts, "ecmaVersion", "latest")
	mustSet(opts, "sourceType", "module")
	mustSet(opts, "allowReturnOutsideFunction", allowReturn)
	mustSet(opts, "allowAwaitOutsideFunction", true)
	v, err := parse(goja.Undefined(), n.iso.vm.ToValue(code), opts)
	if err != nil {
		return goja.Undefined(), err
	}
	return v, nil
}

func (n *nodeRollupParseAst) jsParseAst(call goja.FunctionCall) goja.Value {
	v, err := n.doParse(call)
	if err != nil {
		e := n.iso.vm.NewGoError(err)
		_ = e.Set("code", "PARSE_ERROR")
		panic(e)
	}
	return v
}

func (n *nodeRollupParseAst) jsParseAstAsync(call goja.FunctionCall) goja.Value {
	p, resolve, reject := n.iso.vm.NewPromise()
	v, err := n.doParse(call)
	if err != nil {
		e := n.iso.vm.NewGoError(err)
		_ = e.Set("code", "PARSE_ERROR")
		reject(e)
	} else {
		resolve(v)
	}
	return n.iso.vm.ToValue(p)
}
