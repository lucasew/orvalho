package workers

import "github.com/dop251/goja"

// nodeHTTP2Binding materializes require("http2"). No HTTP/2 session.
type nodeHTTP2Binding struct{}

var _ Binding = nodeHTTP2Binding{}

func (nodeHTTP2Binding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"http2", "node:http2"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeHTTP2(iso), nil
}

func newNodeHTTP2(iso *Isolate) *goja.Object {
	n := &nodeHTTP{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "createServer", n.jsCreateServer)
	mustSet(obj, "createSecureServer", n.jsCreateServer)
	mustSet(obj, "connect", n.jsDeniedOut)
	mustSet(obj, "constants", iso.vm.NewObject())
	mustSet(obj, "default", obj)
	return obj
}
