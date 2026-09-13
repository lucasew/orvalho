package workers

import "github.com/dop251/goja"

// nodeV8Binding materializes require("v8") / require("node:v8").
// There is no V8 isolate: isBuildingSnapshot is false. Heap APIs are absent.
type nodeV8Binding struct{}

var _ Binding = nodeV8Binding{}

func (nodeV8Binding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"v8", "node:v8"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeV8(iso), nil
}

func newNodeV8(iso *Isolate) *goja.Object {
	obj := iso.vm.NewObject()
	snap := iso.vm.NewObject()
	mustSet(snap, "isBuildingSnapshot", func(goja.FunctionCall) goja.Value {
		return iso.vm.ToValue(false)
	})
	mustSet(snap, "addSerializeCallback", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(snap, "addDeserializeCallback", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(snap, "setDeserializeMainFunction", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	mustSet(obj, "startupSnapshot", snap)
	mustSet(obj, "default", obj)
	return obj
}
