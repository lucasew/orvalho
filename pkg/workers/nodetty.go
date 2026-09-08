package workers

import "github.com/dop251/goja"

// nodeTTYBinding materializes require("tty") / require("node:tty").
// There is no host TTY: isatty is always false.
type nodeTTYBinding struct{}

var _ Binding = nodeTTYBinding{}

func (nodeTTYBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"tty", "node:tty"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeTTY(iso), nil
}

type nodeTTY struct {
	iso *Isolate
}

func newNodeTTY(iso *Isolate) *goja.Object {
	n := &nodeTTY{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "isatty", n.jsIsatty)
	mustSet(obj, "ReadStream", n.jsReadStream)
	mustSet(obj, "WriteStream", n.jsWriteStream)
	mustSet(obj, "default", obj)
	return obj
}

func (n *nodeTTY) jsIsatty(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(false)
}

func (n *nodeTTY) jsReadStream(call goja.ConstructorCall) *goja.Object {
	return n.initStream(call.This)
}

func (n *nodeTTY) jsWriteStream(call goja.ConstructorCall) *goja.Object {
	return n.initStream(call.This)
}

func (n *nodeTTY) initStream(this *goja.Object) *goja.Object {
	mustSet(this, "isTTY", false)
	mustSet(this, "setRawMode", func(call goja.FunctionCall) goja.Value {
		return call.This
	})
	mustSet(this, "getWindowSize", func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue([]int{0, 0})
	})
	mustSet(this, "getColorDepth", func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(1)
	})
	mustSet(this, "hasColors", func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(false)
	})
	return this
}
