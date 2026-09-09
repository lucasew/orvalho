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
	// Astro writeWebResponse does `res instanceof Http2ServerResponse`.
	// A missing export is undefined; goja then throws "Value is not an object".
	mustSet(obj, "Http2ServerRequest", func(goja.ConstructorCall) *goja.Object { return nil })
	mustSet(obj, "Http2ServerResponse", func(goja.ConstructorCall) *goja.Object { return nil })
	mustSet(obj, "default", obj)
	return obj
}
