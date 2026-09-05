package workers

import (
	"github.com/dop251/goja"

	"github.com/lucasew/orvalho/pkg/hostobject"
)

// Bind adapts a [hostobject.Object] recipe into a [Binding] for this isolate.
// The recipe is not global; Materialize builds a new guest object on this
// isolate's runtime.
func Bind(o *hostobject.Object) Binding {
	return hostObjectBinding{o: o}
}

type hostObjectBinding struct {
	o *hostobject.Object
}

func (b hostObjectBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil {
		return nil, hostobject.ErrNoRuntime
	}
	return b.o.Build(iso.vm)
}
