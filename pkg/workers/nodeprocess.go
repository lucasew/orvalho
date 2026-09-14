package workers

import "github.com/dop251/goja"

// nodeProcessBinding exposes require("process") / require("node:process")
// as the same injected global process object.
type nodeProcessBinding struct{}

var _ Binding = nodeProcessBinding{}

func (nodeProcessBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v := iso.vm.Get("process"); v != nil {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	iso.installProcess()
	o, ok := iso.vm.Get("process").(*goja.Object)
	if !ok {
		return nil, ErrBindNilObject
	}
	return o, nil
}
