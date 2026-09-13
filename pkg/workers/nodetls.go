package workers

import "github.com/dop251/goja"

// nodeTLSBinding materializes require("tls") / require("node:tls").
// There is no TLS stack: connect is denied unless Options.Dial is set
// (then it is still not TLS). Certificates are empty.
type nodeTLSBinding struct{}

var _ Binding = nodeTLSBinding{}

func (nodeTLSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"tls", "node:tls"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeTLS(iso), nil
}

func newNodeTLS(iso *Isolate) *goja.Object {
	n := &nodeNet{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "connect", n.jsConnect)
	mustSet(obj, "TLSSocket", n.jsSocket)
	mustSet(obj, "Socket", n.jsSocket)
	mustSet(obj, "createServer", n.jsCreateServer)
	mustSet(obj, "createSecureContext", func(goja.FunctionCall) goja.Value {
		return iso.vm.NewObject()
	})
	mustSet(obj, "rootCertificates", []any{})
	mustSet(obj, "DEFAULT_MIN_VERSION", "TLSv1.2")
	mustSet(obj, "DEFAULT_MAX_VERSION", "TLSv1.3")
	mustSet(obj, "default", obj)
	return obj
}
